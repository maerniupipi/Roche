package unifiedqa

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestAuthorizedKBResolverReturnsAllVisibleValidKnowledgeBases(t *testing.T) {
	lister := &fakeKnowledgeBaseLister{kbs: []*types.KnowledgeBase{
		{ID: "kb-b", Name: "B", KnowledgeDomainID: 2},
		nil,
		{ID: "kb-temp", Name: "Temporary", IsTemporary: true, KnowledgeDomainID: 3},
		{ID: "kb-deleted", Name: "Deleted", DeletedAt: gorm.DeletedAt{Valid: true}, KnowledgeDomainID: 4},
		{ID: "kb-a", Name: "A", KnowledgeDomainID: 1},
	}}
	resolver := NewAuthorizedKBResolver(lister, &fakeKnowledgeDomainBatchResolver{domains: map[uint64]*types.KnowledgeDomain{
		1: {ID: 1, Name: "财务部门"}, 2: {ID: 2, Name: "合规部门"},
	}})

	scope, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got, want := scope.KnowledgeBaseIDs, []string{"kb-a", "kb-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("KnowledgeBaseIDs = %v, want %v", got, want)
	}
	if got, want := scope.SearchTargets.GetAllKnowledgeBaseIDs(), []string{"kb-a", "kb-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchTargets IDs = %v, want %v", got, want)
	}
	if scope.SearchTargets[0].KnowledgeDomainID != 1 || scope.SearchTargets[1].KnowledgeDomainID != 2 {
		t.Fatalf("SearchTargets = %+v", scope.SearchTargets)
	}
	if scope.KnowledgeBases[0].KnowledgeDomainName != "财务部门" {
		t.Fatalf("KnowledgeDomainName = %q, want 财务部门", scope.KnowledgeBases[0].KnowledgeDomainName)
	}
}

func TestAuthorizedKBResolverReturnsSentinelForEmptyScope(t *testing.T) {
	resolver := NewAuthorizedKBResolver(&fakeKnowledgeBaseLister{}, nil)
	_, err := resolver.Resolve(context.Background())
	if !errors.Is(err, ErrNoAccessibleKnowledgeBase) {
		t.Fatalf("Resolve() error = %v, want ErrNoAccessibleKnowledgeBase", err)
	}
}

func TestAuthorizedKBResolverPreservesDocumentLevelAccess(t *testing.T) {
	lister := &fakeKnowledgeBaseLister{
		kbs: []*types.KnowledgeBase{{
			ID: "kb-partial", Name: "Partial", KnowledgeDomainID: 7,
			EmbeddingModelID: "embedding-a", Type: types.KnowledgeBaseTypeDocument,
		}},
		scopes: map[string]*types.KnowledgeBaseAccessScope{
			"kb-partial": {
				Allowed:      true,
				FullAccess:   false,
				KnowledgeIDs: []string{"doc-b", "doc-a", "doc-a", ""},
			},
		},
	}

	scope, err := NewAuthorizedKBResolver(lister, nil).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(scope.KnowledgeBases) != 1 || scope.KnowledgeBases[0].FullAccess {
		t.Fatalf("KnowledgeBases = %+v", scope.KnowledgeBases)
	}
	if got, want := scope.KnowledgeBases[0].KnowledgeIDs, []string{"doc-a", "doc-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("KnowledgeIDs = %v, want %v", got, want)
	}
	if len(scope.SearchTargets) != 1 || scope.SearchTargets[0].Type != types.SearchTargetTypeKnowledge {
		t.Fatalf("SearchTargets = %+v", scope.SearchTargets)
	}
	if got, want := scope.SearchTargets[0].KnowledgeIDs, []string{"doc-a", "doc-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("target KnowledgeIDs = %v, want %v", got, want)
	}
}

func TestAuthorizedKBResolverPropagatesListFailure(t *testing.T) {
	want := errors.New("list failed")
	resolver := NewAuthorizedKBResolver(&fakeKnowledgeBaseLister{err: want}, nil)
	_, err := resolver.Resolve(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Resolve() error = %v, want %v", err, want)
	}
}

type fakeKnowledgeBaseLister struct {
	kbs    []*types.KnowledgeBase
	err    error
	scopes map[string]*types.KnowledgeBaseAccessScope
}

type fakeKnowledgeDomainBatchResolver struct {
	domains map[uint64]*types.KnowledgeDomain
	err     error
}

func (f *fakeKnowledgeDomainBatchResolver) GetKnowledgeDomainsByIDs(context.Context, []uint64) (map[uint64]*types.KnowledgeDomain, error) {
	return f.domains, f.err
}

func (f *fakeKnowledgeBaseLister) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return f.kbs, f.err
}

func (f *fakeKnowledgeBaseLister) ResolveKnowledgeBaseAccess(
	_ context.Context,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBaseAccessScope, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.scopes != nil {
		if scope, ok := f.scopes[kb.ID]; ok {
			return scope, nil
		}
	}
	return &types.KnowledgeBaseAccessScope{Allowed: true, FullAccess: true}, nil
}
