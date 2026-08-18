package middleware

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib" // 注册 pgx database/sql 驱动，用于建库/原生 SQL 反查
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	repo "roche.local/knowledge-agent-platform/internal/application/repository"
	svc "roche.local/knowledge-agent-platform/internal/application/service"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// ---------------------------------------------------------------------------
// 审计日志 PG 集成测试（真实 HTTP 接口调用 + 生产同款中间件链）
//
// 与 sqlite 版（audit_integration_test.go）不同，这里：
//   - 使用真实 PostgreSQL（gorm.io/driver/postgres）+ 真实 AuditLogRepository/
//     AuditLogService/GlobalAuditRecorder，走完整的写入链路；
//   - 使用 httptest.NewServer 启动【真实 TCP HTTP server】（127.0.0.1 随机端口），
//     全部请求通过 http.Client 走真实网络发出（与生产 net/http.Server 相同）；
//   - 中间件链与生产 router.go 完全一致：
//       InternalServiceAuth → Auth → BanCheck → AuditServiceProvider → GlobalAuditRecorder
//     认证走生产同款 InternalServiceAuth：请求头 X-Internal-Service-Token 匹配时
//     注入 system-admin 身份（internal-service），Auth 检测到已注入用户后跳过 JWT 校验；
//     未携带 token 的请求则走 Auth 的真实 401 拦截路径（不产生审计行）。
//   - 写入独立测试库 rochekap_audit_test（不污染 RocheKAP 生产库）；
//   - 测试结束后数据保留在 PG 中，可用 psql 直接查询核验；
//   - 提供原生 SQL 反查（绕过 GORM），证明数据真实落库。
//
// 连接配置（环境变量，均有默认值指向本地 dev PG）：
//   AUDIT_TEST_PG_HOST     默认 localhost
//   AUDIT_TEST_PG_PORT     默认 5432
//   AUDIT_TEST_PG_USER     默认 postgres
//   AUDIT_TEST_PG_PASSWORD 默认 postgres123!@#
//   AUDIT_TEST_PG_DB       默认 rochekap_audit_test
//
// 查询落库数据示例：
//   docker exec roche-kap-postgres-dev psql -U postgres -d rochekap_audit_test \
//     -c "SELECT id, action, request_method, request_path, outcome, actor_user_id, actor_role, details FROM audit_logs ORDER BY id"
// ---------------------------------------------------------------------------

const auditTestDBName = "rochekap_audit_test"

// testInternalServiceToken 是测试用内部服务共享 token，
// 对应生产 INTERNAL_SERVICE_TOKEN / config.InternalService.Token。
const testInternalServiceToken = "test-internal-service-token"

// pgTestConn 封装 PG 连接信息。
type pgTestConn struct {
	host string
	port string
	user string
	pass string
	db   string
}

// auditPGTestConn 从环境变量（带默认值）构建连接信息。
func auditPGTestConn(dbName string) pgTestConn {
	return pgTestConn{
		host: envOr("AUDIT_TEST_PG_HOST", "localhost"),
		port: envOr("AUDIT_TEST_PG_PORT", "5432"),
		user: envOr("AUDIT_TEST_PG_USER", "postgres"),
		pass: envOr("AUDIT_TEST_PG_PASSWORD", "postgres123!@#"),
		db:   dbName,
	}
}

// dsn 生成 GORM/lib/pq 兼容的 key=value 格式 DSN。
func (c pgTestConn) dsn() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.host, c.port, c.user, c.pass, c.db)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ensureAuditTestDB 幂等创建测试数据库（连 admin 库 postgres）。
func ensureAuditTestDB(t *testing.T) {
	t.Helper()
	admin := auditPGTestConn("postgres")
	sqlDB, err := sql.Open("pgx", admin.dsn())
	if err != nil {
		t.Fatalf("connect postgres admin: %v", err)
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping postgres admin (host=%s port=%s user=%s): %v", admin.host, admin.port, admin.user, err)
	}
	var exists bool
	if err := sqlDB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", auditTestDBName,
	).Scan(&exists); err != nil {
		t.Fatalf("check test database: %v", err)
	}
	if !exists {
		if _, err := sqlDB.Exec(`CREATE DATABASE "` + auditTestDBName + `"`); err != nil {
			t.Fatalf("create test database: %v", err)
		}
		t.Logf("created test database %q", auditTestDBName)
	}
}

// auditPGEnv 组装真实 PG 审计链路 + 生产同款中间件链 + 真实 TCP HTTP server。
type auditPGEnv struct {
	t             *testing.T
	db            *gorm.DB
	svc           interfaces.AuditLogService
	recorder      *GlobalAuditRecorder
	router        *gin.Engine
	server        *httptest.Server // 真实 TCP 监听（127.0.0.1:随机端口）
	baseURL       string           // server.URL，如 http://127.0.0.1:12345
	client        *http.Client     // 真实 HTTP 客户端
	internalToken string           // X-Internal-Service-Token 值
}

// newAuditPGEnv 构建 PG 测试环境。recordGET 对应 audit.global.record_get 配置。
// 所有接口调用均通过真实 TCP HTTP（http.Client → httptest.Server）发出，
// 中间件链与生产 internal/router/router.go 完全一致。
func newAuditPGEnv(t *testing.T, recordGET bool) *auditPGEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureAuditTestDB(t)

	db, err := gorm.Open(postgres.Open(auditPGTestConn(auditTestDBName).dsn()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&types.AuditLog{}); err != nil {
		t.Fatalf("migrate audit_logs: %v", err)
	}
	// 每次测试前清空，保证可重复运行；序列不重置，不影响断言。
	if err := db.Exec("DELETE FROM audit_logs").Error; err != nil {
		t.Fatalf("clean audit_logs: %v", err)
	}

	cfg := &config.Config{
		Audit: &config.AuditConfig{
			Global: &config.GlobalAuditConfig{
				Enabled:     true,
				CaptureBody: true,
				RecordGET:   recordGET,
			},
		},
		InternalService: &config.InternalServiceConfig{
			Token: testInternalServiceToken,
		},
	}

	auditSvc := svc.NewAuditLogService(repo.NewAuditLogRepository(db))
	rec := NewGlobalAuditRecorder(auditSvc, cfg)

	// 生产同款中间件链（对齐 internal/router/router.go）：
	//   InternalServiceAuth → Auth → BanCheck → AuditServiceProvider → GlobalAuditRecorder
	// 携带 X-Internal-Service-Token 的请求由 InternalServiceAuth 注入 system-admin
	// 身份（internal-service），Auth 检测到已注入用户后跳过 JWT 校验；
	// 未携带 token 的请求走 Auth 的真实 401 拦截路径（不产生审计行）。
	// Auth/BanCheck 的仓库参数传 nil：本测试经内部 token 或未认证路径进入，
	// 不会触发 ValidateToken/黑名单查询（中间件对 nil 均安全降级）。
	r := gin.New()
	r.Use(InternalServiceAuth(cfg))
	r.Use(Auth(nil))
	r.Use(BanCheck(nil))
	r.Use(AuditServiceProvider(auditSvc))
	r.Use(rec.Middleware())
	registerAuditITRoutes(r)

	// 启动真实 HTTP server：httptest.NewServer 在 127.0.0.1 上监听随机端口，
	// 与生产 net/http.Server 一样走完整 TCP 网络栈（ClientIP 取真实来源地址）。
	srv := httptest.NewServer(r)

	e := &auditPGEnv{
		t:             t,
		db:            db,
		svc:           auditSvc,
		recorder:      rec,
		router:        r,
		server:        srv,
		baseURL:       srv.URL,
		client:        srv.Client(),
		internalToken: testInternalServiceToken,
	}
	t.Cleanup(func() {
		rec.Shutdown()
		srv.Close()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return e
}

// call 通过真实 TCP HTTP 调用接口（http.Client → httptest.Server），
// 与生产一致携带 X-Internal-Service-Token 认证头。
func (e *auditPGEnv) call(method, path, body string) *http.Response {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.baseURL+path, rd)
	if err != nil {
		e.t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", e.internalToken)
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("http call %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	return resp
}

// callUnauthenticated 模拟生产未认证请求：不带任何认证头，
// 预期被 Auth 中间件以 401 拦截。
func (e *auditPGEnv) callUnauthenticated(method, path string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(method, e.baseURL+path, nil)
	if err != nil {
		e.t.Fatalf("new request %s %s: %v", method, path, err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("http call %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	return resp
}

// rows 通过 service 查询 audit_logs 表全部行（最新在前）。
func (e *auditPGEnv) rows() []*types.AuditLog {
	e.t.Helper()
	rows, err := e.svc.List(context.Background(), &interfaces.AuditLogQuery{Limit: 1000})
	if err != nil {
		e.t.Fatalf("list audit rows: %v", err)
	}
	return rows
}

// waitRows 轮询等待异步 audit worker 写入至少 min 行。
func (e *auditPGEnv) waitRows(min int) []*types.AuditLog {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows := e.rows()
		if len(rows) >= min {
			return rows
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("timed out waiting for %d audit rows, got %d", min, len(rows))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// rawAuditRow 是 audit_logs 表的原生 SQL 反查结果。
// request_path / request_method 列已随迁移移除，方法与路径在 Details JSON 内。
type rawAuditRow struct {
	ID          uint64
	ActorUserID string
	ActorRole   string
	Action      string
	TargetType  string
	TargetID    string
	Outcome     string
	Details     string // details::text
	CreatedAt   time.Time
}

// rawRows 用原生 SQL（绕过 GORM）直接查询 PG 中的 audit_logs 表，
// 证明数据真实写入 PostgreSQL 而非内存/缓存。
func (e *auditPGEnv) rawRows() []rawAuditRow {
	e.t.Helper()
	sqlDB, err := e.db.DB()
	if err != nil {
		e.t.Fatalf("get sql db: %v", err)
	}
	rows, err := sqlDB.Query(`SELECT id, actor_user_id, actor_role, action, target_type, target_id,
		outcome, details::text, created_at
		FROM audit_logs ORDER BY id`)
	if err != nil {
		e.t.Fatalf("raw query audit_logs: %v", err)
	}
	defer rows.Close()

	var out []rawAuditRow
	for rows.Next() {
		var r rawAuditRow
		if err := rows.Scan(&r.ID, &r.ActorUserID, &r.ActorRole, &r.Action, &r.TargetType, &r.TargetID,
			&r.Outcome, &r.Details, &r.CreatedAt); err != nil {
			e.t.Fatalf("raw scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatalf("raw rows err: %v", err)
	}
	return out
}

// dumpRawRows 打印 PG 原生反查结果（模拟 psql 的 \x / 表格输出）。
func dumpRawRows(t *testing.T, title string, rows []rawAuditRow) {
	t.Helper()
	t.Logf("\n===== PG 原生 SQL 反查 audit_logs（%s，共 %d 行）=====", title, len(rows))
	for _, r := range rows {
		t.Logf("  id=%d action=%s outcome=%s actor=%s role=%s details=%s created_at=%s",
			r.ID, r.Action, r.Outcome, r.ActorUserID, r.ActorRole, r.Details,
			r.CreatedAt.Format("2006-01-02 15:04:05"))
	}
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestAuditPG_AllInterfacesWriteRows 遍历所有接口形态，验证每类接口都向
// PG 的 audit_logs 表写入 http.request 行，并用原生 SQL 反查确认落库。
func TestAuditPG_AllInterfacesWriteRows(t *testing.T) {
	e := newAuditPGEnv(t, true) // RecordGET=true，GET 也记录

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"GET 列表", http.MethodGet, "/api/v1/knowledge-bases", ""},
		{"GET 单条(路径参数)", http.MethodGet, "/api/v1/knowledge-bases/kb-1", ""},
		{"GET 嵌套(路径参数)", http.MethodGet, "/api/v1/knowledge-bases/kb-1/knowledge/k-1", ""},
		{"GET 系统管理", http.MethodGet, "/api/v1/system/admin/users", ""},
		{"POST 创建", http.MethodPost, "/api/v1/knowledge-bases", `{"name":"kb-1","embedding_model":"mock-embed"}`},
		{"POST 文件上传", http.MethodPost, "/api/v1/knowledge-bases/kb-1/knowledge/file", `{"title":"doc.pdf","file_size":1024}`},
		{"PUT 更新", http.MethodPut, "/api/v1/knowledge-bases/kb-1", `{"name":"kb-1-renamed"}`},
		{"PATCH 部分更新", http.MethodPatch, "/api/v1/knowledge-bases/kb-1", `{"name":"kb-2"}`},
		{"DELETE 删除", http.MethodDelete, "/api/v1/knowledge-bases/kb-1", ""},
		{"DELETE 批量", http.MethodDelete, "/api/v1/knowledge-bases/kb-1/knowledge", ""},
		{"DELETE 批量(session)", http.MethodDelete, "/api/v1/sessions/batch", `{"session_ids":["s1","s2"]}`},
		{"POST 服务端错误500", http.MethodPost, "/api/v1/system/admin/users", `{"email":"x@y.com"}`},
		{"POST 客户端错误400", http.MethodPost, "/api/v1/system/admin/users/ban", `{"email":"bad"}`},
		{"POST RBAC拒绝403", http.MethodPost, "/api/v1/system/admin/promote", `{"target_email":"u@x.com"}`},
	}

	for _, tc := range cases {
		resp := e.call(tc.method, tc.path, tc.body)
		t.Logf("请求 %-6s %-52s -> HTTP %d", tc.method, tc.path, resp.StatusCode)
	}

	// 14 个请求全部产生 http.request 行；RBAC 拒绝额外产生 1 行 access.denied。
	rows := e.waitRows(len(cases) + 1)
	dumpAuditRows(t, "PG audit_logs：全部接口调用后的写入数据（GORM service 视角）", rows)
	dumpRawRows(t, "PG audit_logs：原生 SQL 反查", e.rawRows())

	if len(rows) < len(cases) {
		t.Fatalf("expected at least %d rows, got %d", len(cases), len(rows))
	}
	httpRows := 0
	deniedRows := 0
	for _, r := range rows {
		switch r.Action {
		case types.AuditActionHTTPRequest:
			httpRows++
			if r.Outcome == "" {
				t.Errorf("http.request 行缺少 outcome: %+v", r)
			}
		case types.AuditActionAccessDenied:
			deniedRows++
		}
	}
	if httpRows != len(cases) {
		t.Errorf("expected %d http.request rows, got %d", len(cases), httpRows)
	}
	if deniedRows != 1 {
		t.Errorf("expected 1 access.denied row, got %d", deniedRows)
	}

	// 原生 SQL 反查结果与 service 视角行数一致，确认全部真实落库。
	raw := e.rawRows()
	if len(raw) != len(rows) {
		t.Errorf("raw SQL rows=%d != service rows=%d", len(raw), len(rows))
	}
}

// TestAuditPG_RequestBodyAndRouteTemplate 验证 request_body 捕获与脱敏、
// request_path 使用路由模板（而非真实资源 id）——在 PG 的 jsonb 中持久化。
func TestAuditPG_RequestBodyAndRouteTemplate(t *testing.T) {
	e := newAuditPGEnv(t, true)

	body := `{"name":"kb-1","secret":"shhh","password":"p@ss"}`
	e.call(http.MethodPost, "/api/v1/knowledge-bases/kb-12345/knowledge/file", body)

	rows := e.waitRows(1)
	dumpAuditRows(t, "PG audit_logs：POST body 捕获", rows)
	dumpRawRows(t, "PG audit_logs：body 捕获 raw SQL", e.rawRows())

	if got := auditDetailString(t, rows[0], "request_path"); got != "/api/v1/knowledge-bases/:id/knowledge/file" {
		t.Errorf("details.request_path should be route template, got %q", got)
	}
	details := string(rows[0].Details)
	if !strings.Contains(details, `"request_body"`) {
		t.Errorf("details should contain request_body, got %s", details)
	}
	// 敏感字段必须被脱敏（落库的 jsonb 也不能出现明文）
	if strings.Contains(details, "p@ss") || strings.Contains(details, "shhh") {
		t.Errorf("sensitive fields must be redacted in details: %s", details)
	}
}

// TestAuditPG_OutcomeSuccessDenied 验证成功/失败响应的 outcome 区分。
func TestAuditPG_OutcomeSuccessDenied(t *testing.T) {
	e := newAuditPGEnv(t, true)

	e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"ok"}`) // 200 -> success
	e.call(http.MethodPost, "/api/v1/system/admin/users/ban", `{"bad":1}`) // 400 -> denied
	e.call(http.MethodPost, "/api/v1/system/admin/users", `{"x":1}`)       // 500 -> denied

	rows := e.waitRows(3)
	dumpAuditRows(t, "PG audit_logs：outcome 区分", rows)

	byPath := map[string]string{}
	for _, r := range rows {
		if r.Action == types.AuditActionHTTPRequest {
			byPath[auditDetailString(t, r, "request_path")] = string(r.Outcome)
		}
	}
	if byPath["/api/v1/knowledge-bases"] != "success" {
		t.Errorf("200 should be outcome=success, got %q", byPath["/api/v1/knowledge-bases"])
	}
	if byPath["/api/v1/system/admin/users/ban"] != "denied" {
		t.Errorf("400 should be outcome=denied, got %q", byPath["/api/v1/system/admin/users/ban"])
	}
	if byPath["/api/v1/system/admin/users"] != "denied" {
		t.Errorf("500 should be outcome=denied, got %q", byPath["/api/v1/system/admin/users"])
	}
}

// TestAuditPG_RBACDeniedWritesAccessDenied 验证 RBAC 拒绝写入 access.denied 行。
// http.request 行的 actor 来自 InternalServiceAuth（internal-service）；
// access.denied 行由 handler 显式 LogDenied 写入（本测试 mock handler 传入
// mock-user-1 / system_admin）。
func TestAuditPG_RBACDeniedWritesAccessDenied(t *testing.T) {
	e := newAuditPGEnv(t, true)

	e.call(http.MethodPost, "/api/v1/system/admin/promote", `{"target_email":"u@x.com"}`)

	rows := e.waitRows(2)
	dumpAuditRows(t, "PG audit_logs：RBAC 拒绝", rows)

	var foundDenied bool
	for _, r := range rows {
		switch r.Action {
		case types.AuditActionAccessDenied:
			foundDenied = true
			if r.Outcome != types.AuditOutcomeDenied {
				t.Errorf("access.denied 行 outcome 应为 denied，got %q", r.Outcome)
			}
			if r.ActorUserID != "mock-user-1" {
				t.Errorf("access.denied 行 actor 应来自 handler 显式传入的 mock-user-1，got %q", r.ActorUserID)
			}
		case types.AuditActionHTTPRequest:
			// http.request 行 actor 来自 InternalServiceAuth 注入的身份。
			if r.ActorUserID != "internal-service" {
				t.Errorf("http.request 行 actor 应来自 InternalServiceAuth（internal-service），got %q", r.ActorUserID)
			}
		}
	}
	if !foundDenied {
		t.Fatalf("expected access.denied row, got: %+v", rows)
	}
}

// TestAuditPG_GETNotRecordedWhenDisabled 验证 RecordGET=false 时 GET 不产生行。
func TestAuditPG_GETNotRecordedWhenDisabled(t *testing.T) {
	e := newAuditPGEnv(t, false) // RecordGET=false（默认）

	resp := e.call(http.MethodGet, "/api/v1/knowledge-bases", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET should still work, got %d", resp.StatusCode)
	}
	e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"kb-1"}`)

	rows := e.waitRows(1)
	dumpAuditRows(t, "PG audit_logs：RecordGET=false 时 GET+POST", rows)

	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row (POST only), got %d", len(rows))
	}
	if got := auditDetailString(t, rows[0], "request_method"); got != http.MethodPost {
		t.Fatalf("expected only POST row, got method=%s path=%s", got, auditDetailString(t, rows[0], "request_path"))
	}
}

// ---------------------------------------------------------------------------
// 业务审计（BusinessAuditRecorder）
//
// 业务审计动作（如创建知识库、登录、授予权限）由生产业务 handler/service
// 在接口处理内部主动调用 recorder 写入，没有独立的公开 HTTP 接口可以触发，
// 因此这里直接调用 recorder 方法验证落库——这是本文件中唯一不经 HTTP 的部分，
// 其余全部通过真实接口调用。
// ---------------------------------------------------------------------------

// TestAuditPG_BusinessAuditWritesRows 将 12 种业务动作全部写入 PG。
func TestAuditPG_BusinessAuditWritesRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureAuditTestDB(t)

	db, err := gorm.Open(postgres.Open(auditPGTestConn(auditTestDBName).dsn()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&types.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec("DELETE FROM audit_logs").Error; err != nil {
		t.Fatalf("clean audit_logs: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	auditSvc := svc.NewAuditLogService(repo.NewAuditLogRepository(db))
	rec := svc.NewBusinessAuditRecorder(auditSvc)

	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "mock-admin")

	// 模拟各业务接口的审计调用（数据全部为 mock）
	rec.RecordKnowledgeBaseCreated(ctx, 1, "kb-1", 2, "mock-embed") // 创建知识库
	rec.RecordKnowledgeCreated(ctx, "k-1", "doc.pdf", "doc.pdf", "pdf", 1024, "1", "kb-1")
	rec.RecordKnowledgeUpdated(ctx, "k-1", "doc.pdf", "1", []string{"title"}, nil, map[string]interface{}{"title": "doc-v2.pdf"})
	rec.RecordKnowledgeDeleted(ctx, "k-1", "doc.pdf", "1", false)
	rec.RecordKnowledgePublished(ctx, "k-1", "doc.pdf", "1")
	rec.RecordLogin(ctx, "u-1", "mock@example.com", "password", "127.0.0.1")
	rec.RecordLoginFailed(ctx, "mock@example.com", "password", "bad_password", "127.0.0.1")
	rec.RecordLogout(ctx, "u-1", "mock@example.com")
	rec.RecordDomainAdminGranted(ctx, 1, "mock-domain", "u-2", "u2@example.com", "user2", false)
	rec.RecordPermissionGranted(ctx, "knowledge_base", "kb-1", "kb-1", "u-3", "u3@example.com", "manage")
	rec.RecordUserCreated(ctx, "u-4", "u4@example.com", "user4", "manual")
	rec.RecordKnowledgeDownloaded(ctx, "k-1", "doc.pdf", "doc.pdf", 2048, "1")

	rows, err := auditSvc.List(context.Background(), &interfaces.AuditLogQuery{Limit: 1000})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	dumpAuditRows(t, "PG audit_logs：业务审计写入数据", rows)

	// 原生 SQL 反查，确认真实落库
	env := &auditPGEnv{t: t, db: db, svc: auditSvc}
	dumpRawRows(t, "PG audit_logs：业务审计 raw SQL 反查", env.rawRows())

	if len(rows) != 12 {
		t.Fatalf("expected 12 business audit rows, got %d", len(rows))
	}
	actions := map[types.AuditAction]int{}
	for _, r := range rows {
		actions[r.Action]++
		// RecordLoginFailed 生产代码故意不记录 actor（匿名失败），其余行 actor 必须来自 context。
		if r.Action != types.AuditActionLoginFailed && r.ActorUserID != "mock-admin" {
			t.Errorf("业务审计 actor 应来自 context (action=%s): %q", r.Action, r.ActorUserID)
		}
	}
	expect := map[types.AuditAction]int{
		types.AuditActionKnowledgeBaseCreated: 1,
		types.AuditActionKnowledgeCreated:     1,
		types.AuditActionKnowledgeUpdated:     1,
		types.AuditActionKnowledgeDeleted:     1,
		types.AuditActionKnowledgePublished:   1,
		types.AuditActionLogin:                1,
		types.AuditActionLoginFailed:          1,
		types.AuditActionLogout:               1,
		types.AuditActionDomainAdminGranted:   1,
		types.AuditActionPermissionGranted:    1,
		types.AuditActionUserCreated:          1,
		types.AuditActionKnowledgeDownloaded:  1,
	}
	for act, want := range expect {
		if actions[act] != want {
			t.Errorf("action %s: expected %d rows, got %d", act, want, actions[act])
		}
	}

	// 原生反查同样应为 12 行
	raw := env.rawRows()
	if len(raw) != 12 {
		t.Errorf("raw SQL rows=%d, expected 12", len(raw))
	}
}

// ---------------------------------------------------------------------------
// 真实 TCP 网络 + 生产认证行为专项用例
// 这些用例证明测试确实走了真实 HTTP 接口调用（而非进程内 ServeHTTP），
// 且认证路径与生产完全一致（InternalServiceAuth / Auth 中间件真实执行）。
// ---------------------------------------------------------------------------

// TestAuditPG_HTTP_RealTCPNetwork 证明请求走真实 TCP 网络：
// server 监听 127.0.0.1 随机端口，审计行 details.client_ip 也是真实回环地址。
func TestAuditPG_HTTP_RealTCPNetwork(t *testing.T) {
	e := newAuditPGEnv(t, true)

	if !strings.HasPrefix(e.baseURL, "http://127.0.0.1:") {
		t.Fatalf("expected real TCP listener on 127.0.0.1, got %q", e.baseURL)
	}
	t.Logf("真实 HTTP server 监听地址: %s", e.baseURL)

	resp := e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"kb-1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST should reach handler, got %d", resp.StatusCode)
	}

	rows := e.waitRows(1)
	dumpRawRows(t, "PG audit_logs：真实 TCP 网络写入", e.rawRows())

	// GlobalAuditRecorder 记录的 client_ip 来自真实 TCP 连接来源地址，
	// 必须是 127.0.0.1 —— 进程内 ServeHTTP 不可能产生真实来源地址。
	// 注意 jsonb 落库的 key 与值之间带空格（"client_ip": "127.0.0.1"）。
	if !strings.Contains(string(rows[0].Details), `"client_ip": "127.0.0.1"`) {
		t.Errorf("client_ip 应为真实回环地址 127.0.0.1，got details=%s", string(rows[0].Details))
	}
	// 独立列 client_ip（varchar(64)）与 details.client_ip 一致。
	if rows[0].ClientIP != "127.0.0.1" {
		t.Errorf("client_ip 独立列应为 127.0.0.1，got %q", rows[0].ClientIP)
	}
}

// TestAuditPG_HTTP_InternalServiceActor 验证生产 InternalServiceAuth 中间件
// 注入的身份进入审计行（actor=internal-service, role=system_admin）。
func TestAuditPG_HTTP_InternalServiceActor(t *testing.T) {
	e := newAuditPGEnv(t, true)

	resp := e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"kb-1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("internal token request should be authenticated, got %d", resp.StatusCode)
	}

	rows := e.waitRows(1)
	dumpAuditRows(t, "PG audit_logs：内部服务 token 身份", rows)

	if rows[0].ActorUserID != "internal-service" {
		t.Errorf("actor 应来自 InternalServiceAuth 注入的 internal-service，got %q", rows[0].ActorUserID)
	}
	if rows[0].ActorRole != "system_admin" {
		t.Errorf("actor_role 应为 system_admin（IsSystemAdmin=true），got %q", rows[0].ActorRole)
	}
}

// TestAuditPG_HTTP_Unauthenticated401NoRow 验证生产 Auth 中间件对未认证请求
// 返回 401，且不写入任何审计行（与生产行为一致）。
func TestAuditPG_HTTP_Unauthenticated401NoRow(t *testing.T) {
	e := newAuditPGEnv(t, true)

	resp := e.callUnauthenticated(http.MethodGet, "/api/v1/knowledge-bases")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request should be 401, got %d", resp.StatusCode)
	}
	t.Logf("未认证 GET -> HTTP %d（Auth 中间件真实拦截）", resp.StatusCode)

	// Auth 401 Abort 后审计中间件不执行，等待确认无行写入。
	time.Sleep(400 * time.Millisecond)
	rows := e.rows()
	if len(rows) != 0 {
		dumpAuditRows(t, "PG audit_logs：未认证请求（应为空）", rows)
		t.Fatalf("unauthenticated request must NOT write audit rows, got %d", len(rows))
	}
	t.Log("未认证请求不写入 audit_logs：通过")
}

// TestAuditPG_HTTP_InvalidInternalToken 验证错误的内部 token 走 Auth 401 拦截，
// 且不产生审计行。
func TestAuditPG_HTTP_InvalidInternalToken(t *testing.T) {
	e := newAuditPGEnv(t, true)

	req, err := http.NewRequest(http.MethodGet, e.baseURL+"/api/v1/knowledge-bases", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Internal-Service-Token", "wrong-token")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("http call: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong internal token should be 401 (Auth 拦截), got %d", resp.StatusCode)
	}
	time.Sleep(400 * time.Millisecond)
	if rows := e.rows(); len(rows) != 0 {
		t.Fatalf("unauthenticated request must NOT write audit rows, got %d", len(rows))
	}
	t.Log("错误内部 token -> HTTP 401 且不写审计行：通过")
}
