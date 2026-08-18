package service

import (
	"testing"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestRegistrationRoleSelectionSupportsDevelopmentRoles(t *testing.T) {
	svc := &userService{config: &config.Config{Registration: &config.RegistrationConfig{
		Enable: true, DefaultRole: types.RegistrationRoleViewer, DevRoleSelection: true,
	}}}
	tests := []struct {
		role        types.RegistrationRole
		systemAdmin bool
	}{
		{role: types.RegistrationRoleViewer},
		{role: types.RegistrationRoleSystemAdmin, systemAdmin: true},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			systemAdmin, err := svc.resolveRegistrationAccess(tt.role, false)
			if err != nil {
				t.Fatalf("resolveRegistrationAccess: %v", err)
			}
			if systemAdmin != tt.systemAdmin {
				t.Fatalf("got systemAdmin=%v, want %v", systemAdmin, tt.systemAdmin)
			}
		})
	}
}

func TestRegistrationRoleSelectionDefaultsToViewerWhenDisabled(t *testing.T) {
	svc := &userService{config: &config.Config{Registration: &config.RegistrationConfig{
		Enable: true, DefaultRole: types.RegistrationRoleViewer,
	}}}
	systemAdmin, err := svc.resolveRegistrationAccess("", false)
	if err != nil {
		t.Fatalf("resolveRegistrationAccess default: %v", err)
	}
	if systemAdmin {
		t.Fatal("default registration must create a regular user")
	}
	if _, err := svc.resolveRegistrationAccess(types.RegistrationRoleSystemAdmin, false); err == nil {
		t.Fatal("expected explicit role to be rejected when development selection is disabled")
	}
}

func TestTrustedRegistrationRoleBypassesPublicSelector(t *testing.T) {
	svc := &userService{config: &config.Config{Registration: &config.RegistrationConfig{
		Enable: true, DefaultRole: types.RegistrationRoleViewer,
	}}}
	systemAdmin, err := svc.resolveRegistrationAccess(types.RegistrationRoleSystemAdmin, true)
	if err != nil {
		t.Fatalf("trusted role: %v", err)
	}
	if !systemAdmin {
		t.Fatal("trusted system-admin registration must set the platform flag")
	}
}
