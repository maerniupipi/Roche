package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func repositoryConfigDir(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Join(filepath.Dir(currentFile), "..", "..", "config")
}

func TestLoadDeploymentUnifiedQAFallbacks(t *testing.T) {
	cfg, err := LoadUnifiedQAFallbacksFile(filepath.Join(repositoryConfigDir(t), "unified_qa_fallbacks.yaml"))
	if err != nil {
		t.Fatalf("LoadUnifiedQAFallbacksFile() error = %v", err)
	}
	if cfg.Version == "" || len(cfg.Topics) != 3 || cfg.Topics[0].ID != "doa" || cfg.Topics[1].ID != "travel_expense" || cfg.Topics[2].ID != "compliance" {
		t.Fatalf("config = %+v", cfg)
	}
	if got := cfg.Topics[1].KnowledgeBaseNameContains; len(got) != 1 || got[0] != "t&e" {
		t.Fatalf("T&E knowledge-base selectors = %v", got)
	}
}

func TestLoadDeploymentUnifiedQATerms(t *testing.T) {
	cfg, err := LoadUnifiedQATermsFile(filepath.Join(repositoryConfigDir(t), "unified_qa_terms.yaml"))
	if err != nil {
		t.Fatalf("LoadUnifiedQATermsFile() error = %v", err)
	}
	if cfg.Version == "" || len(cfg.AcceptedTerms) < 150 {
		t.Fatalf("terminology config = %+v", cfg)
	}
}
