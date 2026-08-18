package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// stubAuditService implements just AuditLogService.List for the
// handler tests. Embeds the interface so any other call panics —
// surfaces a contract drift loudly instead of silently working.
type stubAuditService struct {
	interfaces.AuditLogService
	list func(ctx context.Context, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error)
}

func (s *stubAuditService) List(
	ctx context.Context, q *interfaces.AuditLogQuery,
) ([]*types.AuditLog, error) {
	return s.list(ctx, q)
}

// newAuditHandlerTestRouter mounts the handler with the production
// ErrorHandler so c.Error renders the canonical envelope. The audit_logs
// table is flat (no knowledge_domain_id column), so the handler ignores
// the :id path param entirely — knowledgeDomain context is not required
// by the handler (PathKnowledgeDomainMatch is stripped from this layer
// because the tests focus on the handler's own surface).
func newAuditHandlerTestRouter(svc interfaces.AuditLogService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	h := NewAuditLogHandler(svc)
	r.GET("/knowledge-domains/:id/audit-log", h.ListKnowledgeDomainAuditLog)
	return r
}

func TestAuditLogHandler_ReturnsEnvelopeAndCursor(t *testing.T) {
	// Two rows: smallest id appears in the next_cursor field so the
	// frontend can re-request older pages without re-parsing the body.
	svc := &stubAuditService{
		list: func(_ context.Context, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			return []*types.AuditLog{
				{ID: 102, Action: types.AuditActionAccessDenied},
				{ID: 95, Action: types.AuditActionAccessDenied, Outcome: types.AuditOutcomeDenied},
			}, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-domains/7/audit-log", nil)
	newAuditHandlerTestRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var got auditLogListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success=true")
	}
	if len(got.Data) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got.Data))
	}
	if got.NextCursor != 95 {
		t.Fatalf("expected next_cursor to be the smallest id (95), got %d", got.NextCursor)
	}
}

func TestAuditLogHandler_PassesQueryFiltersThrough(t *testing.T) {
	// The handler must propagate after_id / limit / action / outcome /
	// actor exactly as the service expects them. A regression here
	// would silently drop a filter and over-return rows on the wire.
	svc := &stubAuditService{
		list: func(_ context.Context, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			if q.AfterID != 100 {
				t.Fatalf("expected after_id=100, got %d", q.AfterID)
			}
			if q.Limit != 25 {
				t.Fatalf("expected limit=25, got %d", q.Limit)
			}
			if q.Action != types.AuditActionAccessDenied {
				t.Fatalf("expected action=access_denied, got %q", q.Action)
			}
			if q.Outcome != types.AuditOutcomeDenied {
				t.Fatalf("expected outcome=denied, got %q", q.Outcome)
			}
			if q.ActorUserID != "u-probing" {
				t.Fatalf("expected actor=u-probing, got %q", q.ActorUserID)
			}
			return nil, nil
		},
	}
	w := httptest.NewRecorder()
	q := "after_id=100&limit=25&action=%E8%AE%BF%E9%97%AE.%E6%8B%92%E7%BB%9D&outcome=denied&actor=u-probing"
	req := httptest.NewRequest(http.MethodGet, "/knowledge-domains/7/audit-log?"+q, nil)
	newAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuditLogHandler_EmptyResultProducesZeroCursor(t *testing.T) {
	// next_cursor=0 is the documented "no more rows" signal; the frontend
	// stops paginating when it sees this, so a regression that returns
	// the previous cursor on an empty page would loop forever.
	svc := &stubAuditService{
		list: func(_ context.Context, _ *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			return nil, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-domains/7/audit-log", nil)
	newAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var got auditLogListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NextCursor != 0 {
		t.Fatalf("expected next_cursor=0 on empty page, got %d", got.NextCursor)
	}
}

func TestAuditLogHandler_NonNumericDomainIDStillServed(t *testing.T) {
	// The audit_logs table is flat — there is no knowledge_domain_id
	// column and the handler no longer parses/validates the :id path
	// param. A non-numeric id must still reach the service and succeed
	// rather than 400; route-level domain scoping (if any) is enforced
	// by PathKnowledgeDomainMatch upstream in the production wiring.
	svc := &stubAuditService{
		list: func(_ context.Context, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			return []*types.AuditLog{{ID: 7, Action: types.AuditActionAccessDenied}}, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-domains/not-a-number/audit-log", nil)
	newAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (handler no longer validates domain id), got %d body=%s", w.Code, w.Body.String())
	}
}

// ---- ListSystemAuditLog (platform / system-scope feed) ----------------
//
// Mirrors the per-knowledgeDomain suite above but covers the
// GET /system/admin/audit-log endpoint. The audit_logs table is flat —
// there is no knowledge_domain_id column any more — so this endpoint
// simply returns the platform-wide feed. The route group is guarded by
// SystemAdmin() at the production wiring layer; the test mounts the
// handler directly so it focuses on the handler's own contract.

func newSystemAuditHandlerTestRouter(svc interfaces.AuditLogService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	h := NewAuditLogHandler(svc)
	r.GET("/system/admin/audit-log", h.ListSystemAuditLog)
	return r
}

func TestSystemAuditLogHandler_ReturnsSystemFeed(t *testing.T) {
	// The audit_logs table is flat (no knowledge_domain_id column), so
	// ListSystemAuditLog forwards the query straight to the service and
	// returns the feed envelope with the cursor from the last row.
	called := false
	svc := &stubAuditService{
		list: func(_ context.Context, _ *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			called = true
			return []*types.AuditLog{
				{ID: 50, Action: types.AuditActionSystemSettingChanged},
				{ID: 42, Action: types.AuditActionSystemAdminPromoted},
			}, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/admin/audit-log", nil)
	newSystemAuditHandlerTestRouter(svc).ServeHTTP(w, req)

	if !called {
		t.Fatalf("expected AuditLogService.List to be invoked")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var got auditLogListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected success=true")
	}
	if len(got.Data) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got.Data))
	}
	if got.NextCursor != 42 {
		t.Fatalf("expected next_cursor to be the smallest id (42), got %d", got.NextCursor)
	}
}

func TestSystemAuditLogHandler_PassesQueryFiltersThrough(t *testing.T) {
	// Same propagation guarantee as the per-knowledgeDomain variant. We exercise
	// every filter the OpenAPI surface advertises so a typo in the
	// query-key list (e.g. dropping "outcome") is caught.
	svc := &stubAuditService{
		list: func(_ context.Context, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			if q.AfterID != 200 {
				t.Fatalf("expected after_id=200, got %d", q.AfterID)
			}
			if q.Limit != 10 {
				t.Fatalf("expected limit=10, got %d", q.Limit)
			}
			if q.Action != types.AuditActionSystemSettingChanged {
				t.Fatalf("expected action=system.setting_changed, got %q", q.Action)
			}
			if q.Outcome != types.AuditOutcomeSuccess {
				t.Fatalf("expected outcome=success, got %q", q.Outcome)
			}
			if q.ActorUserID != "u-admin-1" {
				t.Fatalf("expected actor=u-admin-1, got %q", q.ActorUserID)
			}
			return nil, nil
		},
	}
	w := httptest.NewRecorder()
	q := "after_id=200&limit=10&action=%E7%B3%BB%E7%BB%9F.%E8%AE%BE%E7%BD%AE%E5%8F%98%E6%9B%B4&outcome=success&actor=u-admin-1"
	req := httptest.NewRequest(http.MethodGet, "/system/admin/audit-log?"+q, nil)
	newSystemAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestSystemAuditLogHandler_EmptyResultProducesZeroCursor(t *testing.T) {
	// next_cursor=0 is the documented "no more rows" signal — the
	// platform drawer's infinite-scroll watcher stops paginating on
	// it. A regression returning the previous cursor on an empty page
	// would loop forever and pin the CPU.
	svc := &stubAuditService{
		list: func(_ context.Context, _ *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			return nil, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/admin/audit-log?after_id=10", nil)
	newSystemAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var got auditLogListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NextCursor != 0 {
		t.Fatalf("expected next_cursor=0 on empty page, got %d", got.NextCursor)
	}
	if len(got.Data) != 0 {
		t.Fatalf("expected zero entries, got %d", len(got.Data))
	}
}

func TestSystemAuditLogHandler_GarbageCursorAndLimitTolerated(t *testing.T) {
	// Tolerant parsing matches ListKnowledgeDomainAuditLog so a misconfigured
	// client doesn't hard-fail the first request. Garbage `after_id`
	// or non-positive `limit` are treated as "use defaults" (0 / 0)
	// rather than a 400 — the frontend would otherwise show a blank
	// drawer the moment a stale param ends up in the URL.
	var seenAfter uint64
	var seenLimit int
	svc := &stubAuditService{
		list: func(_ context.Context, q *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			seenAfter = q.AfterID
			seenLimit = q.Limit
			return nil, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/admin/audit-log?after_id=abc&limit=-1", nil)
	newSystemAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (tolerant parse), got %d body=%s", w.Code, w.Body.String())
	}
	if seenAfter != 0 {
		t.Fatalf("garbage after_id must collapse to 0, got %d", seenAfter)
	}
	if seenLimit != 0 {
		t.Fatalf("non-positive limit must collapse to 0 (repo default), got %d", seenLimit)
	}
}

func TestSystemAuditLogHandler_ServiceErrorReturns500(t *testing.T) {
	// AuditLogService.List failures bubble through as 500 via
	// errors.NewInternalServerError + middleware.ErrorHandler. The
	// frontend surfaces the message verbatim in the drawer's error
	// alert, so the body should still carry a non-empty string.
	svc := &stubAuditService{
		list: func(_ context.Context, _ *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
			return nil, fmt.Errorf("db: connection refused")
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/admin/audit-log", nil)
	newSystemAuditHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
	if w.Body.Len() == 0 {
		t.Fatalf("expected non-empty error body for drawer alert")
	}
}
