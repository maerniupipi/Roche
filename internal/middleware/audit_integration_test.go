package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	repo "roche.local/knowledge-agent-platform/internal/application/repository"
	svc "roche.local/knowledge-agent-platform/internal/application/service"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// ---------------------------------------------------------------------------
// 审计日志集成测试
//
// 目标：验证"目前所有接口"调用后，数据库 audit_logs 表实际写入的数据。
// 与现有单测（仅用 stub service）不同，这里使用真实 sqlite 数据库 +
// 真实 AuditLogRepository + 真实 AuditLogService + 真实 GlobalAuditRecorder，
// 走完整的写入链路，最后 dump 出审计表内容供人工核对。
//
// 覆盖的接口形态（对应生产 router.go 中的全部接口类别）：
//   - GET    读接口（RecordGET 开关）
//   - POST   创建接口（捕获 request_body）
//   - PUT/PATCH 更新接口
//   - DELETE 删除接口
//   - 路径参数接口（audit 行 request_path 应为路由模板而非真实 id）
//   - 成功 2xx / 失败 4xx / 5xx（outcome 区分）
//   - RBAC 拒绝（access.denied 行）
//   - 未认证请求（不产生 audit 行）
// ---------------------------------------------------------------------------

// auditITEnv 组装真实审计链路 + 模拟生产中间件链的 gin router。
type auditITEnv struct {
	t        *testing.T
	db       *gorm.DB
	svc      interfaces.AuditLogService
	recorder *GlobalAuditRecorder
	router   *gin.Engine
}

// newAuditITEnv 构建测试环境。recordGET 对应 audit.global.record_get 配置。
func newAuditITEnv(t *testing.T, recordGET bool) *auditITEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "audit_it.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.AuditLog{}); err != nil {
		t.Fatalf("migrate audit_logs: %v", err)
	}

	cfg := &config.Config{
		Audit: &config.AuditConfig{
			Global: &config.GlobalAuditConfig{
				Enabled:     true,
				CaptureBody: true,
				RecordGET:   recordGET,
			},
		},
	}

	auditSvc := svc.NewAuditLogService(repo.NewAuditLogRepository(db))
	rec := NewGlobalAuditRecorder(auditSvc, cfg)

	// 模拟生产中间件链（router.go）：RequestID → Auth → AuditServiceProvider
	// → GlobalAuditRecorder。RequestID 负责把 操作人发起请求的 IP 注入 ctx，
	// 业务审计事件依赖它给记录盖章。
	r := gin.New()
	r.Use(RequestID())
	r.Use(mockAuditActor("mock-user-1", true))
	r.Use(AuditServiceProvider(auditSvc))
	r.Use(rec.Middleware())
	registerAuditITRoutes(r)

	e := &auditITEnv{t: t, db: db, svc: auditSvc, recorder: rec, router: r}
	t.Cleanup(func() {
		rec.Shutdown()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close() // 释放 sqlite 文件句柄（Windows 下必须显式关闭）
		}
	})
	return e
}

// mockAuditActor 模拟 Auth 中间件：把用户身份写入 request context。
func mockAuditActor(userID string, isSystemAdmin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, isSystemAdmin)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ---- mock handlers 模拟生产 handler 的三种响应结果 ----

func auditOK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func auditServerError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "mock internal error"})
}

func auditBadRequest(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "mock bad request"})
}

// auditDenied 模拟 RBAC 拒绝路径：先 LogDenied 再返回 403。
func auditDenied(c *gin.Context) {
	if s := AuditServiceFromContext(c); s != nil {
		_ = s.LogDenied(c.Request.Context(), c, "mock-user-1", "user", "system_admin")
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
}

// registerAuditITRoutes 注册代表生产所有接口形态的路由。
func registerAuditITRoutes(r *gin.Engine) {
	v1 := r.Group("/api/v1")
	// 读接口（GET）
	v1.GET("/knowledge-bases", auditOK)
	v1.GET("/knowledge-bases/:id", auditOK)
	v1.GET("/knowledge-bases/:id/knowledge/:kid", auditOK)
	v1.GET("/system/admin/users", auditOK)
	// 创建接口（POST）
	v1.POST("/knowledge-bases", auditOK)
	v1.POST("/knowledge-bases/:id/knowledge/file", auditOK)
	// 更新接口（PUT / PATCH）
	v1.PUT("/knowledge-bases/:id", auditOK)
	v1.PATCH("/knowledge-bases/:id", auditOK)
	// 删除接口（DELETE）
	v1.DELETE("/knowledge-bases/:id", auditOK)
	v1.DELETE("/knowledge-bases/:id/knowledge", auditOK)
	v1.DELETE("/sessions/batch", auditOK)
	// 失败场景
	v1.POST("/system/admin/users", auditServerError)
	v1.POST("/system/admin/users/ban", auditBadRequest)
	// RBAC 拒绝
	v1.POST("/system/admin/promote", auditDenied)
}

// call 发起一次 HTTP 请求。
func (e *auditITEnv) call(method, path, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// rows 查询 audit_logs 表全部行（最新在前）。
func (e *auditITEnv) rows() []*types.AuditLog {
	e.t.Helper()
	rows, err := e.svc.List(context.Background(), &interfaces.AuditLogQuery{Limit: 1000})
	if err != nil {
		e.t.Fatalf("list audit rows: %v", err)
	}
	return rows
}

// waitRows 轮询等待异步 audit worker 写入至少 min 行。
func (e *auditITEnv) waitRows(min int) []*types.AuditLog {
	e.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
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

// dumpAuditRows 打印 audit_logs 表内容（模拟 SELECT * FROM audit_logs 的输出）。
// request_path / request_method 列已移除，路径与方法信息在 Details JSON 内。
func dumpAuditRows(t *testing.T, title string, rows []*types.AuditLog) {
	t.Helper()
	t.Logf("\n===== %s（共 %d 行）=====", title, len(rows))
	t.Logf("%-5s %-24s %-16s %-16s %-8s %-8s %-12s %s",
		"ID", "ACTION", "ACTOR_NAME", "CLIENT_IP", "OUTCOME", "ACTOR", "ACTOR_ROLE", "DETAILS")
	for _, r := range rows {
		t.Logf("%-5d %-24s %-16s %-16s %-8s %-8s %-12s %s",
			r.ID, r.Action, r.ActorName, r.ClientIP,
			r.Outcome, r.ActorUserID, r.ActorRole, string(r.Details))
	}
}

// auditDetailString 从行的 Details JSONB 中读取字符串键 —— request_path /
// request_method 随列删除后已移入 Details，测试统一经此读取。
func auditDetailString(t *testing.T, row *types.AuditLog, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(row.Details, &m); err != nil {
		t.Fatalf("unmarshal details %q: %v", string(row.Details), err)
	}
	s, _ := m[key].(string)
	return s
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestAuditGlobal_AllInterfacesWriteRows 遍历所有接口形态，验证每类接口
// 都向 audit_logs 表写入 http.request 行，并 dump 出完整数据。
func TestAuditGlobal_AllInterfacesWriteRows(t *testing.T) {
	e := newAuditITEnv(t, true) // RecordGET=true，GET 也记录

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
		w := e.call(tc.method, tc.path, tc.body)
		t.Logf("请求 %-6s %-52s -> HTTP %d", tc.method, tc.path, w.Code)
	}

	// 14 个请求全部产生 http.request 行；RBAC 拒绝额外产生 1 行 access.denied。
	rows := e.waitRows(len(cases) + 1)
	dumpAuditRows(t, "audit_logs 表：全部接口调用后的写入数据", rows)

	if len(rows) < len(cases) {
		t.Fatalf("expected at least %d rows, got %d", len(cases), len(rows))
	}
	for _, r := range rows {
		if r.Action == types.AuditActionHTTPRequest && r.Outcome == "" {
			t.Errorf("http.request 行缺少 outcome: %+v", r)
		}
		// 操作人发起请求的 IP：全局请求审计必须记录来源地址（由 RequestID
		// 注入 / audit_global 直接取 c.ClientIP()）。
		if r.Action == types.AuditActionHTTPRequest && r.ClientIP == "" {
			t.Errorf("http.request 行缺少 client_ip: %+v", r)
		}
	}
}

// TestAuditGlobal_GETNotRecordedWhenDisabled 验证 RecordGET=false 时 GET 不产生行。
func TestAuditGlobal_GETNotRecordedWhenDisabled(t *testing.T) {
	e := newAuditITEnv(t, false) // RecordGET=false（默认）

	w := e.call(http.MethodGet, "/api/v1/knowledge-bases", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET should still work, got %d", w.Code)
	}
	e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"kb-1"}`)

	rows := e.waitRows(1)
	dumpAuditRows(t, "audit_logs 表：RecordGET=false 时 GET+POST", rows)

	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row (POST only), got %d", len(rows))
	}
	if got := auditDetailString(t, rows[0], "request_method"); got != http.MethodPost {
		t.Fatalf("expected only POST row, got method=%s path=%s", got, auditDetailString(t, rows[0], "request_path"))
	}
}

// TestAuditGlobal_UnauthenticatedNotRecorded 验证无用户身份时（401 场景）不产生 audit 行。
func TestAuditGlobal_UnauthenticatedNotRecorded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "audit_unauth.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	auditSvc := svc.NewAuditLogService(repo.NewAuditLogRepository(db))
	cfg := &config.Config{
		Audit: &config.AuditConfig{
			Global: &config.GlobalAuditConfig{Enabled: true, CaptureBody: true, RecordGET: true},
		},
	}
	rec := NewGlobalAuditRecorder(auditSvc, cfg)
	t.Cleanup(func() {
		rec.Shutdown()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	// 无 mockAuditActor：模拟未认证请求
	r := gin.New()
	r.Use(AuditServiceProvider(auditSvc))
	r.Use(rec.Middleware())
	registerAuditITRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("request should reach handler, got %d", w.Code)
	}

	// 等待一小段时间确认没有行写入（未认证请求必须被跳过）
	time.Sleep(300 * time.Millisecond)
	rows, _ := auditSvc.List(context.Background(), &interfaces.AuditLogQuery{Limit: 100})
	if len(rows) != 0 {
		dumpAuditRows(t, "audit_logs 表：未认证请求（应为空）", rows)
		t.Fatalf("unauthenticated request must NOT write audit rows, got %d", len(rows))
	}
	t.Log("未认证请求不写入 audit_logs：通过")
}

// TestAuditGlobal_RequestBodyAndRouteTemplate 验证 request_body 捕获与
// request_path 使用路由模板（而非真实资源 id）。
func TestAuditGlobal_RequestBodyAndRouteTemplate(t *testing.T) {
	e := newAuditITEnv(t, true)

	body := `{"name":"kb-1","secret":"shhh","password":"p@ss"}`
	e.call(http.MethodPost, "/api/v1/knowledge-bases/kb-12345/knowledge/file", body)

	rows := e.waitRows(1)
	dumpAuditRows(t, "audit_logs 表：POST body 捕获", rows)

	if got := auditDetailString(t, rows[0], "request_path"); got != "/api/v1/knowledge-bases/:id/knowledge/file" {
		t.Errorf("details.request_path should be route template, got %q", got)
	}
	if !strings.Contains(string(rows[0].Details), `"request_body"`) {
		t.Errorf("details should contain request_body, got %s", string(rows[0].Details))
	}
	// 敏感字段必须被脱敏
	if strings.Contains(string(rows[0].Details), "p@ss") || strings.Contains(string(rows[0].Details), "shhh") {
		t.Errorf("sensitive fields must be redacted in details: %s", string(rows[0].Details))
	}
}

// TestAuditGlobal_OutcomeSuccessDenied 验证成功/失败响应的 outcome 区分。
func TestAuditGlobal_OutcomeSuccessDenied(t *testing.T) {
	e := newAuditITEnv(t, true)

	e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"ok"}`) // 200 -> success
	e.call(http.MethodPost, "/api/v1/system/admin/users/ban", `{"bad":1}`) // 400 -> denied
	e.call(http.MethodPost, "/api/v1/system/admin/users", `{"x":1}`)       // 500 -> denied

	rows := e.waitRows(3)
	dumpAuditRows(t, "audit_logs 表：outcome 区分", rows)

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

// TestAuditGlobal_RBACDeniedWritesAccessDenied 验证 RBAC 拒绝写入 access.denied 行。
func TestAuditGlobal_RBACDeniedWritesAccessDenied(t *testing.T) {
	e := newAuditITEnv(t, true)

	e.call(http.MethodPost, "/api/v1/system/admin/promote", `{"target_email":"u@x.com"}`)

	// LogDenied 是同步写入；http.request 是异步。等 2 行。
	rows := e.waitRows(2)
	dumpAuditRows(t, "audit_logs 表：RBAC 拒绝", rows)

	var foundDenied bool
	for _, r := range rows {
		if r.Action == types.AuditActionAccessDenied {
			foundDenied = true
			if r.Outcome != types.AuditOutcomeDenied {
				t.Errorf("access.denied 行 outcome 应为 denied，got %q", r.Outcome)
			}
			if r.ActorUserID != "mock-user-1" {
				t.Errorf("actor 应为 mock-user-1，got %q", r.ActorUserID)
			}
		}
	}
	if !foundDenied {
		t.Fatalf("expected access.denied row, got: %+v", rows)
	}
}

// TestAuditGlobal_SystemAdminActorRole 验证系统管理员请求的 actor_role。
func TestAuditGlobal_SystemAdminActorRole(t *testing.T) {
	e := newAuditITEnv(t, true)

	e.call(http.MethodPost, "/api/v1/knowledge-bases", `{"name":"kb"}`)
	rows := e.waitRows(1)

	for _, r := range rows {
		if r.Action == types.AuditActionHTTPRequest && r.ActorRole != "system_admin" {
			t.Errorf("system admin 请求 actor_role 应为 system_admin，got %q", r.ActorRole)
		}
	}
	dumpAuditRows(t, "audit_logs 表：系统管理员 actor_role", rows)
}

// ---------------------------------------------------------------------------
// 业务审计（BusinessAuditRecorder）：直接调用各业务审计方法，验证 audit_logs
// 写入的数据内容。对应生产代码中 handler/service 主动调用的审计点。
// ---------------------------------------------------------------------------

func TestBusinessAuditRecorder_WritesDurableRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "audit_biz.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
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
	rec.RecordKnowledgeBaseCreated(ctx, 1, "kb-1", 2, "mock-embed")          // 创建知识库
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
	dumpAuditRows(t, "audit_logs 表：业务审计写入数据", rows)

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
	// 校验每个动作都有对应行
	expect := map[types.AuditAction]int{
		types.AuditActionKnowledgeBaseCreated:   1,
		types.AuditActionKnowledgeCreated:       1,
		types.AuditActionKnowledgeUpdated:       1,
		types.AuditActionKnowledgeDeleted:       1,
		types.AuditActionKnowledgePublished:     1,
		types.AuditActionLogin:                  1,
		types.AuditActionLoginFailed:            1,
		types.AuditActionLogout:                 1,
		types.AuditActionDomainAdminGranted:     1,
		types.AuditActionPermissionGranted:      1,
		types.AuditActionUserCreated:            1,
		types.AuditActionKnowledgeDownloaded:    1,
	}
	for act, want := range expect {
		if actions[act] != want {
			t.Errorf("action %s: expected %d rows, got %d", act, want, actions[act])
		}
	}
}
