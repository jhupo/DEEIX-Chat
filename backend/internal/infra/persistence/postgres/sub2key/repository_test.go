package sub2key

import (
	"context"
	"testing"
	"time"

	domainsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/sub2key"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestCreateSub2KeyBindingOperationReclaimsExpiredPendingLease(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Sub2KeyBindingOperation{}); err != nil {
		t.Fatalf("migrate operation table: %v", err)
	}
	repo := NewRepo(database)
	operation := &domainsub2key.BindingOperation{
		PrincipalID:    1,
		IdempotencyKey: "12afab56-8b51-4d14-90d8-6687338dc809",
		RequestHash:    "same-request",
		State:          "pending",
	}

	claimed, err := repo.CreateSub2KeyBindingOperation(context.Background(), operation)
	if err != nil || !claimed {
		t.Fatalf("claim operation: claimed=%v err=%v", claimed, err)
	}
	claimed, err = repo.CreateSub2KeyBindingOperation(context.Background(), operation)
	if err != nil || claimed {
		t.Fatalf("active lease was reclaimed: claimed=%v err=%v", claimed, err)
	}
	if err = database.Model(&model.Sub2KeyBindingOperation{}).
		Where("principal_id = ? AND idempotency_key = ?", operation.PrincipalID, operation.IdempotencyKey).
		Update("updated_at", time.Now().Add(-2*bindingOperationLease)).Error; err != nil {
		t.Fatalf("expire operation lease: %v", err)
	}
	claimed, err = repo.CreateSub2KeyBindingOperation(context.Background(), operation)
	if err != nil || !claimed {
		t.Fatalf("reclaim expired lease: claimed=%v err=%v", claimed, err)
	}
}
