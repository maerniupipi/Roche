package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// auditLogService is the high-level wrapper around AuditLogRepository.
// It owns:
//   - timestamp defaulting (Log fills CreatedAt if zero so callers
//     don't have to).
//   - the 1-minute sliding-window dedup that LogDenied applies to keep
//     a probing client from filling the audit_logs table.
//
// The service is intentionally nil-tolerant when consumed: the
// knowledge_domain_admin service holds it as an optional field, so a future
// container reshuffle that constructs knowledge_domain_admin before audit_log
// won't crash. Callers should still aim to inject a real instance —
// the nil path is a degraded mode, not a default.
type auditLogService struct {
	repo interfaces.AuditLogRepository
	now  func() time.Time
}

// NewAuditLogService constructs the production service.
func NewAuditLogService(repo interfaces.AuditLogRepository) interfaces.AuditLogService {
	return &auditLogService{repo: repo, now: time.Now}
}

// denyDedupWindow caps how often LogDenied will write a durable row
// for the same (actor_user_id, action) tuple. 1 minute is long enough
// that a single hammered endpoint at 100 RPS produces 1 row/minute,
// not 6000, and short enough that a probing client alternating actions
// still leaves a trace (one row per action per minute).
const denyDedupWindow = 1 * time.Minute

// Log is the canonical write path. The repo's Create defaults
// CreatedAt at the SQL level, but we also fill it here so tests and
// callers that read entry.CreatedAt right after Log() get a sensible
// value without round-tripping through the database.
func (s *auditLogService) Log(ctx context.Context, entry *types.AuditLog) error {
	if entry == nil {
		return fmt.Errorf("audit log: nil entry")
	}
	if entry.Action == "" {
		return fmt.Errorf("audit log: action is required")
	}
	if entry.Outcome == "" {
		entry.Outcome = types.AuditOutcomeSuccess
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now()
	}
	// 操作人发起请求的 IP：调用方未显式提供时从 ctx 兜底（RequestID
	// middleware 注入），保证业务审计/系统审计事件也带上 IP。
	if entry.ClientIP == "" {
		entry.ClientIP = types.ClientIPFromContext(ctx)
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		// Log loudly but do NOT propagate the error to the caller in
		// the production wiring (see callers in knowledge_domain_admin service
		// and rbac middleware — both ignore the return). Audit failure
		// must never break the underlying business operation.
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"audit_action": entry.Action,
			"audit_target": entry.TargetID,
		})
		return err
	}
	return nil
}

// LogDenied records a middleware-level rejection. Subject to a
// 1-minute sliding-window dedup keyed by (actor_user_id, action) so a
// probing client cannot flood the table.
//
// The non-durable advisory log line `[rbac] role insufficient: ...`
// continues to fire on every reject (in middleware/rbac.go) — the
// dedup only suppresses durable writes, not stderr observability.
func (s *auditLogService) LogDenied(
	ctx context.Context,
	c *gin.Context,
	actorUserID, actorRole string,
	requiredRole string,
) error {
	rawPath := ""
	requestMethod := ""
	routePattern := ""
	if c != nil && c.Request != nil {
		rawPath = c.Request.URL.Path
		requestMethod = c.Request.Method
		// Prefer the route TEMPLATE (gin's c.FullPath, e.g.
		// "/api/v1/knowledge-bases/:id") over the raw URL so details stay
		// compact for an attacker iterating UUIDs; the raw URL is kept
		// alongside for forensics ("which resource was probed").
		routePattern = c.FullPath()
		if routePattern == "" {
			// No matched route (e.g. 404 path that still hit the
			// middleware via a catch-all). Fall back to the raw path so
			// the detail remains non-empty for the audit reader.
			routePattern = rawPath
		}
	}

	// Dedup probe: skip the durable write if this (actor, action) tuple
	// already has a row in the trailing window. Failure here is
	// non-fatal — degraded behaviour is "write a duplicate" which is
	// preferable to "skip the audit because the count failed".
	since := s.now().Add(-denyDedupWindow)
	if n, err := s.repo.CountSinceForDedup(
		ctx, actorUserID, types.AuditActionAccessDenied, since,
	); err == nil && n > 0 {
		return nil
	}

	detailsMap := map[string]string{
		"required_role": requiredRole,
		"request_path":  routePattern,
	}
	if requestMethod != "" {
		detailsMap["request_method"] = requestMethod
	}
	if rawPath != "" && rawPath != routePattern {
		detailsMap["raw_path"] = rawPath
	}
	details, _ := json.Marshal(detailsMap)
	entry := &types.AuditLog{
		ActorUserID: actorUserID,
		ActorRole:   actorRole,
		Action:      types.AuditActionAccessDenied,
		Outcome:     types.AuditOutcomeDenied,
		Details:     types.JSON(details),
	}
	// 拒绝事件同样记录 操作人发起请求的 IP —— 访问.拒绝 是安全事件，
	// 来源 IP 是核心取证维度（优先取 gin 直连地址，context 兜底）。
	if c != nil {
		entry.ClientIP = c.ClientIP()
	}
	return s.Log(ctx, entry)
}

// List proxies to the repository.
func (s *auditLogService) List(
	ctx context.Context,
	q *interfaces.AuditLogQuery,
) ([]*types.AuditLog, error) {
	return s.repo.List(ctx, q)
}

// Purge deletes rows whose created_at is strictly older than
// `retentionDays` ago. retentionDays <= 0 short-circuits — operators
// who configured no retention pay zero database round-trips.
//
// The cutoff is computed off the service's clock (s.now) so tests
// can drive deterministic horizons without touching wall time.
//
// We intentionally do NOT batch the DELETE: at the volumes audit_logs
// realistically reaches in a 24h window, a single DELETE-with-index
// finishes in well under a second on Postgres. If the table ever
// grows large enough that a single sweep blocks vacuum, the repo
// helper is the place to add LIMIT-style chunking — the service stays
// simple.
func (s *auditLogService) Purge(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := s.now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	return s.repo.DeleteOlderThan(ctx, cutoff)
}
