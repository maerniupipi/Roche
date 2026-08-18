package container

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// fakeAuditSvc records Log calls.
type fakeAuditSvc struct {
	logged []*types.AuditLog
	err    error
}

func (f *fakeAuditSvc) Log(_ context.Context, e *types.AuditLog) error {
	f.logged = append(f.logged, e)
	return f.err
}
func (f *fakeAuditSvc) LogDenied(context.Context, *gin.Context, string, string, string) error {
	return nil
}
func (f *fakeAuditSvc) List(context.Context, *interfaces.AuditLogQuery) ([]*types.AuditLog, error) {
	return nil, nil
}
func (f *fakeAuditSvc) Purge(context.Context, int) (int64, error) { return 0, nil }

func TestAuditSinkAdapter_Emits(t *testing.T) {
	f := &fakeAuditSvc{}
	sink := newAuditSinkAdapter(f)

	sink.EmitIndexCreated(context.Background(), "roche_kap_768", 768)
	if len(f.logged) != 1 {
		t.Fatalf("want 1 audit entry, got %d", len(f.logged))
	}
	e := f.logged[0]
	if e.Action != types.AuditActionOpenSearchIndexCreated {
		t.Errorf("action: want %s, got %s", types.AuditActionOpenSearchIndexCreated, e.Action)
	}
	if e.Details == nil {
		t.Error("details should be populated")
	}

	sink.EmitReindexExecuted(context.Background(), "src", "dst", 9)
	if len(f.logged) != 2 || f.logged[1].Action != types.AuditActionOpenSearchReindexExecuted {
		t.Errorf("reindex audit not recorded: %+v", f.logged)
	}
}

func TestAuditSinkAdapter_NilServiceNoPanic(t *testing.T) {
	sink := newAuditSinkAdapter(nil)
	sink.EmitIndexCreated(context.Background(), "x", 1) // must not panic
}
