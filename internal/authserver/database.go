package authserver

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenDatabase opens the identity store used by the authentication service.
// Schema migrations remain owned by the application migration job; the auth
// service deliberately has no permission to mutate business schemas.
func OpenDatabase(ctx context.Context) (*gorm.DB, error) {
	if driver := strings.TrimSpace(os.Getenv("DB_DRIVER")); driver != "" && driver != "postgres" {
		return nil, fmt.Errorf("auth service supports DB_DRIVER=postgres only, got %q", driver)
	}

	host := envOrDefault("DB_HOST", "postgres")
	port := envOrDefault("DB_PORT", "5432")
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	database := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslMode := envOrDefault("DB_SSLMODE", "disable")
	if user == "" || password == "" || database == "" {
		return nil, fmt.Errorf("DB_USER, DB_PASSWORD and DB_NAME are required")
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		host, port, user, password, database, sslMode,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
		Logger:  logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open authentication database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get authentication database pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(envInt("AUTH_DB_MAX_OPEN_CONNECTIONS", 20))
	sqlDB.SetMaxIdleConns(envInt("AUTH_DB_MAX_IDLE_CONNECTIONS", 5))
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping authentication database: %w", err)
	}
	return db, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
