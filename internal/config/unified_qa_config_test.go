package config

import "testing"

func TestApplyUnifiedQAEnvOverrides(t *testing.T) {
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_ID", " configured-route-model ")
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_NAME", " router-grpo ")
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_BASE_URL", " http://router.example/v1 ")
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_API_KEY", " ollama ")
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_PROVIDER", " generic ")
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_OUTPUT_SCHEMA", " grpo ")
	t.Setenv("UNIFIED_QA_SUMMARY_MODEL_ID", " configured-summary-model ")
	cfg := &Config{UnifiedQA: &UnifiedQAConfig{
		RouteModel: &UnifiedQARouteModelConfig{ID: "yaml-route-model"}, SummaryModelID: "yaml-summary-model",
	}}

	applyUnifiedQAEnvOverrides(cfg)

	if route := cfg.UnifiedQA.RouteModel; route.ID != "configured-route-model" || route.ModelName != "router-grpo" ||
		route.BaseURL != "http://router.example/v1" || route.APIKey != "ollama" || route.Provider != "generic" ||
		route.OutputSchema != "grpo" {
		t.Fatalf("route model = %+v", route)
	}
	if cfg.UnifiedQA.SummaryModelID != "configured-summary-model" {
		t.Fatalf("summary model ID = %q", cfg.UnifiedQA.SummaryModelID)
	}
}

func TestApplyUnifiedQAEnvOverridesPreservesYAMLValue(t *testing.T) {
	t.Setenv("UNIFIED_QA_ROUTE_MODEL_ID", "")
	t.Setenv("UNIFIED_QA_SUMMARY_MODEL_ID", "")
	cfg := &Config{UnifiedQA: &UnifiedQAConfig{
		RouteModel: &UnifiedQARouteModelConfig{
			ID: " yaml-route-model ", ModelName: " router-grpo ", BaseURL: " http://router.example/v1 ",
		},
		SummaryModelID: " yaml-summary-model ",
	}}

	applyUnifiedQAEnvOverrides(cfg)

	if route := cfg.UnifiedQA.RouteModel; route.ID != "yaml-route-model" || route.ModelName != "router-grpo" || route.BaseURL != "http://router.example/v1" {
		t.Fatalf("route model = %+v", route)
	}
	if cfg.UnifiedQA.SummaryModelID != "yaml-summary-model" {
		t.Fatalf("summary model ID = %q", cfg.UnifiedQA.SummaryModelID)
	}
}
