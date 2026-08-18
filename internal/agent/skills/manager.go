package skills

import (
	"context"
	"fmt"
	"sync"
)

// Manager manages instruction-only skills. Enterprise builds deliberately do
// not execute scripts shipped inside skill directories.
type Manager struct {
	loader *Loader

	// Configuration
	skillDirs     []string
	allowedSkills []string // Empty means all skills are allowed
	enabled       bool

	// Cache
	metadataCache []*SkillMetadata
	mu            sync.RWMutex
}

// ManagerConfig holds configuration for the skill manager
type ManagerConfig struct {
	SkillDirs     []string // Directories to search for skills
	AllowedSkills []string // Skill names whitelist (empty = allow all)
	Enabled       bool     // Whether skills are enabled
}

// NewManager creates a new skill manager with the given configuration
func NewManager(config *ManagerConfig, _ any) *Manager {
	if config == nil {
		config = &ManagerConfig{
			Enabled: false,
		}
	}

	return &Manager{
		loader:        NewLoader(config.SkillDirs),
		skillDirs:     config.SkillDirs,
		allowedSkills: config.AllowedSkills,
		enabled:       config.Enabled,
	}
}

// IsEnabled returns whether skills are enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// Initialize discovers all skills and caches their metadata
// This should be called at startup
func (m *Manager) Initialize(ctx context.Context) error {
	if !m.enabled {
		return nil
	}

	metadata, err := m.loader.DiscoverSkills()
	if err != nil {
		return fmt.Errorf("failed to discover skills: %w", err)
	}

	// Filter by allowed skills if specified
	if len(m.allowedSkills) > 0 {
		metadata = m.filterAllowedSkills(metadata)
	}

	m.mu.Lock()
	m.metadataCache = metadata
	m.mu.Unlock()

	return nil
}

// filterAllowedSkills filters metadata to only include allowed skills
func (m *Manager) filterAllowedSkills(metadata []*SkillMetadata) []*SkillMetadata {
	if len(m.allowedSkills) == 0 {
		return metadata
	}

	allowedSet := make(map[string]bool)
	for _, name := range m.allowedSkills {
		allowedSet[name] = true
	}

	var filtered []*SkillMetadata
	for _, meta := range metadata {
		if allowedSet[meta.Name] {
			filtered = append(filtered, meta)
		}
	}
	return filtered
}

// GetAllMetadata returns metadata for all discovered skills
// This is used for system prompt injection (Level 1)
func (m *Manager) GetAllMetadata() []*SkillMetadata {
	if !m.enabled {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*SkillMetadata, len(m.metadataCache))
	copy(result, m.metadataCache)
	return result
}

// LoadSkill loads the full instructions of a skill (Level 2)
func (m *Manager) LoadSkill(ctx context.Context, skillName string) (*Skill, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	// Check if skill is allowed
	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	return m.loader.LoadSkillInstructions(skillName)
}

// GetSkillTools returns a defensive copy of the tool names declared by an
// allowed skill.
func (m *Manager) GetSkillTools(ctx context.Context, skillName string) ([]string, error) {
	skill, err := m.LoadSkill(ctx, skillName)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), skill.Tools...), nil
}

// GetAllSkillToolBindings returns the declarative tool bindings for every
// discovered and allowed skill.
func (m *Manager) GetAllSkillToolBindings(ctx context.Context) (map[string][]string, error) {
	bindings := make(map[string][]string)
	for _, metadata := range m.GetAllMetadata() {
		tools, err := m.GetSkillTools(ctx, metadata.Name)
		if err != nil {
			return nil, err
		}
		bindings[metadata.Name] = tools
	}
	return bindings, nil
}

// isSkillAllowed checks if a skill is in the allowed list
func (m *Manager) isSkillAllowed(skillName string) bool {
	if len(m.allowedSkills) == 0 {
		return true
	}
	for _, name := range m.allowedSkills {
		if name == skillName {
			return true
		}
	}
	return false
}

// ReadSkillFile reads an additional file from a skill directory (Level 3)
func (m *Manager) ReadSkillFile(ctx context.Context, skillName, filePath string) (string, error) {
	if !m.enabled {
		return "", fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return "", fmt.Errorf("skill not allowed: %s", skillName)
	}

	file, err := m.loader.LoadSkillFile(skillName, filePath)
	if err != nil {
		return "", err
	}

	return file.Content, nil
}

// ListSkillFiles lists all files in a skill directory
func (m *Manager) ListSkillFiles(ctx context.Context, skillName string) ([]string, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	return m.loader.ListSkillFiles(skillName)
}

// GetSkillInfo returns detailed information about a skill
func (m *Manager) GetSkillInfo(ctx context.Context, skillName string) (*SkillInfo, error) {
	if !m.enabled {
		return nil, fmt.Errorf("skills are not enabled")
	}

	if !m.isSkillAllowed(skillName) {
		return nil, fmt.Errorf("skill not allowed: %s", skillName)
	}

	skill, err := m.loader.LoadSkillInstructions(skillName)
	if err != nil {
		return nil, err
	}

	files, err := m.loader.ListSkillFiles(skillName)
	if err != nil {
		files = []string{} // Non-fatal error
	}

	return &SkillInfo{
		Name:         skill.Name,
		Description:  skill.Description,
		BasePath:     skill.BasePath,
		Instructions: skill.Instructions,
		Files:        files,
	}, nil
}

// SkillInfo provides detailed information about a skill
type SkillInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	BasePath     string   `json:"base_path"`
	Instructions string   `json:"instructions"`
	Files        []string `json:"files"`
}

// Reload refreshes the skill cache by rediscovering all skills
func (m *Manager) Reload(ctx context.Context) error {
	if !m.enabled {
		return nil
	}

	metadata, err := m.loader.Reload()
	if err != nil {
		return err
	}

	if len(m.allowedSkills) > 0 {
		metadata = m.filterAllowedSkills(metadata)
	}

	m.mu.Lock()
	m.metadataCache = metadata
	m.mu.Unlock()

	return nil
}

// Cleanup is retained for engine lifecycle compatibility. Instruction-only
// skills do not allocate executable resources.
func (m *Manager) Cleanup(context.Context) error { return nil }
