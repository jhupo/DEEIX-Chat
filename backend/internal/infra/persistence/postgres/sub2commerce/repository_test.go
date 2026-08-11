package sub2commerce

import (
	"context"
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestPaymentOperationRetainsRemoteOrderAndConverges(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Sub2PaymentOperation{}); err != nil {
		t.Fatalf("migrate payment operation table: %v", err)
	}
	repo := NewRepo(database)
	ctx := context.Background()
	const (
		principalID = uint(42001)
		key         = "f47ac10b-58cc-4372-a567-0e02b2c3d482"
		remoteID    = "trade-persisted"
	)
	operation, claimed, err := repo.ClaimPaymentOperation(ctx, principalID, key, "request-hash")
	if err != nil || !claimed || operation.State != "send_started" {
		t.Fatalf("claim: operation=%#v claimed=%v err=%v", operation, claimed, err)
	}
	if err = repo.FinishPaymentOperation(ctx, principalID, key, "outcome_unknown", remoteID); err != nil {
		t.Fatalf("store recoverable outcome: %v", err)
	}
	if err = repo.FinishPaymentOperation(ctx, principalID, key, "outcome_unknown", ""); err != nil {
		t.Fatalf("repeat unknown outcome: %v", err)
	}
	operation, err = repo.GetPaymentOperation(ctx, principalID, key)
	if err != nil || operation.State != "outcome_unknown" || operation.ExternalOrderID != remoteID {
		t.Fatalf("recoverable outcome was not retained: %#v err=%v", operation, err)
	}
	if err = repo.FinishPaymentOperation(ctx, principalID, key, "completed_success", remoteID); err != nil {
		t.Fatalf("converge operation: %v", err)
	}
	operation, err = repo.GetPaymentOperation(ctx, principalID, key)
	if err != nil || operation.State != "completed_success" || operation.ExternalOrderID != remoteID {
		t.Fatalf("operation did not converge: %#v err=%v", operation, err)
	}
}
