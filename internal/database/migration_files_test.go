package database

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var versionedMigrationName = regexp.MustCompile(`^(\d+)_.+\.(up|down)\.sql$`)

func TestVersionedMigrationNumbersAreUnique(t *testing.T) {
	migrationDir := filepath.Join("..", "..", "migrations", "versioned")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read versioned migrations: %v", err)
	}

	seen := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := versionedMigrationName.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		key := matches[1] + "." + matches[2]
		if previous, exists := seen[key]; exists {
			t.Fatalf("duplicate migration version and direction %s: %s and %s", key, previous, entry.Name())
		}
		seen[key] = entry.Name()
	}
}
