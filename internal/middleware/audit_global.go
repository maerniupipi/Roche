package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

const (
	// auditGlobalChannelSize is the buffer capacity for async audit
	// entries. When the channel is full, incoming entries are dropped
	// (best-effort) — the dropped counter is exposed for monitoring.
	// 4096 slots means ~2 seconds of burst at 2000 RPS with 2 workers.
	auditGlobalChannelSize = 4096

	// auditGlobalBodyMaxSize caps the request body stored in the
	// details JSONB. Beyond this the body is truncated with a marker.
	auditGlobalBodyMaxSize = 8192

	// auditGlobalWorkers is the number of background goroutines
	// consuming from the channel and calling AuditLogService.Log().
	auditGlobalWorkers = 2
)

// auditGlobalEntry is the lightweight struct passed through the
// buffered async channel from the middleware hot path to the
// background persistence workers.
type auditGlobalEntry struct {
	ActorUserID string
	ActorName   string
	ActorRole   string
	Action      types.AuditAction
	Outcome     types.AuditOutcome
	StatusCode  int
	DurationMs  int64
	ClientIP    string
	Details     map[string]interface{}
	CreatedAt   time.Time
}

// GlobalAuditRecorder records every authenticated API request as an
// http.request audit row. It runs asynchronously — a buffered channel
// decouples the hot HTTP path from the database write, and a set of
// background goroutines consumes the channel. When the channel is full
// incoming entries are silently dropped; a counter is exposed for
// monitoring so operators can detect saturation.
//
// The recorder is nil-safe when disabled: NewGlobalAuditRecorder
// returns nil when AuditConfig.Global.Enabled == false, and the
// Middleware() method returns a no-op handler in that case.
type GlobalAuditRecorder struct {
	svc     interfaces.AuditLogService
	cfg     *config.GlobalAuditConfig
	ch      chan *auditGlobalEntry
	done    chan struct{}
	wg      sync.WaitGroup
	dropped atomic.Int64
}

// NewGlobalAuditRecorder constructs the recorder. When the audit
// service is absent or global audit is disabled in config, it returns
// a no-op recorder whose Middleware() and Shutdown() are transparent
// pass-throughs — this avoids optional-dependency complexity in dig.
func NewGlobalAuditRecorder(
	svc interfaces.AuditLogService,
	cfg *config.Config,
) *GlobalAuditRecorder {
	globalCfg := &config.GlobalAuditConfig{}
	if cfg != nil && cfg.Audit != nil && cfg.Audit.Global != nil {
		globalCfg = cfg.Audit.Global
	}

	r := &GlobalAuditRecorder{
		svc: svc,
		cfg: globalCfg,
	}

	if !globalCfg.Enabled || svc == nil {
		return r // no-op; Middleware() returns a pass-through handler
	}

	r.ch = make(chan *auditGlobalEntry, auditGlobalChannelSize)
	r.done = make(chan struct{})
	for i := 0; i < auditGlobalWorkers; i++ {
		r.wg.Add(1)
		go r.worker()
	}
	logger.Infof(context.Background(),
		"[audit-global] started: channel_cap=%d workers=%d capture_body=%v record_get=%v",
		auditGlobalChannelSize, auditGlobalWorkers, globalCfg.CaptureBody, globalCfg.RecordGET)
	return r
}

// worker drains entries from the channel and writes them to the
// database. AuditLogService.Log() already logs errors at ERROR level
// and never propagates them to the caller, so we simply discard the
// return value.
func (r *GlobalAuditRecorder) worker() {
	defer r.wg.Done()
	for {
		select {
		case entry := <-r.ch:
			if entry == nil {
				continue
			}
			r.persist(entry)
		case <-r.done:
			// Drain remaining entries before exiting so a graceful
			// shutdown doesn't lose the trailing window of audit rows.
			for {
				select {
				case entry := <-r.ch:
					if entry != nil {
						r.persist(entry)
					}
				default:
					return
				}
			}
		}
	}
}

// persist marshals the entry into an AuditLog and writes it via the
// service. Context.Background() is intentional — the HTTP request
// context may already be cancelled by the time the worker picks up
// the entry.
func (r *GlobalAuditRecorder) persist(entry *auditGlobalEntry) {
	detailBytes, err := json.Marshal(entry.Details)
	if err != nil {
		// Fallback: write a minimal valid JSON so the row is not lost.
		detailBytes = []byte(`{"error":"details_marshal_failed"}`)
	}
	auditLog := &types.AuditLog{
		ActorUserID: entry.ActorUserID,
		ActorName:   entry.ActorName,
		ActorRole:   entry.ActorRole,
		Action:      entry.Action,
		ClientIP:    entry.ClientIP,
		Outcome:     entry.Outcome,
		Details:     types.JSON(detailBytes),
		CreatedAt:   entry.CreatedAt,
	}
	// Best-effort: AuditLogService.Log already logs failures and never
	// returns errors that break the caller. We discard the return here
	// because the worker has no upstream to propagate to.
	_ = r.svc.Log(context.Background(), auditLog)
}

// Dropped returns the count of entries dropped because the channel
// was full. A non-zero value after sustained uptime suggests the
// channel size or worker count should be increased.
func (r *GlobalAuditRecorder) Dropped() int64 {
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}

// Shutdown signals all workers to stop and blocks until the last
// in-flight entry has been persisted. Call from the graceful-shutdown
// path so the final batch of audit rows is not lost on SIGTERM.
// When the recorder is a no-op (ch==nil), returns immediately.
func (r *GlobalAuditRecorder) Shutdown() {
	if r == nil || r.ch == nil {
		return
	}
	close(r.done)
	r.wg.Wait()
	if n := r.Dropped(); n > 0 {
		logger.Warnf(context.Background(),
			"[audit-global] shutdown complete: dropped=%d entries were lost (channel full)", n)
		return
	}
	logger.Infof(context.Background(), "[audit-global] shutdown complete")
}

// Middleware returns the gin middleware that records every
// authenticated API request. When the recorder is nil (disabled),
// the handler is a transparent pass-through no-op so the router
// doesn't need a conditional.
//
// Skip rules (no audit row emitted):
//   - /health, /swagger/, /assets/, /favicon.ico  (infra/internal paths)
//   - OPTIONS (CORS preflight)
//   - GET requests when RecordGET is false
//   - Unauthenticated endpoints listed in noAuthAPI
//   - Requests where the actor cannot be determined after Auth middleware
func (r *GlobalAuditRecorder) Middleware() gin.HandlerFunc {
	// No-op: global audit is disabled or the service is absent.
	if r.ch == nil {
		return func(c *gin.Context) { c.Next() }
	}

	skipPrefixes := []string{"/health", "/swagger/", "/assets/", "/favicon.ico"}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method

		// --- Skip rules (fast-path rejection) ---

		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(path, prefix) {
				c.Next()
				return
			}
		}
		if method == http.MethodOptions {
			c.Next()
			return
		}
		if method == http.MethodGet && !r.cfg.RecordGET {
			c.Next()
			return
		}
		if isNoAuthAPI(path, method) {
			c.Next()
			return
		}

		// --- Capture phase ---

		start := time.Now()

		// Snapshot request body before it's consumed by handlers.
		// Only for mutation methods where a body is expected.
		var reqBodyBytes []byte
		if r.cfg.CaptureBody && c.Request.Body != nil &&
			(method == http.MethodPost || method == http.MethodPut ||
				method == http.MethodPatch || method == http.MethodDelete) {
			reqBodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))
		}

		// Execute the handler chain.
		c.Next()

		// --- Record phase ---

		ctx := c.Request.Context()

		// Actor is always set by Auth middleware for authenticated paths.
		actorUserID, _ := types.UserIDFromContext(ctx)
		if actorUserID == "" {
			return
		}
		// 操作人 name：优先取 Auth 注入的 *types.User.Username；
		// 内部服务身份（internal-service）没有 username 时留空。
		actorName := ""
		if user, ok := types.UserFromContext(ctx); ok {
			actorName = user.Username
		}

		// Prefer the route template (e.g. /api/v1/employees/:id) over the
		// concrete URL (e.g. /api/v1/employees/abc-123) so audit rows group
		// by endpoint rather than by individual resource id. The pattern is
		// kept inside Details (操作详情) — there is no dedicated column.
		routePattern := c.FullPath()
		if routePattern == "" {
			routePattern = path
		}

		statusCode := c.Writer.Status()
		outcome := types.AuditOutcomeSuccess
		if statusCode >= 400 {
			outcome = types.AuditOutcomeDenied
		}

		// Derive a human-readable role label for the audit row.
		actorRole := "viewer"
		if types.IsSystemAdminFromContext(ctx) {
			actorRole = "system_admin"
		}

		// Build the details payload that lands in the JSONB column.
		details := map[string]interface{}{
			"request_path":   routePattern,
			"request_method": method,
			"status_code":    statusCode,
			"duration_ms":    time.Since(start).Milliseconds(),
			"client_ip":      c.ClientIP(),
		}

		if len(reqBodyBytes) > 0 {
			bodyStr := string(reqBodyBytes)
			if len(bodyStr) > auditGlobalBodyMaxSize {
				bodyStr = bodyStr[:auditGlobalBodyMaxSize] + "...[truncated]"
			}
			// Reuse the existing sensitive-field redaction from logger.go.
			details["request_body"] = sanitizeBody(bodyStr)
		}

		if method == http.MethodGet && r.cfg.RecordGET {
			if raw := c.Request.URL.RawQuery; raw != "" {
				details["query"] = secutils.SanitizeForLog(raw)
			}
		}

		entry := &auditGlobalEntry{
			ActorUserID: actorUserID,
			ActorName:   actorName,
			ActorRole:   actorRole,
			Action:      types.AuditActionHTTPRequest,
			Outcome:     outcome,
			StatusCode:  statusCode,
			DurationMs:  time.Since(start).Milliseconds(),
			ClientIP:    c.ClientIP(),
			Details:     details,
			CreatedAt:   start,
		}

		// Non-blocking send: if the channel buffer is full we drop rather
		// than add latency to the HTTP response. The dropped counter is
		// surfaced through the /health or metrics endpoint.
		select {
		case r.ch <- entry:
		default:
			r.dropped.Add(1)
		}
	}
}
