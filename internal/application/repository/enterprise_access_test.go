package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestEnterpriseAccessSearchAndValidateActiveUsersByStatus(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&types.User{}); err != nil {
		t.Fatal(err)
	}

	activeID := uuid.NewString()
	users := []*types.User{
		{
			ID:           activeID,
			Username:     "active-user",
			Email:        "active@example.com",
			PasswordHash: "not-used",
			Status:       types.UserStatusNormal,
		},
		{
			ID:           uuid.NewString(),
			Username:     "banned-user",
			Email:        "banned@example.com",
			PasswordHash: "not-used",
			Status:       types.UserStatusBanned,
		},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}

	repo := &enterpriseAccessRepository{db: db}
	got, err := repo.SearchActiveUsers(context.Background(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != activeID || !got[0].IsActive {
		t.Fatalf("expected only the active user, got %+v", got)
	}

	active, err := repo.IsActiveUser(context.Background(), activeID)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected status=normal user to be active")
	}
	active, err = repo.IsActiveUser(context.Background(), users[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("expected banned user to be inactive")
	}
}
