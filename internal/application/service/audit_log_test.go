package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// stubAuditRepo collects Create calls and answers CountSinceForDedup
// from the in-memory state. Embeds the interface so any unstubbed
// method will nil-panic, surfacing a contract drift loudly instead of
// silently returning zero-values.
type stubAuditRepo struct {
	interfaces.AuditLogRepository

	mu      sync.Mutex
	created []*types.AuditLog
	// countErr lets a test inject a transient lookup failure.
	countErr error
}

func (s *stubAuditRepo) Create(_ context.Context, entry *types.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created = append(s.created, entry)
	return nil
}

func (s *stubAuditRepo) CountSinceForDedup(
	_ context.Context, actorUserID string,
	action types.AuditAction, since time.Time,
) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, e := range s.created {
		if e.ActorUserID == actorUserID &&
			e.Action == action &&
			!e.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

// fakeClock returns a controllable time source so the dedup-window
// tests can simulate "1 minute later" without sleeping.
type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time           { return f.t }
func (f *fakeClock) Advance(by time.Duration) { f.t = f.t.Add(by) }

func newSvcForTest() (*auditLogService, *stubAuditRepo, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)}
	repo := &stubAuditRepo{}
	svc := &auditLogService{repo: repo, now: clock.Now}
	return svc, repo, clock
}

func newDeniedCtx(t *testing.T, method, path string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, nil)
	return c
}

func TestAuditLog_Log_FillsCreatedAtAndOutcome(t *testing.T) {
	svc, repo, clock := newSvcForTest()
	entry := &types.AuditLog{
		Action: types.AuditActionAccessDenied,
		// CreatedAt left zero, Outcome left empty
	}
	if err := svc.Log(context.Background(), entry); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 written entry, got %d", len(repo.created))
	}
	if !entry.CreatedAt.Equal(clock.Now()) {
		t.Fatalf("expected CreatedAt to default to clock time, got %v", entry.CreatedAt)
	}
	if entry.Outcome != types.AuditOutcomeSuccess {
		t.Fatalf("expected Outcome to default to success, got %q", entry.Outcome)
	}
}

func TestAuditLog_Log_FillsClientIPFromContext(t *testing.T) {
	// 操作人发起请求的 IP 由 RequestID middleware 注入 ctx；调用方未显式
	// 提供时 Log 必须兜底取用，保证业务/系统审计事件也带上来源 IP。
	svc, repo, _ := newSvcForTest()
	ctx := types.WithClientIP(context.Background(), "203.0.113.9")
	if err := svc.Log(ctx, &types.AuditLog{Action: types.AuditActionSystemSettingChanged}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 written entry, got %d", len(repo.created))
	}
	if got := repo.created[0].ClientIP; got != "203.0.113.9" {
		t.Fatalf("expected ClientIP to fall back to ctx value, got %q", got)
	}
}

func TestAuditLog_Log_KeepsExplicitClientIP(t *testing.T) {
	// 调用方显式提供的 IP（如 RecordLogin 的 clientIP 参数）优先于 ctx。
	svc, repo, _ := newSvcForTest()
	ctx := types.WithClientIP(context.Background(), "203.0.113.9")
	if err := svc.Log(ctx, &types.AuditLog{
		Action:   types.AuditActionLogin,
		ClientIP: "198.51.100.7",
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if got := repo.created[0].ClientIP; got != "198.51.100.7" {
		t.Fatalf("expected explicit ClientIP to win, got %q", got)
	}
}

func TestAuditLog_Log_RejectsEmptyAction(t *testing.T) {
	// Schema requires action; the service guards the contract upfront so
	// callers get a clean error instead of a constraint violation later.
	svc, _, _ := newSvcForTest()
	err := svc.Log(context.Background(), &types.AuditLog{})
	if err == nil {
		t.Fatalf("expected error when Action is empty")
	}
}

func TestAuditLog_LogDenied_DedupesRepeatedRejectsWithinWindow(t *testing.T) {
	// First denied write hits the table; second within the window does
	// NOT — the dedup primitive is the headline durability guarantee
	// against probing clients filling the audit table at line rate.
	svc, repo, _ := newSvcForTest()
	c := newDeniedCtx(t, "PUT", "/api/v1/knowledge-domains/7")

	if err := svc.LogDenied(context.Background(), c, "u-viewer", "regular_user", "system_admin"); err != nil {
		t.Fatalf("LogDenied: %v", err)
	}
	if err := svc.LogDenied(context.Background(), c, "u-viewer", "regular_user", "system_admin"); err != nil {
		t.Fatalf("LogDenied second call: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected dedup to drop second write, got %d entries", len(repo.created))
	}
}

func TestAuditLog_LogDenied_StampsClientIP(t *testing.T) {
	// 访问.拒绝 是安全事件，来源 IP 是核心取证维度 —— 即使 ctx 没有注入
	// IP，LogDenied 也必须从 gin.Context 直接取 c.ClientIP()。
	svc, repo, _ := newSvcForTest()
	c := newDeniedCtx(t, "PUT", "/api/v1/knowledge-domains/7")
	// c.ClientIP() 对 httptest 请求返回 RemoteAddr 的 IP 部分。
	expected := c.ClientIP()
	if expected == "" {
		t.Fatal("expected a non-empty RemoteAddr-derived IP from httptest request")
	}

	if err := svc.LogDenied(context.Background(), c, "u-viewer", "regular_user", "system_admin"); err != nil {
		t.Fatalf("LogDenied: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 written entry, got %d", len(repo.created))
	}
	if got := repo.created[0].ClientIP; got != expected {
		t.Fatalf("expected LogDenied to stamp client IP %q, got %q", expected, got)
	}
}

func TestAuditLog_LogDenied_WritesAgainAfterWindowExpires(t *testing.T) {
	// The dedup is a sliding window, not a once-per-tuple lock — once
	// the trailing window is empty, the next denied call must record.
	svc, repo, clock := newSvcForTest()
	c := newDeniedCtx(t, "PUT", "/api/v1/knowledge-domains/7")

	_ = svc.LogDenied(context.Background(), c, "u-viewer", "regular_user", "system_admin")
	clock.Advance(denyDedupWindow + time.Second)
	_ = svc.LogDenied(context.Background(), c, "u-viewer", "regular_user", "system_admin")

	if len(repo.created) != 2 {
		t.Fatalf("expected window-expiry to allow second write, got %d entries", len(repo.created))
	}
}

func TestAuditLog_LogDenied_DedupIsPerActorAndAction(t *testing.T) {
	// The dedup key is (actor_user_id, action) — the same actor probing
	// two distinct endpoints within the window must be suppressed, while
	// a different actor hitting the same endpoint still records.
	svc, repo, _ := newSvcForTest()
	c1 := newDeniedCtx(t, "PUT", "/api/v1/knowledge-domains/7")
	c2 := newDeniedCtx(t, "PUT", "/api/v1/agents/abc")

	_ = svc.LogDenied(context.Background(), c1, "u-viewer-a", "regular_user", "system_admin")
	_ = svc.LogDenied(context.Background(), c1, "u-viewer-b", "regular_user", "system_admin")
	_ = svc.LogDenied(context.Background(), c2, "u-viewer-a", "regular_user", "system_admin")

	if len(repo.created) != 2 {
		t.Fatalf("expected 2 distinct (actor,action) writes, got %d", len(repo.created))
	}
}

func TestAuditLog_LogDenied_DegradesGracefullyOnDedupLookupError(t *testing.T) {
	// If the dedup count returns an error (DB hiccup, transient), we
	// must NOT silently skip the audit row. Better to write a
	// duplicate than to lose a denied event during incident response.
	svc, repo, _ := newSvcForTest()
	repo.countErr = errors.New("transient")
	c := newDeniedCtx(t, "PUT", "/api/v1/knowledge-domains/7")

	if err := svc.LogDenied(context.Background(), c, "u-viewer", "regular_user", "system_admin"); err != nil {
		t.Fatalf("LogDenied with dedup error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected fallthrough write on dedup error, got %d entries", len(repo.created))
	}
}
