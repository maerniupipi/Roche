package config

import "testing"

func TestLocalPasswordLoginDefaultsToEnabled(t *testing.T) {
	t.Setenv("AUTH_PASSWORD_LOGIN_ENABLE", "")
	cfg := &Config{}
	applyLocalAuthEnvOverrides(cfg)
	if cfg.LocalAuth == nil || !cfg.LocalAuth.PasswordLoginEnable {
		t.Fatal("local password login should remain available by default in development")
	}
}

func TestLocalPasswordLoginCanBeDisabled(t *testing.T) {
	t.Setenv("AUTH_PASSWORD_LOGIN_ENABLE", "false")
	cfg := &Config{}
	applyLocalAuthEnvOverrides(cfg)
	if cfg.LocalAuth == nil || cfg.LocalAuth.PasswordLoginEnable {
		t.Fatal("AUTH_PASSWORD_LOGIN_ENABLE=false must disable password login")
	}
}
