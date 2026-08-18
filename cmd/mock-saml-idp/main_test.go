package main

import (
	"testing"

	"github.com/crewjam/saml/samlidp"
	"golang.org/x/crypto/bcrypt"
)

func TestConfiguredUsersIncludesIndependentDevelopers(t *testing.T) {
	t.Setenv("MOCK_SAML_USERNAME", "admin")
	t.Setenv("MOCK_SAML_PASSWORD", "Admin123!")
	t.Setenv("MOCK_SAML_EMAIL", "admin@rochekap.local")
	t.Setenv("MOCK_SAML_DEVELOPER_COUNT", "100")
	t.Setenv("MOCK_SAML_DEVELOPER_PASSWORD", "Dev12345!")
	t.Setenv("MOCK_SAML_DEVELOPER_EMAIL_DOMAIN", "rochekap.local")

	users, err := configuredUsers()
	if err != nil {
		t.Fatalf("configuredUsers() error = %v", err)
	}
	if len(users) != 101 {
		t.Fatalf("configuredUsers() returned %d users, want 101", len(users))
	}
	if users[0].username != "admin" || users[0].email != "admin@rochekap.local" {
		t.Fatalf("primary account = %#v", users[0])
	}
	if users[1].username != "developer001" || users[1].email != "developer001@rochekap.local" {
		t.Fatalf("first developer account = %#v", users[1])
	}
	if users[100].username != "developer100" || users[100].email != "developer100@rochekap.local" {
		t.Fatalf("last developer account = %#v", users[100])
	}
}

func TestConfiguredUsersRejectsInvalidDeveloperCount(t *testing.T) {
	t.Setenv("MOCK_SAML_DEVELOPER_COUNT", "501")
	if _, err := configuredUsers(); err == nil {
		t.Fatal("configuredUsers() accepted more than 500 developer accounts")
	}
}

func TestAddMockUserPersistsUsablePasswordHash(t *testing.T) {
	store := &samlidp.MemoryStore{}
	cfg := mockUserConfig{
		username: "admin",
		password: "Admin123!",
		email:    "admin@rochekap.local",
	}
	if err := addMockUser(store, cfg); err != nil {
		t.Fatalf("addMockUser() error = %v", err)
	}

	var stored samlidp.User
	if err := store.Get("/users/admin", &stored); err != nil {
		t.Fatalf("load stored mock user: %v", err)
	}
	if len(stored.HashedPassword) == 0 {
		t.Fatal("mock user has no password hash")
	}
	if err := bcrypt.CompareHashAndPassword(stored.HashedPassword, []byte(cfg.password)); err != nil {
		t.Fatalf("stored mock password cannot be verified: %v", err)
	}
}
