package sub2commerce

import (
	"context"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }
func (r *Repo) ClaimPaymentOperation(ctx context.Context, principalID uint, key, hash string) (*repository.Sub2PaymentOperation, bool, error) {
	var item model.Sub2PaymentOperation
	claimed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidate := model.Sub2PaymentOperation{PrincipalID: principalID, IdempotencyKey: key, RequestHash: hash, State: "prepared"}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "principal_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("principal_id = ? AND idempotency_key = ?", principalID, key).First(&item).Error; err != nil {
			return err
		}
		if item.RequestHash != hash || item.State != "prepared" {
			return nil
		}
		result := tx.Model(&model.Sub2PaymentOperation{}).Where("id = ? AND state = ?", item.ID, "prepared").Update("state", "send_started")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			item.State = "send_started"
			claimed = true
		}
		return nil
	})
	if err != nil {
		return nil, false, mapErr(err)
	}
	return &repository.Sub2PaymentOperation{RequestHash: item.RequestHash, State: item.State, ExternalOrderID: item.RemoteOrderID}, claimed, nil
}
func (r *Repo) GetPaymentOperation(ctx context.Context, principalID uint, key string) (*repository.Sub2PaymentOperation, error) {
	var item model.Sub2PaymentOperation
	if err := r.db.WithContext(ctx).Where("principal_id = ? AND idempotency_key = ?", principalID, key).First(&item).Error; err != nil {
		return nil, mapErr(err)
	}
	return &repository.Sub2PaymentOperation{RequestHash: item.RequestHash, State: item.State, ExternalOrderID: item.RemoteOrderID}, nil
}
func (r *Repo) FinishPaymentOperation(ctx context.Context, principalID uint, key, state, remoteID string) error {
	result := r.db.WithContext(ctx).Model(&model.Sub2PaymentOperation{}).
		Where("principal_id = ? AND idempotency_key = ? AND state IN ?", principalID, key, []string{"send_started", "outcome_unknown"}).
		Updates(map[string]any{"state": state, "remote_order_id": gorm.Expr("COALESCE(NULLIF(?, ''), remote_order_id)", remoteID)})
	if result.Error != nil {
		return mapErr(result.Error)
	}
	if result.RowsAffected != 1 {
		return repository.ErrNotFound
	}
	return nil
}
func mapErr(err error) error {
	if err == gorm.ErrRecordNotFound {
		return repository.ErrNotFound
	}
	return err
}
