package skill

import (
	"context"
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestDeleteSkillCleansConversationProjectAssociations(t *testing.T) {
	db := testutil.Postgres(t)
	var err error
	if err := db.AutoMigrate(&model.Skill{}, &model.ConversationProjectSkill{}); err != nil {
		t.Fatalf("migrate PostgreSQL tables: %v", err)
	}

	skill := model.Skill{
		Scope:       "user",
		OwnerUserID: 1,
		Title:       "Project skill",
		Trigger:     "project-skill",
		Enabled:     true,
	}
	if err = db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err = db.Create(&model.ConversationProjectSkill{ProjectID: 9, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("create project Skill association: %v", err)
	}

	if err = NewRepo(db).DeleteSkill(context.Background(), skill.ID); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}

	var associationCount int64
	if err = db.Model(&model.ConversationProjectSkill{}).Where("skill_id = ?", skill.ID).Count(&associationCount).Error; err != nil {
		t.Fatalf("count project Skill associations: %v", err)
	}
	if associationCount != 0 {
		t.Fatalf("project Skill association count = %d, want 0", associationCount)
	}
}
