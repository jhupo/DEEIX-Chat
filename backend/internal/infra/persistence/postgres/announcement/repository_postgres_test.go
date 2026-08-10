package announcement

import (
	"context"
	"testing"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
	"gorm.io/gorm"
)

func TestDismissAnnouncementTodayPostgresUsesDeclaredUniqueIndex(t *testing.T) {
	db := openAnnouncementPostgresTestDB(t)
	if !db.Migrator().HasIndex(&model.AnnouncementUserState{}, "idx_announcement_user_states_version") {
		t.Fatal("expected announcement user state version unique index")
	}

	now := time.Date(2026, 6, 6, 16, 42, 0, 0, time.UTC)
	item := model.Announcement{
		Title:           "notice",
		ContentMarkdown: "content",
		Status:          "active",
		Type:            "info",
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create announcement: %v", err)
	}

	repo := NewRepo(db)
	if err := repo.DismissAnnouncementToday(context.Background(), 1, item.ID, item.UpdatedAt, now, now.Add(8*time.Hour)); err != nil {
		t.Fatalf("dismiss announcement first time: %v", err)
	}
	if err := repo.DismissAnnouncementToday(context.Background(), 1, item.ID, item.UpdatedAt, now, now.Add(12*time.Hour)); err != nil {
		t.Fatalf("dismiss announcement second time: %v", err)
	}

	var count int64
	if err := db.Model(&model.AnnouncementUserState{}).Count(&count).Error; err != nil {
		t.Fatalf("count states: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one upserted state, got %d", count)
	}
}

func openAnnouncementPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.Announcement{}, &model.AnnouncementUserState{}); err != nil {
		t.Fatalf("migrate announcement tables: %v", err)
	}
	return db
}
