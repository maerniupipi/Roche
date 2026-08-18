package types

import (
	"context"
	"testing"
)

func TestPrincipalFromContextFallsBackToWebUser(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDContextKey, "u1")

	p, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("expected principal")
	}
	if p.Type != PrincipalWebUser || p.ID != "u1" {
		t.Fatalf("principal = %#v", p)
	}
}

func TestWithPrincipalRejectsBlankValues(t *testing.T) {
	ctx := WithPrincipal(context.Background(), Principal{Type: " ", ID: "x"})

	if _, ok := PrincipalFromContext(ctx); ok {
		t.Fatal("blank principal type should not be stored")
	}
}

func TestPrincipalStorageID(t *testing.T) {
	p := Principal{Type: PrincipalWebUser, ID: "alice"}

	if got := p.StorageID(); got != "web_user:alice" {
		t.Fatalf("StorageID() = %q", got)
	}
}

func TestSessionOwnerIDFromContextFallsBackToUserID(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDContextKey, "system-7")

	if got := SessionOwnerIDFromContext(ctx); got != "system-7" {
		t.Fatalf("SessionOwnerIDFromContext() = %q", got)
	}
}
