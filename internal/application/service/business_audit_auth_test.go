package service

import (
	"context"
	"encoding/json"
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

// newRecorderForTest builds a BusinessAuditRecorder backed by the
// shared stubAuditRepo, so all written audit entries are inspectable.
func newRecorderForTest() (*BusinessAuditRecorder, *stubAuditRepo) {
	svc, repo, _ := newSvcForTest()
	return NewBusinessAuditRecorder(svc), repo
}

// assertActionEqual is a test helper that fetches the last recorded
// audit entry and compares its Action field to the expected value.
func assertActionEqual(t *testing.T, created []*types.AuditLog, idx int, expected types.AuditAction) {
	t.Helper()
	if idx >= len(created) {
		t.Fatalf("expected at least %d audit entries, got %d", idx+1, len(created))
	}
	if created[idx].Action != expected {
		t.Fatalf("entry[%d].Action = %q, want %q", idx, created[idx].Action, expected)
	}
}

func TestBusinessAuditRecorder_RecordLogin(t *testing.T) {
	rec, repo := newRecorderForTest()
	ctx := context.Background()

	rec.RecordLogin(ctx, "user-001", "alice@example.com", "email_password", "192.168.1.1")

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
	}
	e := repo.created[0]
	assertActionEqual(t, repo.created, 0, types.AuditActionLogin)
	if e.Outcome != types.AuditOutcomeSuccess {
		t.Fatalf("expected Outcome=success, got %q", e.Outcome)
	}
	if e.TargetType != "user" {
		t.Fatalf("expected TargetType=user, got %q", e.TargetType)
	}
	if e.TargetID != "user-001" {
		t.Fatalf("expected TargetID=user-001, got %q", e.TargetID)
	}
	// KnowledgeDomainID column removed

	// Verify Details payload
	var details map[string]interface{}
	if err := json.Unmarshal(e.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal Details: %v", err)
	}
	if details["email"] != "alice@example.com" {
		t.Fatalf("expected email=alice@example.com, got %v", details["email"])
	}
	if details["method"] != "email_password" {
		t.Fatalf("expected method=email_password, got %v", details["method"])
	}
	if details["client_ip"] != "192.168.1.1" {
		t.Fatalf("expected client_ip=192.168.1.1, got %v", details["client_ip"])
	}
}

func TestBusinessAuditRecorder_RecordLogin_OIDC(t *testing.T) {
	rec, repo := newRecorderForTest()
	rec.RecordLogin(context.Background(), "user-002", "bob@example.com", "oidc", "")

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
	}
	var details map[string]interface{}
	_ = json.Unmarshal(repo.created[0].Details, &details)
	if details["method"] != "oidc" {
		t.Fatalf("expected method=oidc, got %v", details["method"])
	}
}

func TestBusinessAuditRecorder_RecordLogin_SAML(t *testing.T) {
	rec, repo := newRecorderForTest()
	rec.RecordLogin(context.Background(), "user-003", "carol@example.com", "saml", "")

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
	}
	var details map[string]interface{}
	_ = json.Unmarshal(repo.created[0].Details, &details)
	if details["method"] != "saml" {
		t.Fatalf("expected method=saml, got %v", details["method"])
	}
}

func TestBusinessAuditRecorder_RecordLoginFailed(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{"wrong_password", "wrong_password"},
		{"user_not_found", "user_not_found"},
		{"account_disabled", "account_disabled"},
		{"account_banned", "account_banned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, repo := newRecorderForTest()
			rec.RecordLoginFailed(context.Background(), "fail@example.com", "email_password", tt.reason, "10.0.0.1")

			if len(repo.created) != 1 {
				t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
			}
			e := repo.created[0]
			assertActionEqual(t, repo.created, 0, types.AuditActionLoginFailed)
			if e.Outcome != types.AuditOutcomeDenied {
				t.Fatalf("expected Outcome=denied, got %q", e.Outcome)
			}
			if e.ActorUserID != "" {
				t.Fatalf("expected empty ActorUserID for failed login, got %q", e.ActorUserID)
			}
			if e.TargetID != "" {
				t.Fatalf("expected empty TargetID for failed login, got %q", e.TargetID)
			}

			var details map[string]interface{}
			_ = json.Unmarshal(e.Details, &details)
			if details["email"] != "fail@example.com" {
				t.Fatalf("expected email=fail@example.com, got %v", details["email"])
			}
			if details["reason"] != tt.reason {
				t.Fatalf("expected reason=%s, got %v", tt.reason, details["reason"])
			}
		})
	}
}

func TestBusinessAuditRecorder_RecordLogout(t *testing.T) {
	rec, repo := newRecorderForTest()

	rec.RecordLogout(context.Background(), "user-001", "alice@example.com")

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
	}
	e := repo.created[0]
	assertActionEqual(t, repo.created, 0, types.AuditActionLogout)
	if e.Outcome != types.AuditOutcomeSuccess {
		t.Fatalf("expected Outcome=success, got %q", e.Outcome)
	}
	if e.TargetType != "user" {
		t.Fatalf("expected TargetType=user, got %q", e.TargetType)
	}

	var details map[string]interface{}
	_ = json.Unmarshal(e.Details, &details)
	if details["user_id"] != "user-001" {
		t.Fatalf("expected user_id=user-001, got %v", details["user_id"])
	}
}

func TestBusinessAuditRecorder_RecordKnowledgeDownloaded(t *testing.T) {
	rec, repo := newRecorderForTest()

	rec.RecordKnowledgeDownloaded(
		context.Background(),
		"kg-abc-123",        // knowledgeID
		"产品手册 v2.1.pdf",    // title
		"product_manual.pdf", // fileName
		2048576,             // fileSize
		"42",                // knowledgeBaseID
	)

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
	}
	e := repo.created[0]
	assertActionEqual(t, repo.created, 0, types.AuditActionKnowledgeDownloaded)
	if e.Outcome != types.AuditOutcomeSuccess {
		t.Fatalf("expected Outcome=success, got %q", e.Outcome)
	}
	if e.TargetType != "knowledge" {
		t.Fatalf("expected TargetType=knowledge, got %q", e.TargetType)
	}
	if e.TargetID != "kg-abc-123" {
		t.Fatalf("expected TargetID=kg-abc-123, got %q", e.TargetID)
	}

	// Verify all detail fields
	var details map[string]interface{}
	if err := json.Unmarshal(e.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal Details: %v", err)
	}
	if details["knowledge_id"] != "kg-abc-123" {
		t.Fatalf("expected knowledge_id=kg-abc-123, got %v", details["knowledge_id"])
	}
	if details["title"] != "产品手册 v2.1.pdf" {
		t.Fatalf("expected title=产品手册 v2.1.pdf, got %v", details["title"])
	}
	if details["file_name"] != "product_manual.pdf" {
		t.Fatalf("expected file_name=product_manual.pdf, got %v", details["file_name"])
	}
	if details["knowledge_base_id"] != "42" {
		t.Fatalf("expected knowledge_base_id=42, got %v", details["knowledge_base_id"])
	}
	// file_size comes as float64 from JSON unmarshal
	fs, ok := details["file_size"].(float64)
	if !ok || int64(fs) != 2048576 {
		t.Fatalf("expected file_size=2048576, got %v (%T)", details["file_size"], details["file_size"])
	}
}

// TestBusinessAuditRecorder_NilReceiver verifies every new method is
// safe to call on a nil *BusinessAuditRecorder (no-op, no panic).
func TestBusinessAuditRecorder_NilReceiver(t *testing.T) {
	var nilRec *BusinessAuditRecorder
	ctx := context.Background()

	// All methods must not panic when called on nil receiver.
	nilRec.RecordLogin(ctx, "u", "e", "m", "")
	nilRec.RecordLoginFailed(ctx, "e", "m", "r", "")
	nilRec.RecordLogout(ctx, "u", "e")
	nilRec.RecordKnowledgeDownloaded(ctx, "k", "t", "f", 0, "kb")
	nilRec.RecordKnowledgeCreated(ctx, "k", "t", "f", "pdf", 100, "kb", "kb-name")
	nilRec.RecordKnowledgeDeleted(ctx, "k", "t", "kb", false)

	// If we reach here without panicking, the nil-guard works.
}

// TestBusinessAuditRecorder_AllNewActions ensures every new action
// constant has a corresponding record method that writes it.
func TestBusinessAuditRecorder_AllNewActions(t *testing.T) {
	rec, repo := newRecorderForTest()
	ctx := context.Background()

	// Exercise all four new record methods in sequence.
	rec.RecordLogin(ctx, "u1", "a@b.com", "email_password", "127.0.0.1")
	rec.RecordLoginFailed(ctx, "x@b.com", "email_password", "wrong_password", "127.0.0.1")
	rec.RecordLogout(ctx, "u1", "a@b.com")
	rec.RecordKnowledgeDownloaded(ctx, "kg-1", "doc.pdf", "doc.pdf", 1024, "kb-1")

	if len(repo.created) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(repo.created))
	}

	expectedActions := []types.AuditAction{
		types.AuditActionLogin,
		types.AuditActionLoginFailed,
		types.AuditActionLogout,
		types.AuditActionKnowledgeDownloaded,
	}
	for i, want := range expectedActions {
		if repo.created[i].Action != want {
			t.Fatalf("entry[%d].Action = %q, want %q", i, repo.created[i].Action, want)
		}
	}
}
