package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillFile(t *testing.T) {
	content := `---
name: test-skill
description: A test skill for unit testing purposes.
---
# Test Skill

This is the content of the test skill.

## Usage

Use this skill when testing.
`

	skill, err := ParseSkillFile(content)
	if err != nil {
		t.Fatalf("Failed to parse skill file: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got '%s'", skill.Name)
	}

	if skill.Description != "A test skill for unit testing purposes." {
		t.Errorf("Expected description 'A test skill for unit testing purposes.', got '%s'", skill.Description)
	}

	if skill.Instructions == "" {
		t.Error("Expected instructions to be non-empty")
	}

	if !skill.Loaded {
		t.Error("Expected Loaded to be true after parsing")
	}

	t.Logf("Parsed skill: name=%s, description=%s, instructions_len=%d",
		skill.Name, skill.Description, len(skill.Instructions))
}

func TestParseSkillTools(t *testing.T) {
	content := `---
name: text-tools
description: Text helpers for deterministic transformations.
tools:
  - text_reverse
  - text_word_count
---
# Text Tools

Use the declared tools when they match the request.
`

	skill, err := ParseSkillFile(content)
	if err != nil {
		t.Fatalf("ParseSkillFile() error = %v", err)
	}

	want := []string{"text_reverse", "text_word_count"}
	if len(skill.Tools) != len(want) {
		t.Fatalf("Tools length = %d, want %d (%v)", len(skill.Tools), len(want), skill.Tools)
	}
	for i := range want {
		if skill.Tools[i] != want[i] {
			t.Errorf("Tools[%d] = %q, want %q", i, skill.Tools[i], want[i])
		}
	}
}

func TestSkillToolValidation(t *testing.T) {
	tests := []struct {
		name        string
		tools       []string
		wantErr     bool
		errContains string
	}{
		{name: "instruction only", tools: nil},
		{name: "valid", tools: []string{"text_reverse", "mcp-service_tool"}},
		{name: "empty", tools: []string{""}, wantErr: true, errContains: "tool name"},
		{name: "invalid characters", tools: []string{"bad tool"}, wantErr: true, errContains: "bad tool"},
		{name: "too long", tools: []string{"t" + strings.Repeat("x", 64)}, wantErr: true, errContains: "invalid tool name"},
		{name: "duplicate", tools: []string{"text_reverse", "text_reverse"}, wantErr: true, errContains: "duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &Skill{
				Name:        "text-tools",
				Description: "Text helpers.",
				Tools:       tt.tools,
			}

			err := skill.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() error = nil, want error containing %q", tt.errContains)
				}
				if !containsString(err.Error(), tt.errContains) {
					t.Fatalf("Validate() error = %q, want substring %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestSkillValidation(t *testing.T) {
	tests := []struct {
		name        string
		skillName   string
		description string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid skill",
			skillName:   "my-skill",
			description: "A valid skill",
			wantErr:     false,
		},
		{
			name:        "empty name",
			skillName:   "",
			description: "A skill",
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name:        "invalid characters in name",
			skillName:   "My Skill",
			description: "A skill",
			wantErr:     true,
			errContains: "lowercase letters",
		},
		{
			name:        "reserved word in name",
			skillName:   "my-claude-skill",
			description: "A skill",
			wantErr:     true,
			errContains: "reserved word",
		},
		{
			name:        "empty description",
			skillName:   "my-skill",
			description: "",
			wantErr:     true,
			errContains: "description is required",
		},
		{
			name:        "name too long",
			skillName:   "this-is-a-very-long-skill-name-that-exceeds-the-maximum-allowed-length-of-64-characters",
			description: "A skill",
			wantErr:     true,
			errContains: "exceeds maximum length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &Skill{
				Name:        tt.skillName,
				Description: tt.description,
			}

			err := skill.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errContains)
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoaderDiscoverSkills(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for loader testing.
---
# Test Skill

This is the test skill content.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create loader and discover skills
	loader := NewLoader([]string{tmpDir})
	metadata, err := loader.DiscoverSkills()
	if err != nil {
		t.Fatalf("Failed to discover skills: %v", err)
	}

	if len(metadata) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(metadata))
	}

	if metadata[0].Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", metadata[0].Name)
	}

	t.Logf("Discovered %d skills: %v", len(metadata), metadata[0].Name)
}

func TestLoaderLoadSkillInstructions(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for content loading.
---
# Test Skill

This is the main content.

## Section 1

More content here.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create loader and load skill instructions
	loader := NewLoader([]string{tmpDir})
	skill, err := loader.LoadSkillInstructions("test-skill")
	if err != nil {
		t.Fatalf("Failed to load skill instructions: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", skill.Name)
	}

	if skill.Instructions == "" {
		t.Error("Expected instructions to be non-empty")
	}

	if !skill.Loaded {
		t.Error("Expected Loaded to be true")
	}

	t.Logf("Loaded skill: name=%s, instructions_len=%d", skill.Name, len(skill.Instructions))
}

func TestLoaderLoadSkillFile(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory with additional files
	skillDir := filepath.Join(tmpDir, "test-skill")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill with additional files.
---
# Test Skill

See [GUIDE.md](GUIDE.md) for more info.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Write additional file
	guideContent := "# Guide\n\nThis is the guide content."
	if err := os.WriteFile(filepath.Join(skillDir, "GUIDE.md"), []byte(guideContent), 0644); err != nil {
		t.Fatalf("Failed to write GUIDE.md: %v", err)
	}

	// Write a script
	scriptContent := "#!/usr/bin/env python3\nprint('Hello from script')"
	if err := os.WriteFile(filepath.Join(scriptsDir, "hello.py"), []byte(scriptContent), 0644); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Create loader and discover skills first
	loader := NewLoader([]string{tmpDir})
	_, err = loader.DiscoverSkills()
	if err != nil {
		t.Fatalf("Failed to discover skills: %v", err)
	}

	// Load additional file
	file, err := loader.LoadSkillFile("test-skill", "GUIDE.md")
	if err != nil {
		t.Fatalf("Failed to load skill file: %v", err)
	}

	if file.Content != guideContent {
		t.Errorf("Expected guide content, got '%s'", file.Content)
	}

	if file.IsScript {
		t.Error("GUIDE.md should not be marked as script")
	}

	// Load script file
	scriptFile, err := loader.LoadSkillFile("test-skill", "scripts/hello.py")
	if err != nil {
		t.Fatalf("Failed to load script file: %v", err)
	}

	if !scriptFile.IsScript {
		t.Error("hello.py should be marked as script")
	}

	t.Logf("Loaded files: GUIDE.md=%d bytes, hello.py=%d bytes (isScript=%v)",
		len(file.Content), len(scriptFile.Content), scriptFile.IsScript)
}

func TestManagerIntegration(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for manager integration testing.
tools:
  - test_tool
---
# Test Skill

Integration test content.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create manager with config
	config := &ManagerConfig{
		SkillDirs:     []string{tmpDir},
		AllowedSkills: []string{}, // Allow all
		Enabled:       true,
	}

	manager := NewManager(config, nil)

	// Initialize
	ctx := context.Background()
	if err := manager.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize manager: %v", err)
	}

	// Get all metadata
	metadata := manager.GetAllMetadata()
	if len(metadata) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(metadata))
	}

	// Load skill
	skill, err := manager.LoadSkill(ctx, "test-skill")
	if err != nil {
		t.Fatalf("Failed to load skill: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", skill.Name)
	}

	declaredTools, err := manager.GetSkillTools(ctx, "test-skill")
	if err != nil {
		t.Fatalf("GetSkillTools() error = %v", err)
	}
	if len(declaredTools) != 1 || declaredTools[0] != "test_tool" {
		t.Fatalf("GetSkillTools() = %v, want [test_tool]", declaredTools)
	}
	declaredTools[0] = "mutated"
	declaredToolsAgain, err := manager.GetSkillTools(ctx, "test-skill")
	if err != nil {
		t.Fatalf("GetSkillTools() second call error = %v", err)
	}
	if declaredToolsAgain[0] != "test_tool" {
		t.Fatalf("GetSkillTools() returned shared state: %v", declaredToolsAgain)
	}

	bindings, err := manager.GetAllSkillToolBindings(ctx)
	if err != nil {
		t.Fatalf("GetAllSkillToolBindings() error = %v", err)
	}
	if len(bindings) != 1 || len(bindings["test-skill"]) != 1 || bindings["test-skill"][0] != "test_tool" {
		t.Fatalf("GetAllSkillToolBindings() = %v", bindings)
	}

	t.Logf("Manager integration test passed: %d skills discovered", len(metadata))
}

func TestIsScript(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"script.py", true},
		{"script.sh", true},
		{"script.bash", true},
		{"script.js", true},
		{"script.ts", true},
		{"script.rb", true},
		{"README.md", false},
		{"data.json", false},
		{"config.yaml", false},
	}

	for _, tt := range tests {
		result := IsScript(tt.path)
		if result != tt.expected {
			t.Errorf("IsScript(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}
