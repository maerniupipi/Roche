package config

import "testing"

func TestEnterpriseFeaturesDefaultOff(t *testing.T) {
	for _, name := range []string{
		"ROCHE_KAP_FEATURE_MCP",
		"ROCHE_KAP_FEATURE_SKILLS",
	} {
		t.Setenv(name, "")
	}
	cfg := &Config{}
	applyFeatureOverrides(cfg)
	if cfg.IsMCPEnabled() || cfg.AreSkillsEnabled() {
		t.Fatal("optional enterprise modules must default to disabled")
	}
}

func TestEnterpriseFeatureEnvironmentOverrides(t *testing.T) {
	t.Setenv("ROCHE_KAP_FEATURE_MCP", "TRUE")
	t.Setenv("ROCHE_KAP_FEATURE_SKILLS", "true")
	cfg := &Config{}
	applyFeatureOverrides(cfg)
	if !cfg.IsMCPEnabled() || !cfg.AreSkillsEnabled() {
		t.Fatal("explicit feature environment overrides were not applied")
	}
}
