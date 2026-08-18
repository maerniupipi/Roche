package utils

import "testing"

func TestParseTaskID_CompoundTaskType(t *testing.T) {
	taskID := GenerateTaskID("faq_import", 42, "kb-abc-123")
	taskType, knowledgeDomainID, _, uuidPart, businessID, err := ParseTaskID(taskID)
	if err != nil {
		t.Fatalf("ParseTaskID failed: %v", err)
	}
	if taskType != "faq_import" {
		t.Fatalf("taskType = %q, want faq_import", taskType)
	}
	if knowledgeDomainID != 42 {
		t.Fatalf("knowledgeDomainID = %d, want 42", knowledgeDomainID)
	}
	if uuidPart == "" {
		t.Fatal("expected uuid part")
	}
	if businessID == "" {
		t.Fatal("expected business ID")
	}
}

func TestTaskKnowledgeDomainID(t *testing.T) {
	taskID := GenerateTaskID("kb_clone", 7, "source-kb")
	got, err := TaskKnowledgeDomainID(taskID)
	if err != nil {
		t.Fatalf("TaskKnowledgeDomainID failed: %v", err)
	}
	if got != 7 {
		t.Fatalf("TaskKnowledgeDomainID = %d, want 7", got)
	}
}
