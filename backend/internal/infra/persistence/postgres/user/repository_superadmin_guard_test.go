package user

import (
	"context"
	"errors"
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestUpdateFieldsKeepsLastSuperAdminProtected(t *testing.T) {
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user table: %v", err)
	}

	userItem := model.User{
		PublicID: "superadmin_public_id",
		Username: "root",
		Role:     model.RoleSuperAdmin,
		Status:   model.UserStatusActive,
		Timezone: "Etc/UTC",
		Locale:   "en-US",
	}
	if err := db.Create(&userItem).Error; err != nil {
		t.Fatalf("create superadmin: %v", err)
	}

	nextRole := model.RoleAdmin
	_, err := NewRepo(db).UpdateFields(context.Background(), userItem.ID, repository.UpdateUserFieldsInput{
		Role: &nextRole,
	})
	if !errors.Is(err, repository.ErrLastSuperAdminRoleChange) {
		t.Fatalf("expected last superadmin guard, got %v", err)
	}

	var persisted model.User
	if err := db.First(&persisted, userItem.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if persisted.Role != model.RoleSuperAdmin {
		t.Fatalf("expected role to remain %q, got %q", model.RoleSuperAdmin, persisted.Role)
	}
}
