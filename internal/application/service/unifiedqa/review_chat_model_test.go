package unifiedqa

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestBuildEvidenceReviewJSONSchemaUsesAgentProfile(t *testing.T) {
	catalog, err := NewAgentCatalog(testThreeAgentCatalogConfig(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	hr, ok := catalog.Get("hr")
	if !ok {
		t.Fatal("Get(hr) returned no profile")
	}

	schema, err := buildEvidenceReviewJSONSchema(hr, 0)
	if err != nil {
		t.Fatalf("buildEvidenceReviewJSONSchema() error = %v", err)
	}
	if !json.Valid(schema) {
		t.Fatalf("evidence review schema is not valid JSON: %s", schema)
	}
	got := string(schema)
	for _, required := range []string{
		`"enum":["hr"]`, `"enum":["knowledge_search"]`,
		`"required":["agent_id","status","facts","requires_scenario_selection","missing_requirements","conflicts"]`,
		`"required":["statement","quote","is_ambiguous","scenario","document_level","currency","citations"]`,
		`"facts":{"type":"array","maxItems":8`, `"citations":{"type":"array","minItems":1,"maxItems":3`,
		`"document_level"`, `"currency"`, `"requires_scenario_selection":{"type":"boolean"}`,
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("evidence review schema does not contain %q: %s", required, got)
		}
	}
	if evidenceReviewMaxCompletionTokens < 4000 {
		t.Fatalf("evidence review completion budget = %d, want at least 4000", evidenceReviewMaxCompletionTokens)
	}
}

func TestBuildEvidenceReviewJSONSchemaDisallowsRecoveryAfterFirstAttempt(t *testing.T) {
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
	schema, err := buildEvidenceReviewJSONSchema(profile, 1)
	if err != nil {
		t.Fatalf("buildEvidenceReviewJSONSchema() error = %v", err)
	}
	var decoded struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if got := string(decoded.Properties["recovery_request"]); got != `{"type":"null"}` {
		t.Fatalf("recovery_request schema = %s", got)
	}
}

func TestChatReviewModelUsesLargerCompletionBudgetAndCompactCandidates(t *testing.T) {
	chatModel := &fakeRouteChat{response: &types.ChatResponse{Content: `{"agent_id":"finance","status":"insufficient","facts":[],"missing_requirements":[],"conflicts":[]}`}}
	provider := &fakeRouteChatProvider{
		models: []*types.Model{{ID: "review-model", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive}},
		chat:   chatModel,
	}
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
	_, err := NewChatReviewModel(provider).GenerateReview(context.Background(), ReviewModelRequest{
		SystemPrompt: "prompt", Question: "Q", Profile: profile, ModelID: "review-model",
		Candidates: []EvidenceCandidate{{
			OpaqueID: "e_1", KnowledgeBaseID: "kb", KnowledgeID: "doc", ChunkID: "chunk",
			Title: "Policy", Description: strings.Repeat("description", 100), Content: "policy evidence",
			Score: 0.9, RetrievalScore: 0.8, RerankScore: 0.9,
		}},
	})
	if err != nil {
		t.Fatalf("GenerateReview() error = %v", err)
	}
	if chatModel.options == nil || chatModel.options.MaxCompletionTokens != evidenceReviewMaxCompletionTokens {
		t.Fatalf("chat options = %+v", chatModel.options)
	}
	var input struct {
		CoverageChecklist []string         `json:"coverage_checklist"`
		Candidates        []map[string]any `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(chatModel.messages[1].Content), &input); err != nil {
		t.Fatalf("decode review input: %v", err)
	}
	if len(input.Candidates) != 1 || input.Candidates[0]["opaque_id"] != "e_1" || input.Candidates[0]["content"] != "policy evidence" {
		t.Fatalf("compact candidates = %+v", input.Candidates)
	}
	if len(input.CoverageChecklist) == 0 || input.CoverageChecklist[0] != "Q" {
		t.Fatalf("coverage checklist = %v", input.CoverageChecklist)
	}
	for _, omitted := range []string{"knowledge_base_id", "knowledge_id", "chunk_id", "knowledge_description", "retrieval_score", "rerank_score"} {
		if _, exists := input.Candidates[0][omitted]; exists {
			t.Fatalf("compact candidate unexpectedly contains %q: %+v", omitted, input.Candidates[0])
		}
	}
}
