package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

func newSuggestedQuestionRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&types.HomepageSuggestedQuestion{}); err != nil {
		t.Fatalf("migrate suggested questions: %v", err)
	}
	return db
}

func TestSuggestedQuestionRepositoryReplacesAndOrdersAllRows(t *testing.T) {
	repo := NewSuggestedQuestionRepository(newSuggestedQuestionRepositoryTestDB(t))
	ctx := context.Background()
	first := []types.HomepageSuggestedQuestion{
		{ID: "1", Question: "Q3", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 3},
		{ID: "2", Question: "Q1", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 1},
		{ID: "3", Question: "Q2", AnswerMode: types.SuggestedQuestionAnswerCustom, CustomAnswer: "A2", SortOrder: 2},
	}
	if err := repo.ReplaceAll(ctx, first); err != nil {
		t.Fatalf("ReplaceAll(first): %v", err)
	}
	rows, err := repo.List(ctx)
	if err != nil || len(rows) != 3 || rows[0].SortOrder != 1 || rows[2].SortOrder != 3 {
		t.Fatalf("List() rows=%+v err=%v", rows, err)
	}
	row, err := repo.Get(ctx, "2")
	if err != nil || row == nil || row.Question != "Q1" {
		t.Fatalf("Get() row=%+v err=%v", row, err)
	}
	second := []types.HomepageSuggestedQuestion{
		{ID: "4", Question: "N1", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 1},
		{ID: "5", Question: "N2", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 2},
		{ID: "6", Question: "N3", AnswerMode: types.SuggestedQuestionAnswerGenerated, SortOrder: 3},
	}
	if err := repo.ReplaceAll(ctx, second); err != nil {
		t.Fatalf("ReplaceAll(second): %v", err)
	}
	rows, err = repo.List(ctx)
	if err != nil || len(rows) != 3 || rows[0].ID != "4" {
		t.Fatalf("List() after replace rows=%+v err=%v", rows, err)
	}
}
