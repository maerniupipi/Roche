package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

// TestDashboardStatsComputeDaySQLite exercises the full ComputeDay pipeline on
// an in-memory SQLite database: schema, aggregation SQL, row building and the
// upsert. This is the dialect used by lite mode, so it also guards the
// JSON/datetime handling that differs from PostgreSQL.
func TestDashboardStatsComputeDaySQLite(t *testing.T) {
	db := openTestSQLite(t)
	seedDashboardStatsData(t, db)

	svc := NewDashboardStatsService(db)
	day := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if err := svc.ComputeDay(context.Background(), day); err != nil {
		t.Fatalf("ComputeDay failed: %v", err)
	}

	rows := loadStatsRows(t, db, "2026-08-15")
	if len(rows) == 0 {
		t.Fatal("no dashboard_daily_stats rows written for 2026-08-15")
	}

	global := findRow(rows, 0, "")
	if global == nil {
		t.Fatal("global row (domain 0, kb '') missing")
	}
	if global.QuestionCount != 4 {
		t.Errorf("global question_count = %d, want 4", global.QuestionCount)
	}
	// Global unique_users is the SUM of per-domain distinct users (the
	// pre-aggregation approximation). u1 asks in both domain 1 (s1) and domain
	// 2 (s3), so it is double-counted: 2 (domain 1: u1,u2) + 1 (domain 2: u1)
	// + 1 (domain 0: u3) = 4, not the globally de-duplicated 3.
	if global.UniqueUsers != 4 {
		t.Errorf("global unique_users = %d, want 4 (per-domain sum, u1 double-counted)", global.UniqueUsers)
	}
	if global.ValidAnswerCount != 2 || global.FallbackAnswerCount != 2 {
		t.Errorf("global valid/fallback = %d/%d, want 2/2", global.ValidAnswerCount, global.FallbackAnswerCount)
	}
	if global.SatisfactionPct < 66 || global.SatisfactionPct > 67 {
		t.Errorf("global satisfaction_pct = %v, want ~66.67", global.SatisfactionPct)
	}

	var dist []types.DashboardDomainDistributionDetail
	if err := json.Unmarshal(global.DomainDistribution, &dist); err != nil {
		t.Fatalf("unmarshal domain_distribution: %v", err)
	}
	if len(dist) != 3 {
		t.Errorf("domain_distribution length = %d, want 3 (HR/Finance/default)", len(dist))
	}
	if global.CrossDomainSingleCount == 0 && global.CrossDomainMultiCount == 0 {
		t.Error("cross_domain counts both zero; expected named-domain split")
	}

	hr := findRow(rows, 1, "")
	if hr == nil {
		t.Fatal("domain 1 row missing")
	}
	if hr.QuestionCount != 2 || hr.UniqueUsers != 2 {
		t.Errorf("domain1 question/unique = %d/%d, want 2/2", hr.QuestionCount, hr.UniqueUsers)
	}
	if hr.ValidAnswerCount != 1 || hr.FallbackAnswerCount != 1 {
		t.Errorf("domain1 valid/fallback = %d/%d, want 1/1", hr.ValidAnswerCount, hr.FallbackAnswerCount)
	}
	var hrDocs []types.DashboardTopDocumentDetail
	if err := json.Unmarshal(hr.TopDocuments, &hrDocs); err != nil {
		t.Fatalf("unmarshal domain1 top_documents: %v", err)
	}
	if len(hrDocs) != 2 {
		t.Errorf("domain1 top_documents length = %d, want 2", len(hrDocs))
	}

	fin := findRow(rows, 2, "")
	if fin == nil {
		t.Fatal("domain 2 row missing")
	}
	if fin.QuestionCount != 1 {
		t.Errorf("domain2 question_count = %d, want 1", fin.QuestionCount)
	}

	kb1 := findRow(rows, 0, "kb-1")
	if kb1 == nil {
		t.Fatal("knowledge base kb-1 row missing")
	}
	if kb1.PublishedCount != 1 || kb1.UploadFailedCount != 1 || kb1.ArchivedCount != 1 {
		t.Errorf("kb-1 counters = published:%d failed:%d archived:%d, want 1/1/1",
			kb1.PublishedCount, kb1.UploadFailedCount, kb1.ArchivedCount)
	}

	// Idempotency: recomputing the same day must not duplicate rows.
	if err := svc.ComputeDay(context.Background(), day); err != nil {
		t.Fatalf("second ComputeDay failed: %v", err)
	}
	if rows2 := loadStatsRows(t, db, "2026-08-15"); len(rows2) != len(rows) {
		t.Errorf("row count after recompute = %d, want %d", len(rows2), len(rows))
	}

	// hasDay must report the day as computed.
	done, err := svc.hasDay(context.Background(), day)
	if err != nil {
		t.Fatalf("hasDay failed: %v", err)
	}
	if !done {
		t.Error("hasDay returned false after ComputeDay")
	}
}

// TestDashboardStatsDomainRowWithoutChatSQLite verifies that a knowledge
// domain owning documents but producing no chat traffic on the aggregated day
// still gets a per-domain row with its document counters populated. This is
// the regression guard for the domain-row fix: previously domain rows were
// only generated from chatByDomain (the messages table), so document-only
// domains disappeared from dashboard_daily_stats.
func TestDashboardStatsDomainRowWithoutChatSQLite(t *testing.T) {
	db := openTestSQLite(t)
	seedDashboardStatsData(t, db)

	// Domain 3 (IT) owns documents but has no messages on the aggregated day.
	mustExec(t, db, `INSERT INTO knowledge_domains (id, name) VALUES (3, 'IT')`)
	mustExec(t, db, `INSERT INTO knowledge_bases (id, name) VALUES ('kb-3', 'IT Docs')`)
	mustExec(t, db,
		`INSERT INTO knowledges (id, knowledge_domain_id, knowledge_base_id, parse_status, enable_status, title)
		 VALUES ('docE', 3, 'kb-3', 'completed', 'enabled', 'Doc E'),
		        ('docF', 3, 'kb-3', 'failed', 'enabled', 'Doc F')`)

	svc := NewDashboardStatsService(db)
	day := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if err := svc.ComputeDay(context.Background(), day); err != nil {
		t.Fatalf("ComputeDay failed: %v", err)
	}

	rows := loadStatsRows(t, db, "2026-08-15")
	it := findRow(rows, 3, "")
	if it == nil {
		t.Fatal("domain 3 row missing: domain with documents but no chat must still get a row")
	}
	if it.QuestionCount != 0 || it.AnswerCount != 0 || it.UniqueUsers != 0 {
		t.Errorf("domain3 chat counters = q:%d/u:%d/a:%d, want 0/0/0",
			it.QuestionCount, it.UniqueUsers, it.AnswerCount)
	}
	if it.PublishedCount != 1 || it.UploadFailedCount != 1 {
		t.Errorf("domain3 document counters = published:%d failed:%d, want 1/1",
			it.PublishedCount, it.UploadFailedCount)
	}

	// The global row must include the newly added documents (docB + docF failed).
	global := findRow(rows, 0, "")
	if global == nil {
		t.Fatal("global row missing")
	}
	if global.UploadFailedCount != 2 {
		t.Errorf("global upload_failed = %d, want 2 (docB + docF)", global.UploadFailedCount)
	}
}

func openTestSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE messages (
			id TEXT PRIMARY KEY, request_id TEXT, session_id TEXT, role TEXT,
			content TEXT, knowledge_references TEXT, is_fallback INTEGER,
			agent_duration_ms INTEGER, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY, last_request_state TEXT, user_id TEXT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE knowledge_domains (id INTEGER PRIMARY KEY, name TEXT, deleted_at DATETIME)`,
		`CREATE TABLE knowledges (
			id TEXT PRIMARY KEY, knowledge_domain_id INTEGER, knowledge_base_id TEXT,
			parse_status TEXT, enable_status TEXT, title TEXT, deleted_at DATETIME)`,
		`CREATE TABLE knowledge_bases (id TEXT PRIMARY KEY, name TEXT, deleted_at DATETIME)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT)`,
		`CREATE TABLE message_feedbacks (
			id TEXT PRIMARY KEY, message_id TEXT, rating TEXT, created_at DATETIME)`,
		`CREATE TABLE dashboard_feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT, knowledge_domain_id INTEGER,
			category TEXT, created_at DATETIME)`,
		`CREATE TABLE dashboard_daily_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stat_date TEXT NOT NULL,
			knowledge_domain_id INTEGER NOT NULL DEFAULT 0,
			knowledge_base_id TEXT NOT NULL DEFAULT '',
			published_count INTEGER NOT NULL DEFAULT 0,
			upload_success_count INTEGER NOT NULL DEFAULT 0,
			upload_failed_count INTEGER NOT NULL DEFAULT 0,
			scheduled_publish_count INTEGER NOT NULL DEFAULT 0,
			unpublished_count INTEGER NOT NULL DEFAULT 0,
			archived_count INTEGER NOT NULL DEFAULT 0,
			question_count INTEGER NOT NULL DEFAULT 0,
			unique_users INTEGER NOT NULL DEFAULT 0,
			satisfaction_pct REAL NOT NULL DEFAULT 0,
			answer_count INTEGER NOT NULL DEFAULT 0,
			total_agent_duration_ms INTEGER NOT NULL DEFAULT 0,
			valid_answer_count INTEGER NOT NULL DEFAULT 0,
			fallback_answer_count INTEGER NOT NULL DEFAULT 0,
			cross_domain_single_count INTEGER NOT NULL DEFAULT 0,
			cross_domain_multi_count INTEGER NOT NULL DEFAULT 0,
			domain_distribution TEXT NOT NULL DEFAULT '[]',
			top_documents TEXT NOT NULL DEFAULT '[]',
			product_feedback TEXT NOT NULL DEFAULT '[]',
			top_users TEXT NOT NULL DEFAULT '[]',
			fallback_questions TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT uq_dashboard_daily_stats UNIQUE (stat_date, knowledge_domain_id, knowledge_base_id))`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table failed: %v\nSQL: %s", err, stmt)
		}
	}
	return db
}

func seedDashboardStatsData(t *testing.T, db *gorm.DB) {
	t.Helper()
	d15 := "2026-08-15 10:00:00+00:00"
	d14 := "2026-08-14 10:00:00+00:00"

	mustExec(t, db,
		`INSERT INTO knowledge_domains (id, name) VALUES (1, 'HR'), (2, 'Finance')`)

	mustExec(t, db,
		`INSERT INTO users (id, username) VALUES ('u1', 'alice'), ('u2', 'bob'), ('u3', 'carol'), ('u4', 'dave')`)

	mustExec(t, db,
		`INSERT INTO knowledge_bases (id, name) VALUES ('kb-1', 'HR Docs'), ('kb-2', 'Finance Docs')`)

	mustExec(t, db,
		`INSERT INTO knowledges (id, knowledge_domain_id, knowledge_base_id, parse_status, enable_status, title)
		 VALUES ('docA', 1, 'kb-1', 'completed', 'enabled', 'Policy A'),
		        ('docB', 1, 'kb-1', 'failed', 'enabled', 'Policy B'),
		        ('docC', 2, 'kb-2', 'completed', 'disabled', 'Report C'),
		        ('docD', 1, 'kb-1', 'completed', 'archived', 'Old D')`)

	mustExec(t, db,
		`INSERT INTO sessions (id, last_request_state, user_id) VALUES
		 ('s1', '{"knowledge_domain_id": "1"}', 'u1'),
		 ('s2', '{"knowledge_domain_id": "1"}', 'u2'),
		 ('s3', '{"knowledge_domain_id": "2"}', 'u1'),
		 ('s4', NULL, 'u3'),
		 ('s5', '{"knowledge_domain_id": "1"}', 'u4')`)

	mustExec(t, db,
		`INSERT INTO messages (id, request_id, session_id, role, content, knowledge_references, is_fallback, agent_duration_ms, created_at) VALUES
		 ('m1', 'r1', 's1', 'user', 'q1 hr', '[]', 0, 0, '`+d15+`'),
		 ('m2', 'r2', 's1', 'assistant', 'a1 hr', '[{"knowledge_id":"docA","title":"Policy A"}]', 0, 1200, '`+d15+`'),
		 ('m3', 'r3', 's2', 'user', 'q2 hr', '[]', 0, 0, '`+d15+`'),
		 ('m4', 'r4', 's2', 'assistant', 'fallback hr', '[{"knowledge_id":"docA","title":"Policy A"},{"knowledge_id":"docB","title":"Policy B"}]', 1, 800, '`+d15+`'),
		 ('m5', 'r5', 's3', 'user', 'q3 fin', '[]', 0, 0, '`+d15+`'),
		 ('m6', 'r6', 's3', 'assistant', 'a3 fin', '[{"knowledge_id":"docC","title":"Report C"}]', 0, 500, '`+d15+`'),
		 ('m7', 'r7', 's4', 'user', 'q4 nodomain', '[]', 0, 0, '`+d15+`'),
		 ('m8', 'r8', 's4', 'assistant', 'fallback nodomain', '[]', 1, 300, '`+d15+`'),
		 ('m9', 'r9', 's5', 'user', 'q5 prev day', '[]', 0, 0, '`+d14+`'),
		 ('m10', 'r10', 's5', 'assistant', 'a5 prev day', '[]', 0, 600, '`+d14+`')`)

	mustExec(t, db,
		`INSERT INTO message_feedbacks (id, message_id, rating, created_at) VALUES
		 ('f1', 'm2', 'like', '`+d15+`'),
		 ('f2', 'm4', 'dislike', '`+d15+`'),
		 ('f3', 'm6', 'like', '`+d15+`')`)

	mustExec(t, db,
		`INSERT INTO dashboard_feedback (knowledge_domain_id, category, created_at) VALUES
		 (1, 'accuracy', '`+d15+`'),
		 (1, 'accuracy', '`+d15+`'),
		 (2, 'speed', '`+d15+`')`)
}

func loadStatsRows(t *testing.T, db *gorm.DB, dateKey string) []types.DashboardDailyStat {
	t.Helper()
	// stat_date is deliberately excluded: SQLite stores it as TEXT with a time
	// component that the driver cannot scan into a time.Time directly.
	cols := "id, knowledge_domain_id, knowledge_base_id, published_count, " +
		"upload_success_count, upload_failed_count, scheduled_publish_count, " +
		"unpublished_count, archived_count, question_count, unique_users, " +
		"satisfaction_pct, answer_count, total_agent_duration_ms, " +
		"valid_answer_count, fallback_answer_count, cross_domain_single_count, " +
		"cross_domain_multi_count, domain_distribution, top_documents, " +
		"product_feedback, top_users, fallback_questions"
	var rows []types.DashboardDailyStat
	if err := db.Model(&types.DashboardDailyStat{}).
		Select(cols).Where("date(stat_date) = ?", dateKey).Scan(&rows).Error; err != nil {
		t.Fatalf("load stats rows: %v", err)
	}
	return rows
}

func findRow(rows []types.DashboardDailyStat, domainID uint64, kbID string) *types.DashboardDailyStat {
	for i := range rows {
		if rows[i].KnowledgeDomainID == domainID && rows[i].KnowledgeBaseID == kbID {
			return &rows[i]
		}
	}
	return nil
}

func mustExec(t *testing.T, db *gorm.DB, sql string) {
	t.Helper()
	if err := db.Exec(sql).Error; err != nil {
		t.Fatalf("exec failed: %v\nSQL: %s", err, sql)
	}
}
