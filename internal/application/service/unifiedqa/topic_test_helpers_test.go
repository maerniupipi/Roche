package unifiedqa

import (
	"path/filepath"
	"runtime"
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
)

func mustTestTopicCatalog(t *testing.T) *AgentCatalog {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	configDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "config")
	fallbacks, err := config.LoadUnifiedQAFallbacksFile(filepath.Join(configDir, "unified_qa_fallbacks.yaml"))
	if err != nil {
		t.Fatalf("LoadUnifiedQAFallbacksFile() error = %v", err)
	}
	catalog, err := NewAgentCatalog(testAgentCatalogConfig(), func(string) bool { return true }, fallbacks)
	if err != nil {
		t.Fatalf("NewAgentCatalog() error = %v", err)
	}
	return catalog
}
