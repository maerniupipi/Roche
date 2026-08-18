package repository

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

func newAdminAnswerRecordTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, email TEXT, chinese_name TEXT, english_name TEXT)`,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, user_id TEXT, title TEXT, deleted_at DATETIME)`,
		`CREATE TABLE messages (
            id TEXT PRIMARY KEY, session_id TEXT, request_id TEXT, channel TEXT,
            role TEXT, content TEXT, knowledge_references TEXT, is_completed BOOLEAN, is_fallback BOOLEAN,
            created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
        )`,
		`CREATE TABLE message_feedbacks (
            id TEXT PRIMARY KEY, message_id TEXT, session_id TEXT, user_id TEXT,
            rating TEXT, reason TEXT, comment TEXT, created_at DATETIME, updated_at DATETIME
        )`,
		`CREATE TABLE knowledge_bases (id TEXT PRIMARY KEY, name TEXT, deleted_at DATETIME)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	return db
}

func seedAdminAnswerRecords(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	mustExec := func(query string, args ...interface{}) {
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	mustExec(`INSERT INTO users (id, username, email) VALUES ('u1', 'Helen', 'helen@example.com'), ('u2', 'Harold', 'harold@example.com')`)
	mustExec(`INSERT INTO sessions (id, user_id, title) VALUES ('s1', 'u1', '报销问答'), ('s2', 'u2', '合规问答')`)
	mustExec(`INSERT INTO knowledge_bases (id, name) VALUES ('kb1', 'DoA'), ('kb2', 'T&E')`)
	mustExec(`INSERT INTO messages (id, session_id, request_id, channel, role, content, knowledge_references, is_completed, is_fallback, created_at, updated_at)
		VALUES (?, 's1', 'r1', 'web', 'user', '费用如何报销？', '[]', 1, 0, ?, ?)`, "q1", now, now)
	mustExec(`INSERT INTO messages (id, session_id, request_id, channel, role, content, knowledge_references, is_completed, is_fallback, created_at, updated_at)
		VALUES (?, 's1', 'r1', 'web', 'assistant', '请按流程报销。', ?, 1, 1, ?, ?)`,
		"a1", `[{"knowledge_base_id":"kb2"},{"knowledge_base_id":"kb1"},{"knowledge_base_id":"kb1"}]`, now.Add(time.Second), now.Add(time.Second))
	mustExec(`INSERT INTO message_feedbacks (id, message_id, session_id, user_id, rating, reason, comment, created_at, updated_at)
        VALUES ('f1', 'a1', 's1', 'u1', 'dislike', 'other', '回答时间不对', ?, ?)`, now.Add(time.Minute), now.Add(time.Minute))
	mustExec(`INSERT INTO messages (id, session_id, request_id, channel, role, content, knowledge_references, is_completed, is_fallback, created_at, updated_at)
		VALUES ('q2', 's2', 'r2', 'app', 'user', '合规要求？', '[]', 1, 0, ?, ?)`, now.Add(2*time.Hour), now.Add(2*time.Hour))
	mustExec(`INSERT INTO messages (id, session_id, request_id, channel, role, content, knowledge_references, is_completed, is_fallback, created_at, updated_at)
		VALUES ('a2', 's2', 'r2', 'app', 'assistant', '请遵守政策。', '[]', 1, 0, ?, ?)`, now.Add(2*time.Hour+time.Second), now.Add(2*time.Hour+time.Second))
}

func TestAdminAnswerRecordRepositoryReturnsNamesAndFeedbackDetails(t *testing.T) {
	db := newAdminAnswerRecordTestDB(t)
	seedAdminAnswerRecords(t, db)
	records, total, err := NewAdminAnswerRecordRepository(db).Query(context.Background(), &types.AdminAnswerRecordQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if total != 2 || len(records) != 2 {
		t.Fatalf("total=%d records=%+v", total, records)
	}
	record := records[1]
	if record.Username != "Helen" || record.Question != "费用如何报销？" || record.Answer != "请按流程报销。" {
		t.Fatalf("record=%+v", record)
	}
	if len(record.KnowledgeBases) != 2 || record.KnowledgeBases[0] != "DoA" || record.KnowledgeBases[1] != "T&E" {
		t.Fatalf("knowledge_bases=%v", record.KnowledgeBases)
	}
	if record.Feedback == nil || record.Feedback.Rating != types.FeedbackRatingDislike ||
		record.Feedback.Reason != "other" || record.Feedback.ReasonZh == "" ||
		record.Feedback.ReasonEn == "" || record.Feedback.Comment != "回答时间不对" {
		t.Fatalf("feedback=%+v", record.Feedback)
	}
}

func TestAdminAnswerRecordRepositoryFiltersFeedbackUsernameAndChannel(t *testing.T) {
	db := newAdminAnswerRecordTestDB(t)
	seedAdminAnswerRecords(t, db)
	repo := NewAdminAnswerRecordRepository(db)
	tests := []struct {
		query types.AdminAnswerRecordQuery
		want  string
	}{
		{query: types.AdminAnswerRecordQuery{Feedback: "dislike"}, want: "Helen"},
		{query: types.AdminAnswerRecordQuery{Feedback: "none"}, want: "Harold"},
		{query: types.AdminAnswerRecordQuery{Username: "hele"}, want: "Helen"},
		{query: types.AdminAnswerRecordQuery{Channel: "app"}, want: "Harold"},
		{query: types.AdminAnswerRecordQuery{IsFallback: boolPointer(true)}, want: "Helen"},
		{query: types.AdminAnswerRecordQuery{IsFallback: boolPointer(false)}, want: "Harold"},
	}
	for _, test := range tests {
		test.query.Page, test.query.PageSize = 1, 20
		records, total, err := repo.Query(context.Background(), &test.query)
		if err != nil || total != 1 || len(records) != 1 || records[0].Username != test.want {
			t.Fatalf("query=%+v total=%d records=%+v err=%v", test.query, total, records, err)
		}
	}
}

func boolPointer(value bool) *bool { return &value }
