package unifiedqa

import (
	"reflect"
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
)

func TestTerminologyCatalogFindsUnknownProprietaryTerms(t *testing.T) {
	catalog, err := NewTerminologyCatalog(&config.UnifiedQATermsConfig{
		Version: "test-v1", AcceptedTerms: []string{"DoA", "T&E", "RDSL", "L1 distributor"},
	})
	if err != nil {
		t.Fatalf("NewTerminologyCatalog() error = %v", err)
	}
	got := catalog.UnknownTerms("请确认 DoA、T&E、RDSL_DOA_16.0 和 ABC-123，L1 distributor 是否适用")
	if want := []string{"ABC-123"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown terms = %v, want %v", got, want)
	}
}

func TestTerminologyCatalogIgnoresOrdinaryEnglishWords(t *testing.T) {
	catalog, err := NewTerminologyCatalog(&config.UnifiedQATermsConfig{Version: "test-v1", AcceptedTerms: []string{"DoA"}})
	if err != nil {
		t.Fatalf("NewTerminologyCatalog() error = %v", err)
	}
	if got := catalog.UnknownTerms("Can this be reimbursed under the policy?"); len(got) != 0 {
		t.Fatalf("unknown terms = %v", got)
	}
}
