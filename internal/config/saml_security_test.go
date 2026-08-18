package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validSAMLConfig() *Config {
	return &Config{SAMLAuth: &SAMLAuthConfig{
		Enable:      true,
		IdPMetadata: "<EntityDescriptor />",
		SPEntityID:  "urn:test:sp",
		ACSUrl:      "https://example.test/api/v1/auth/saml/acs",
		SPCert:      "certificate",
		SPKey:       "private-key",
	}}
}

func TestValidateConfigRequiresStableSAMLKeyPairByDefault(t *testing.T) {
	cfg := validSAMLConfig()
	cfg.SAMLAuth.SPCert = ""
	cfg.SAMLAuth.SPKey = ""

	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stable SAML SP certificate/key are required")
}

func TestValidateConfigAllowsExplicitDevelopmentEphemeralSAMLKeyPair(t *testing.T) {
	cfg := validSAMLConfig()
	cfg.SAMLAuth.SPCert = ""
	cfg.SAMLAuth.SPKey = ""
	cfg.SAMLAuth.AllowEphemeralSPCert = true

	require.NoError(t, ValidateConfig(cfg))
}

func TestValidateConfigRejectsPartialSAMLKeyPair(t *testing.T) {
	cfg := validSAMLConfig()
	cfg.SAMLAuth.SPKey = ""

	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sp_cert and sp_key must be configured together")
}

func TestValidateConfigGuardsDevelopmentSAMLSystemAdmins(t *testing.T) {
	cfg := validSAMLConfig()
	cfg.SAMLAuth.DevSystemAdminEmails = []string{"developer001@example.test"}
	cfg.Registration = &RegistrationConfig{DevRoleSelection: false}

	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dev_system_admin_emails requires registration.dev_role_selection=true")

	cfg.Registration.DevRoleSelection = true
	require.NoError(t, ValidateConfig(cfg))
}

func TestSplitNormalizedCSV(t *testing.T) {
	actual := splitNormalizedCSV(" A@Example.Test, b@example.test, a@example.test, ")
	assert.Equal(t, []string{"a@example.test", "b@example.test"}, actual)
}
