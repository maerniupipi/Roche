package container

import (
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"roche.local/knowledge-agent-platform/internal/database"
)

func TestRunRequiredMigrationsFailureStopsDatabaseStartup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration-failure.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	migrationErr := errors.New("synthetic migration failure")
	gotDSN := ""
	gotOpts := database.MigrationOptions{}
	err = runRequiredMigrations(
		db,
		"sqlite3://test.db",
		database.MigrationOptions{AutoRecoverDirty: true, SQLiteDBPath: dbPath},
		func(dsn string, opts database.MigrationOptions) error {
			gotDSN = dsn
			gotOpts = opts
			return migrationErr
		},
	)

	if !errors.Is(err, migrationErr) {
		t.Fatalf("expected migration error to be preserved, got %v", err)
	}
	if gotDSN != "sqlite3://test.db" {
		t.Fatalf("migration DSN = %q, want sqlite3://test.db", gotDSN)
	}
	if !gotOpts.AutoRecoverDirty || gotOpts.SQLiteDBPath != dbPath {
		t.Fatalf("migration options were not forwarded: %+v", gotOpts)
	}
	if pingErr := sqlDB.Ping(); pingErr == nil {
		t.Fatal("database connection remained open after migration failure")
	}
}
