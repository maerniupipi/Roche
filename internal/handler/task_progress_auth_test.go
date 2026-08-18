package handler

import (
	"context"
	"net/http"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/utils"
)

func TestRequireTaskProgressKnowledgeDomain_RejectsCrossKnowledgeDomain(t *testing.T) {
	taskID := utils.GenerateTaskID("faq_import", 999, "kb-victim")
	ctx := context.WithValue(context.Background(), types.KnowledgeDomainIDContextKey, uint64(1))

	err := requireTaskProgressKnowledgeDomain(ctx, taskID)
	if err == nil {
		t.Fatal("expected cross-knowledgeDomain task to be rejected")
	}
}

func TestRequireTaskProgressKnowledgeDomain_AllowsOwnKnowledgeDomain(t *testing.T) {
	taskID := utils.GenerateTaskID("kb_clone", 1, "kb-source")
	ctx := context.WithValue(context.Background(), types.KnowledgeDomainIDContextKey, uint64(1))

	if err := requireTaskProgressKnowledgeDomain(ctx, taskID); err != nil {
		t.Fatalf("expected own-knowledgeDomain task to pass, got %v", err)
	}
}

func TestRequireTaskProgressKnowledgeDomain_InvalidTaskID(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.KnowledgeDomainIDContextKey, uint64(1))
	err := requireTaskProgressKnowledgeDomain(ctx, "not-a-task")
	if err == nil {
		t.Fatal("expected invalid task ID to fail")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected error message")
	}
	_ = http.StatusBadRequest
}
