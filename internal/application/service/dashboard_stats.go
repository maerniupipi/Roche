// Package service: dashboard daily stats computation.
//
// DashboardStatsService pre-aggregates the three dashboard endpoints' inputs
// into the dashboard_daily_stats table once a day:
//
//   - The cron runner computes the previous UTC day at 00:00 UTC every day.
//   - On startup it backfills any missing days (capped, see maxBackfillDays)
//     so a freshly deployed instance has data immediately.
//
// The endpoints (handler -> service -> repository) read only from
// dashboard_daily_stats and never scan the raw messages/sessions/knowledges
// tables at request time.
//
// The aggregation SQL is written for both PostgreSQL (production) and SQLite
// (lite mode) dialects; the dialect is picked once per query.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
)

const (
	// dashboardStatsCronSpec runs every day at 00:00 UTC (seconds field included).
	dashboardStatsCronSpec = "0 0 0 * * *"
	// dashboardTopListLimit caps how many detail rows are stored per day for the
	// list-type overview metrics (top documents / top users / fallback questions).
	dashboardTopListLimit = 100
	// dashboardMaxBackfillDays caps the startup catch-up window so a cold start
	// cannot trigger an unbounded backfill over years of data.
	dashboardMaxBackfillDays = 90
)

// DashboardStatsService computes and persists daily dashboard aggregates.
type DashboardStatsService struct {
	db   *gorm.DB
	cron *cron.Cron

	mu      sync.Mutex
	started bool
	runLock sync.Mutex // serialises ComputeDay so overlapping runs never race
}

// NewDashboardStatsService creates the service. It does NOT start the cron —
// call Start in the application bootstrap.
func NewDashboardStatsService(db *gorm.DB) *DashboardStatsService {
	return &DashboardStatsService{
		db: db,
		cron: cron.New(cron.WithSeconds(), cron.WithChain(
			cron.Recover(cron.DefaultLogger),
		)),
	}
}

// Start registers the daily schedule and kicks off an async backfill of
// missing days. Idempotent.
func (s *DashboardStatsService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	if _, err := s.cron.AddFunc(dashboardStatsCronSpec, func() {
		// Use Background so a cancelled bootstrap ctx doesn't stop sweeps.
		s.runDaily(context.Background())
	}); err != nil {
		return err
	}
	s.cron.Start()
	s.started = true
	logger.Infof(ctx, "[DashboardStats] daily aggregation scheduled (%s)", dashboardStatsCronSpec)

	// Backfill runs async so a cold start with many missing days does not
	// block the rest of the application from coming up.
	go func() {
		bg := context.Background()
		if err := s.backfill(bg); err != nil {
			logger.Warnf(bg, "[DashboardStats] backfill failed: %v", err)
		}
	}()
	return nil
}

// Stop halts the cron and waits for in-flight runs to finish.
func (s *DashboardStatsService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	c := s.cron.Stop()
	<-c.Done()
	s.started = false
}

// runDaily is the cron entry point: compute the previous UTC day.
func (s *DashboardStatsService) runDaily(ctx context.Context) {
	day := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	if err := s.ComputeDay(ctx, day); err != nil {
		logger.Errorf(ctx, "[DashboardStats] daily aggregation for %s failed: %v",
			day.Format("2006-01-02"), err)
		return
	}
	logger.Infof(ctx, "[DashboardStats] daily aggregation for %s completed", day.Format("2006-01-02"))
}

// backfill computes every missing day from the earliest data date up to
// yesterday, bounded by dashboardMaxBackfillDays.
func (s *DashboardStatsService) backfill(ctx context.Context) error {
	first, err := s.firstDataDate(ctx)
	if err != nil {
		return fmt.Errorf("resolve earliest data date: %w", err)
	}
	first = first.Truncate(24 * time.Hour)
	yesterday := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	if first.After(yesterday) {
		return nil
	}
	if days := int(yesterday.Sub(first).Hours() / 24); days > dashboardMaxBackfillDays {
		first = yesterday.AddDate(0, 0, -(dashboardMaxBackfillDays - 1))
	}

	computed := 0
	for d := first; !d.After(yesterday); d = d.Add(24 * time.Hour) {
		done, err := s.hasDay(ctx, d)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		if err := s.ComputeDay(ctx, d); err != nil {
			return fmt.Errorf("compute day %s: %w", d.Format("2006-01-02"), err)
		}
		computed++
	}
	if computed > 0 {
		logger.Infof(ctx, "[DashboardStats] backfilled %d missing day(s)", computed)
	}
	return nil
}

// ComputeDay computes and upserts every dashboard_daily_stats row for the UTC
// day. Idempotent: rows are upserted on conflict, so re-running overwrites
// with fresh values. Exporting it lets ops trigger a manual recompute.
func (s *DashboardStatsService) ComputeDay(ctx context.Context, day time.Time) error {
	day = day.Truncate(24 * time.Hour)
	start := day
	end := day.Add(24 * time.Hour)
	dateKey := day.Format("2006-01-02")

	s.runLock.Lock()
	defer s.runLock.Unlock()

	isPostgres := s.db.Dialector.Name() == "postgres"

	// 1) Raw aggregations.
	chatByDomain, err := s.chatStatsByDomain(ctx, isPostgres, start, end)
	if err != nil {
		return fmt.Errorf("chat stats: %w", err)
	}
	domainDist, err := s.domainDistribution(ctx, isPostgres, start, end)
	if err != nil {
		return fmt.Errorf("domain distribution: %w", err)
	}
	kbGroups, err := s.knowledgeBaseStatsByGroup(ctx, isPostgres)
	if err != nil {
		return fmt.Errorf("knowledge base stats: %w", err)
	}
	satByDomain, err := s.satisfactionByDomain(ctx, isPostgres, start, end)
	if err != nil {
		return fmt.Errorf("satisfaction: %w", err)
	}

	// 2) Per-domain and per-KB aggregation shapes.
	chatByDomain = fillZeroDomains(chatByDomain, domainDist)

	globalTopDocs, err := s.topDocuments(ctx, isPostgres, start, end, 0)
	if err != nil {
		return fmt.Errorf("top documents (global): %w", err)
	}
	globalFeedback, err := s.productFeedback(ctx, start, end, 0)
	if err != nil {
		return fmt.Errorf("product feedback (global): %w", err)
	}
	globalTopUsers, err := s.topUsers(ctx, isPostgres, start, end, 0)
	if err != nil {
		return fmt.Errorf("top users (global): %w", err)
	}
	globalFallback, err := s.fallbackQuestions(ctx, isPostgres, start, end, 0)
	if err != nil {
		return fmt.Errorf("fallback questions (global): %w", err)
	}

	// 3) Build rows.
	rows := make([]*types.DashboardDailyStat, 0, 1+len(chatByDomain)+len(kbGroups))

	// Global row (d, 0, '').
	global := &types.DashboardDailyStat{
		StatDate: day,
	}
	for _, c := range chatByDomain {
		global.QuestionCount += c.QuestionCount
		global.UniqueUsers += c.UniqueUsers
		global.AnswerCount += c.AnswerCount
		global.TotalAgentDurationMs += c.TotalAgentDurationMs
		global.ValidAnswerCount += c.ValidAnswerCount
		global.FallbackAnswerCount += c.FallbackAnswerCount
	}
	for _, k := range kbGroups {
		global.PublishedCount += k.Published
		global.UploadSuccessCount += k.UploadSuccess
		global.UploadFailedCount += k.UploadFailed
		global.ScheduledPublishCount += k.ScheduledPublish
		global.UnpublishedCount += k.Unpublished
		global.ArchivedCount += k.Archived
	}
	global.SatisfactionPct = satisfactionPct(aggregateSatisfaction(satByDomain))
	global.DomainDistribution = mustJSON(domainDist)
	single, multi := deriveCrossDomain(domainDist)
	global.CrossDomainSingleCount = single
	global.CrossDomainMultiCount = multi
	global.TopDocuments = mustJSON(globalTopDocs)
	global.ProductFeedback = mustJSON(globalFeedback)
	global.TopUsers = mustJSON(globalTopUsers)
	global.FallbackQuestions = mustJSON(globalFallback)
	rows = append(rows, global)

	// Per-domain rows (d, domainID, '').
	//
	// A domain row must exist for every domain that either saw chat traffic on
	// this day (chatByDomain) or owns knowledge documents (kbGroups). The
	// kbGroups contribution is what makes document counters available per domain
	// on days with no questions — without it, knowledge-base-stats queries
	// filtered by knowledge_domain_id would find no row (and thus no document
	// counters) for domains that only hold documents.
	chatByID := make(map[uint64]chatStatsRow, len(chatByDomain))
	domainIDs := make(map[uint64]struct{}, len(chatByDomain)+len(kbGroups))
	for _, c := range chatByDomain {
		if c.KnowledgeDomainID == 0 {
			continue // no-domain messages are already part of the global row
		}
		chatByID[c.KnowledgeDomainID] = c
		domainIDs[c.KnowledgeDomainID] = struct{}{}
	}
	for _, k := range kbGroups {
		if k.KnowledgeDomainID != 0 {
			domainIDs[k.KnowledgeDomainID] = struct{}{}
		}
	}
	ordered := make([]uint64, 0, len(domainIDs))
	for id := range domainIDs {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	for _, domainID := range ordered {
		row := &types.DashboardDailyStat{
			StatDate:           day,
			KnowledgeDomainID:  domainID,
			DomainDistribution: types.JSON([]byte("[]")),
		}
		if c, ok := chatByID[domainID]; ok {
			row.QuestionCount = c.QuestionCount
			row.UniqueUsers = c.UniqueUsers
			row.AnswerCount = c.AnswerCount
			row.TotalAgentDurationMs = c.TotalAgentDurationMs
			row.ValidAnswerCount = c.ValidAnswerCount
			row.FallbackAnswerCount = c.FallbackAnswerCount
		}
		if sat, ok := satByDomain[domainID]; ok {
			row.SatisfactionPct = satisfactionPct(sat)
		}
		for _, k := range kbGroups {
			if k.KnowledgeDomainID == domainID {
				row.PublishedCount += k.Published
				row.UploadSuccessCount += k.UploadSuccess
				row.UploadFailedCount += k.UploadFailed
				row.ScheduledPublishCount += k.ScheduledPublish
				row.UnpublishedCount += k.Unpublished
				row.ArchivedCount += k.Archived
			}
		}
		docs, err := s.topDocuments(ctx, isPostgres, start, end, domainID)
		if err != nil {
			return fmt.Errorf("top documents (domain %d): %w", domainID, err)
		}
		fb, err := s.productFeedback(ctx, start, end, domainID)
		if err != nil {
			return fmt.Errorf("product feedback (domain %d): %w", domainID, err)
		}
		users, err := s.topUsers(ctx, isPostgres, start, end, domainID)
		if err != nil {
			return fmt.Errorf("top users (domain %d): %w", domainID, err)
		}
		fallback, err := s.fallbackQuestions(ctx, isPostgres, start, end, domainID)
		if err != nil {
			return fmt.Errorf("fallback questions (domain %d): %w", domainID, err)
		}
		row.TopDocuments = mustJSON(docs)
		row.ProductFeedback = mustJSON(fb)
		row.TopUsers = mustJSON(users)
		row.FallbackQuestions = mustJSON(fallback)
		rows = append(rows, row)
	}

	// Per-knowledge-base rows (d, 0, kbID). Document counters only; the
	// knowledge-base-stats endpoint reads these rows directly.
	kbRows := aggregateKBStats(kbGroups)
	for _, k := range kbRows {
		rows = append(rows, &types.DashboardDailyStat{
			StatDate:              day,
			KnowledgeBaseID:       k.KnowledgeBaseID,
			PublishedCount:        k.Published,
			UploadSuccessCount:    k.UploadSuccess,
			UploadFailedCount:     k.UploadFailed,
			ScheduledPublishCount: k.ScheduledPublish,
			UnpublishedCount:      k.Unpublished,
			ArchivedCount:         k.Archived,
			DomainDistribution:    types.JSON([]byte("[]")),
			TopDocuments:          types.JSON([]byte("[]")),
			ProductFeedback:       types.JSON([]byte("[]")),
			TopUsers:              types.JSON([]byte("[]")),
			FallbackQuestions:     types.JSON([]byte("[]")),
		})
	}

	// 4) Upsert all rows in one transaction.
	if len(rows) == 0 {
		logger.Infof(ctx, "[DashboardStats] day %s has no data to aggregate", dateKey)
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "stat_date"},
					{Name: "knowledge_domain_id"},
					{Name: "knowledge_base_id"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"published_count", "upload_success_count", "upload_failed_count",
					"scheduled_publish_count", "unpublished_count", "archived_count",
					"question_count", "unique_users", "satisfaction_pct",
					"answer_count", "total_agent_duration_ms",
					"valid_answer_count", "fallback_answer_count",
					"cross_domain_single_count", "cross_domain_multi_count",
					"domain_distribution", "top_documents", "product_feedback",
					"top_users", "fallback_questions",
					"updated_at",
				}),
			}).Create(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// hasDay reports whether any row already exists for the given UTC day.
func (s *DashboardStatsService) hasDay(ctx context.Context, day time.Time) (bool, error) {
	dateKey := day.Format("2006-01-02")
	var n int64
	q := s.db.WithContext(ctx).Model(&types.DashboardDailyStat{})
	if s.db.Dialector.Name() == "postgres" {
		q = q.Where("stat_date = ?", dateKey)
	} else {
		// SQLite stores datetimes with a time component; compare the date part.
		q = q.Where("date(stat_date) = ?", dateKey)
	}
	err := q.Count(&n).Error
	return n > 0, err
}

// firstDataDate returns the UTC date of the earliest message, or today when
// there are no messages yet. The MIN(created_at) value is read as a string and
// parsed client-side because Postgres and SQLite emit different timestamp
// formats through their drivers.
func (s *DashboardStatsService) firstDataDate(ctx context.Context) (time.Time, error) {
	// Scan into *string: MIN(created_at) is NULL when the messages table is
	// empty, and NULL -> string is rejected by the database/sql converter.
	var min *string
	if err := s.db.WithContext(ctx).Raw("SELECT MIN(created_at) FROM messages").Scan(&min).Error; err != nil {
		return time.Time{}, err
	}
	if min == nil || *min == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, *min); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse earliest message date %q", *min)
}

// --- raw aggregation queries -------------------------------------------------

type chatStatsRow struct {
	KnowledgeDomainID    uint64 `gorm:"column:knowledge_domain_id"`
	QuestionCount        int64  `gorm:"column:question_count"`
	UniqueUsers          int64  `gorm:"column:unique_users"`
	AnswerCount          int64  `gorm:"column:answer_count"`
	TotalAgentDurationMs int64  `gorm:"column:total_agent_duration_ms"`
	ValidAnswerCount     int64  `gorm:"column:valid_answer_count"`
	FallbackAnswerCount  int64  `gorm:"column:fallback_answer_count"`
}

// chatStatsByDomain aggregates question/answer counters grouped by the
// session's knowledge domain (via sessions.last_request_state).
func (s *DashboardStatsService) chatStatsByDomain(ctx context.Context, isPostgres bool, start, end time.Time) ([]chatStatsRow, error) {
	var sql string
	if isPostgres {
		sql = `
			SELECT
				COALESCE(NULLIF(s.last_request_state->>'knowledge_domain_id', ''), '0')::bigint AS knowledge_domain_id,
				COUNT(*) FILTER (WHERE m.role = 'user') AS question_count,
				COUNT(DISTINCT s.user_id) FILTER (WHERE m.role = 'user') AS unique_users,
				COUNT(*) FILTER (WHERE m.role = 'assistant') AS answer_count,
				COALESCE(SUM(m.agent_duration_ms) FILTER (WHERE m.role = 'assistant' AND m.agent_duration_ms > 0), 0) AS total_agent_duration_ms,
				COUNT(*) FILTER (WHERE m.role = 'assistant' AND m.is_fallback = false) AS valid_answer_count,
				COUNT(*) FILTER (WHERE m.role = 'assistant' AND m.is_fallback = true) AS fallback_answer_count
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?
			GROUP BY 1`
	} else {
		sql = `
			SELECT
				CAST(COALESCE(NULLIF(json_extract(s.last_request_state, '$.knowledge_domain_id'), ''), '0') AS INTEGER) AS knowledge_domain_id,
				SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END) AS question_count,
				COUNT(DISTINCT CASE WHEN m.role = 'user' THEN s.user_id END) AS unique_users,
				SUM(CASE WHEN m.role = 'assistant' THEN 1 ELSE 0 END) AS answer_count,
				COALESCE(SUM(CASE WHEN m.role = 'assistant' AND m.agent_duration_ms > 0 THEN m.agent_duration_ms ELSE 0 END), 0) AS total_agent_duration_ms,
				SUM(CASE WHEN m.role = 'assistant' AND m.is_fallback = 0 THEN 1 ELSE 0 END) AS valid_answer_count,
				SUM(CASE WHEN m.role = 'assistant' AND m.is_fallback = 1 THEN 1 ELSE 0 END) AS fallback_answer_count
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?
			GROUP BY 1`
	}
	var rows []chatStatsRow
	if err := s.db.WithContext(ctx).Raw(sql, start, end).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// fillZeroDomains merges the per-domain chat rows with the distribution list so
// that domains appearing only in one source get a complete row.
func fillZeroDomains(rows []chatStatsRow, dist []types.DashboardDomainDistributionDetail) []chatStatsRow {
	byID := make(map[uint64]int, len(rows))
	for i := range rows {
		byID[rows[i].KnowledgeDomainID] = i
	}
	out := rows
	for _, d := range dist {
		if _, ok := byID[d.KnowledgeDomainID]; ok || d.KnowledgeDomainID == 0 {
			continue
		}
		out = append(out, chatStatsRow{KnowledgeDomainID: d.KnowledgeDomainID})
		byID[d.KnowledgeDomainID] = len(out) - 1
	}
	return out
}

// domainDistribution groups user questions by knowledge domain name. This is a
// global metric (not domain-scoped), matching the endpoint behaviour.
func (s *DashboardStatsService) domainDistribution(ctx context.Context, isPostgres bool, start, end time.Time) ([]types.DashboardDomainDistributionDetail, error) {
	var sql string
	if isPostgres {
		sql = `
			SELECT
				COALESCE(kd.id, 0) AS knowledge_domain_id,
				COALESCE(kd.name, 'default') AS name,
				COUNT(*) AS value
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			LEFT JOIN knowledge_domains kd ON kd.id::text = COALESCE(s.last_request_state->>'knowledge_domain_id', '')
			WHERE m.role = 'user' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?
			GROUP BY kd.id, kd.name
			ORDER BY value DESC`
	} else {
		sql = `
			SELECT
				COALESCE(kd.id, 0) AS knowledge_domain_id,
				COALESCE(kd.name, 'default') AS name,
				COUNT(*) AS value
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			LEFT JOIN knowledge_domains kd ON CAST(kd.id AS TEXT) = COALESCE(json_extract(s.last_request_state, '$.knowledge_domain_id'), '')
			WHERE m.role = 'user' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?
			GROUP BY kd.id, kd.name
			ORDER BY value DESC`
	}
	var rows []types.DashboardDomainDistributionDetail
	if err := s.db.WithContext(ctx).Raw(sql, start, end).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type knowledgeGroupStats struct {
	KnowledgeBaseID    string
	KnowledgeDomainID  uint64
	Published          int64
	UploadSuccess      int64
	UploadFailed       int64
	ScheduledPublish   int64
	Unpublished        int64
	Archived           int64
}

// knowledgeBaseStatsByGroup counts knowledge lifecycle states grouped by
// (knowledge_base_id, knowledge_domain_id).
func (s *DashboardStatsService) knowledgeBaseStatsByGroup(ctx context.Context, isPostgres bool) ([]knowledgeGroupStats, error) {
	var sql string
	if isPostgres {
		sql = `
			SELECT
				knowledge_base_id,
				knowledge_domain_id,
				COUNT(*) FILTER (WHERE parse_status = 'completed' AND enable_status = 'enabled') AS published,
				COUNT(*) FILTER (WHERE parse_status = 'completed') AS upload_success,
				COUNT(*) FILTER (WHERE parse_status = 'failed') AS upload_failed,
				COUNT(*) FILTER (WHERE parse_status IN ('pending','processing','finalizing')) AS scheduled_publish,
				COUNT(*) FILTER (WHERE enable_status = 'disabled') AS unpublished,
				COUNT(*) FILTER (WHERE enable_status = 'archived') AS archived
			FROM knowledges
			WHERE deleted_at IS NULL
			GROUP BY knowledge_base_id, knowledge_domain_id`
	} else {
		sql = `
			SELECT
				knowledge_base_id,
				knowledge_domain_id,
				SUM(CASE WHEN parse_status = 'completed' AND enable_status = 'enabled' THEN 1 ELSE 0 END) AS published,
				SUM(CASE WHEN parse_status = 'completed' THEN 1 ELSE 0 END) AS upload_success,
				SUM(CASE WHEN parse_status = 'failed' THEN 1 ELSE 0 END) AS upload_failed,
				SUM(CASE WHEN parse_status IN ('pending','processing','finalizing') THEN 1 ELSE 0 END) AS scheduled_publish,
				SUM(CASE WHEN enable_status = 'disabled' THEN 1 ELSE 0 END) AS unpublished,
				SUM(CASE WHEN enable_status = 'archived' THEN 1 ELSE 0 END) AS archived
			FROM knowledges
			WHERE deleted_at IS NULL
			GROUP BY knowledge_base_id, knowledge_domain_id`
	}
	var rows []knowledgeGroupStats
	if err := s.db.WithContext(ctx).Raw(sql).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// aggregateKBStats collapses the (kb, domain) groups into one row per
// knowledge base for the KB-scoped daily rows.
func aggregateKBStats(groups []knowledgeGroupStats) []knowledgeGroupStats {
	byKB := make(map[string]int, len(groups))
	out := make([]knowledgeGroupStats, 0, len(groups))
	for _, g := range groups {
		i, ok := byKB[g.KnowledgeBaseID]
		if !ok {
			byKB[g.KnowledgeBaseID] = len(out)
			out = append(out, knowledgeGroupStats{KnowledgeBaseID: g.KnowledgeBaseID})
			i = len(out) - 1
		}
		out[i].Published += g.Published
		out[i].UploadSuccess += g.UploadSuccess
		out[i].UploadFailed += g.UploadFailed
		out[i].ScheduledPublish += g.ScheduledPublish
		out[i].Unpublished += g.Unpublished
		out[i].Archived += g.Archived
	}
	return out
}

type satisfactionRow struct {
	KnowledgeDomainID uint64 `gorm:"column:knowledge_domain_id"`
	Likes             int64  `gorm:"column:likes"`
	Total             int64  `gorm:"column:total"`
}

// satisfactionByDomain counts like/dislike feedback grouped by domain.
func (s *DashboardStatsService) satisfactionByDomain(ctx context.Context, isPostgres bool, start, end time.Time) (map[uint64]satisfactionRow, error) {
	var sql string
	if isPostgres {
		sql = `
			SELECT
				COALESCE(NULLIF(s.last_request_state->>'knowledge_domain_id', ''), '0')::bigint AS knowledge_domain_id,
				COUNT(*) FILTER (WHERE f.rating = 'like') AS likes,
				COUNT(*) AS total
			FROM message_feedbacks f
			JOIN messages m ON m.id = f.message_id
			JOIN sessions s ON s.id = m.session_id
			WHERE f.created_at >= ? AND f.created_at < ?
			GROUP BY 1`
	} else {
		sql = `
			SELECT
				CAST(COALESCE(NULLIF(json_extract(s.last_request_state, '$.knowledge_domain_id'), ''), '0') AS INTEGER) AS knowledge_domain_id,
				SUM(CASE WHEN f.rating = 'like' THEN 1 ELSE 0 END) AS likes,
				COUNT(*) AS total
			FROM message_feedbacks f
			JOIN messages m ON m.id = f.message_id
			JOIN sessions s ON s.id = m.session_id
			WHERE f.created_at >= ? AND f.created_at < ?
			GROUP BY 1`
	}
	var rows []satisfactionRow
	if err := s.db.WithContext(ctx).Raw(sql, start, end).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]satisfactionRow, len(rows))
	for _, r := range rows {
		out[r.KnowledgeDomainID] = r
	}
	return out, nil
}

// topDocuments lists documents ordered by citation hits, optionally filtered
// by the document's owning knowledge domain.
func (s *DashboardStatsService) topDocuments(ctx context.Context, isPostgres bool, start, end time.Time, knowledgeDomainID uint64) ([]types.DashboardTopDocumentDetail, error) {
	var sql string
	if isPostgres {
		sql = `
			SELECT
				ref.value->>'knowledge_id' AS knowledge_id,
				MAX(COALESCE(ref.value->>'title', k.title, '')) AS title,
				MAX(COALESCE(kb.name, '')) AS kb_name,
				COUNT(*) AS hit_count
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			CROSS JOIN LATERAL jsonb_array_elements(m.knowledge_references) AS ref
			LEFT JOIN knowledges k ON k.id = ref.value->>'knowledge_id'
			LEFT JOIN knowledge_bases kb ON kb.id = k.knowledge_base_id
			WHERE m.role = 'assistant' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?`
		if knowledgeDomainID > 0 {
			sql += ` AND k.knowledge_domain_id = ?`
		}
		sql += `
			GROUP BY ref.value->>'knowledge_id'
			ORDER BY hit_count DESC
			LIMIT ?`
	} else {
		sql = `
			SELECT
				json_extract(ref.value, '$.knowledge_id') AS knowledge_id,
				MAX(COALESCE(json_extract(ref.value, '$.title'), k.title, '')) AS title,
				MAX(COALESCE(kb.name, '')) AS kb_name,
				COUNT(*) AS hit_count
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			JOIN json_each(m.knowledge_references) AS ref
			LEFT JOIN knowledges k ON k.id = json_extract(ref.value, '$.knowledge_id')
			LEFT JOIN knowledge_bases kb ON kb.id = k.knowledge_base_id
			WHERE m.role = 'assistant' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.knowledge_references IS NOT NULL
			  AND m.created_at >= ? AND m.created_at < ?`
		if knowledgeDomainID > 0 {
			sql += ` AND k.knowledge_domain_id = ?`
		}
		sql += `
			GROUP BY json_extract(ref.value, '$.knowledge_id')
			ORDER BY hit_count DESC
			LIMIT ?`
	}
	args := []interface{}{start, end}
	if knowledgeDomainID > 0 {
		args = append(args, knowledgeDomainID)
	}
	args = append(args, dashboardTopListLimit)

	var rows []types.DashboardTopDocumentDetail
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// productFeedback groups feedback categories, optionally domain-scoped.
func (s *DashboardStatsService) productFeedback(ctx context.Context, start, end time.Time, knowledgeDomainID uint64) ([]types.DashboardFeedbackItem, error) {
	sql := `
		SELECT category, COUNT(*) AS count
		FROM dashboard_feedback
		WHERE created_at >= ? AND created_at < ?`
	args := []interface{}{start, end}
	if knowledgeDomainID > 0 {
		sql += ` AND knowledge_domain_id = ?`
		args = append(args, knowledgeDomainID)
	}
	sql += ` GROUP BY category ORDER BY count DESC LIMIT ?`
	args = append(args, dashboardTopListLimit)

	var rows []types.DashboardFeedbackItem
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// topUsers lists users ordered by question count, optionally domain-scoped via
// the session's knowledge domain.
func (s *DashboardStatsService) topUsers(ctx context.Context, isPostgres bool, start, end time.Time, knowledgeDomainID uint64) ([]types.DashboardTopUserDetail, error) {
	var sql string
	if isPostgres {
		sql = `
			SELECT COALESCE(u.username, s.user_id) AS user_name, COUNT(*) AS question_count
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			LEFT JOIN users u ON u.id = s.user_id
			WHERE m.role = 'user' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?`
		if knowledgeDomainID > 0 {
			sql += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		}
		sql += ` GROUP BY COALESCE(u.username, s.user_id) ORDER BY question_count DESC LIMIT ?`
	} else {
		sql = `
			SELECT COALESCE(u.username, s.user_id) AS user_name, COUNT(*) AS question_count
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			LEFT JOIN users u ON u.id = s.user_id
			WHERE m.role = 'user' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
			  AND m.created_at >= ? AND m.created_at < ?`
		if knowledgeDomainID > 0 {
			sql += ` AND json_extract(s.last_request_state, '$.knowledge_domain_id') = ?`
		}
		sql += ` GROUP BY COALESCE(u.username, s.user_id) ORDER BY question_count DESC LIMIT ?`
	}
	args := []interface{}{start, end}
	if knowledgeDomainID > 0 {
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}
	args = append(args, dashboardTopListLimit)

	var rows []types.DashboardTopUserDetail
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// fallbackQuestions lists user questions that triggered a fallback answer,
// optionally domain-scoped via the session's knowledge domain.
func (s *DashboardStatsService) fallbackQuestions(ctx context.Context, isPostgres bool, start, end time.Time, knowledgeDomainID uint64) ([]types.DashboardFallbackQuestionDetail, error) {
	fallbackFlag := "true"
	if !isPostgres {
		fallbackFlag = "1"
	}
	sql := `
		SELECT m.content, COUNT(*) AS count
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.role = 'user' AND m.deleted_at IS NULL AND s.deleted_at IS NULL
		  AND m.created_at >= ? AND m.created_at < ?`
	args := []interface{}{start, end}
	if knowledgeDomainID > 0 {
		if isPostgres {
			sql += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		} else {
			sql += ` AND json_extract(s.last_request_state, '$.knowledge_domain_id') = ?`
		}
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}
	sql += ` AND EXISTS (
			SELECT 1 FROM messages ans
			WHERE ans.session_id = m.session_id
			  AND ans.role = 'assistant'
			  AND ans.created_at > m.created_at
			  AND ans.is_fallback = ` + fallbackFlag + `
			  AND ans.deleted_at IS NULL
		)
		GROUP BY m.content
		ORDER BY count DESC
		LIMIT ?`
	args = append(args, dashboardTopListLimit)

	var rows []types.DashboardFallbackQuestionDetail
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// --- helpers -----------------------------------------------------------------

// deriveCrossDomain reproduces the single/multi split used by the overview
// endpoint: named domains count as "single", the default (unknown) bucket as
// "multi", with the historical swap when any named domain exists.
func deriveCrossDomain(dist []types.DashboardDomainDistributionDetail) (single, multi int64) {
	for _, item := range dist {
		if item.Name != "" && item.Name != "default" {
			multi += item.Value
		} else {
			single += item.Value
		}
	}
	if multi > 0 {
		single, multi = multi, single
	}
	return single, multi
}

func aggregateSatisfaction(rows map[uint64]satisfactionRow) satisfactionRow {
	var total satisfactionRow
	for _, r := range rows {
		total.Likes += r.Likes
		total.Total += r.Total
	}
	return total
}

// satisfactionPct returns the like ratio in percent (0 when no feedback).
func satisfactionPct(s satisfactionRow) float64 {
	if s.Total <= 0 {
		return 0
	}
	return float64(s.Likes) / float64(s.Total) * 100
}

func mustJSON(v interface{}) types.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		return types.JSON([]byte("[]"))
	}
	return types.JSON(b)
}
