package sub2key

import (
	"context"
	"time"

	domainsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/sub2key"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

const bindingOperationLease = time.Minute

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) ListSub2KeyBindings(ctx context.Context, principalID uint) ([]domainsub2key.Binding, error) {
	items := []model.Sub2KeyBinding{}
	err := r.db.WithContext(ctx).Where("principal_id = ? AND deleted_at IS NULL", principalID).Order("id DESC").Find(&items).Error
	if err != nil {
		return nil, translate(err)
	}
	out := make([]domainsub2key.Binding, 0, len(items))
	for _, item := range items {
		out = append(out, toDomainBinding(item))
	}
	return out, nil
}
func (r *Repo) GetSub2KeyBinding(ctx context.Context, principalID uint, publicID string) (*domainsub2key.Binding, error) {
	var item model.Sub2KeyBinding
	err := r.db.WithContext(ctx).Where("principal_id = ? AND public_id = ? AND deleted_at IS NULL", principalID, publicID).First(&item).Error
	if err != nil {
		return nil, translate(err)
	}
	out := toDomainBinding(item)
	return &out, nil
}
func (r *Repo) GetSub2KeyBindingByRemoteKeyID(ctx context.Context, principalID uint, remoteKeyID int64) (*domainsub2key.Binding, error) {
	var item model.Sub2KeyBinding
	err := r.db.WithContext(ctx).Where("principal_id = ? AND remote_key_id = ?", principalID, remoteKeyID).First(&item).Error
	if err != nil {
		return nil, translate(err)
	}
	out := toDomainBinding(item)
	return &out, nil
}
func (r *Repo) UpsertSub2KeyBinding(ctx context.Context, item *domainsub2key.Binding) error {
	return translate(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.Sub2KeyBinding
		err := tx.Where("principal_id = ? AND remote_key_id = ?", item.PrincipalID, item.RemoteKeyID).First(&current).Error
		candidate := fromDomainBinding(*item)
		if err == nil {
			candidate.ID = current.ID
			candidate.CreatedAt = current.CreatedAt
			candidate.DeletedAt = nil
			item.ID = current.ID
			item.CreatedAt = current.CreatedAt
			return tx.Save(&candidate).Error
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		return tx.Create(&candidate).Error
	}))
}
func (r *Repo) MarkSub2KeyBindingUnavailable(ctx context.Context, principalID uint, remoteKeyID int64, validatedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&model.Sub2KeyBinding{}).Where("principal_id = ? AND remote_key_id = ? AND deleted_at IS NULL", principalID, remoteKeyID).Updates(map[string]any{"status": "unavailable", "last_validated_at": &validatedAt})
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}
func (r *Repo) RevokeSub2KeyBinding(ctx context.Context, principalID uint, publicID string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&model.Sub2KeyBinding{}).Where("principal_id = ? AND public_id = ? AND deleted_at IS NULL", principalID, publicID).Updates(map[string]any{"ciphertext": "", "fingerprint": "", "status": "revoked", "deleted_at": &now})
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}
func (r *Repo) GetSub2KeyBindingOperation(ctx context.Context, principalID uint, key string) (*domainsub2key.BindingOperation, error) {
	var item model.Sub2KeyBindingOperation
	if err := r.db.WithContext(ctx).Where("principal_id = ? AND idempotency_key = ?", principalID, key).First(&item).Error; err != nil {
		return nil, translate(err)
	}
	return &domainsub2key.BindingOperation{PrincipalID: item.PrincipalID, IdempotencyKey: item.IdempotencyKey, RequestHash: item.RequestHash, State: item.State, BindingPublicID: item.BindingPublicID}, nil
}
func (r *Repo) CreateSub2KeyBindingOperation(ctx context.Context, item *domainsub2key.BindingOperation) (bool, error) {
	candidate := model.Sub2KeyBindingOperation{PrincipalID: item.PrincipalID, IdempotencyKey: item.IdempotencyKey, RequestHash: item.RequestHash, State: item.State, BindingPublicID: item.BindingPublicID}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "principal_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&candidate)
	if result.Error != nil || result.RowsAffected == 1 {
		return result.RowsAffected == 1, translate(result.Error)
	}
	result = r.db.WithContext(ctx).Model(&model.Sub2KeyBindingOperation{}).
		Where("principal_id = ? AND idempotency_key = ? AND request_hash = ? AND state = ? AND updated_at < ?", item.PrincipalID, item.IdempotencyKey, item.RequestHash, "pending", time.Now().Add(-bindingOperationLease)).
		Update("updated_at", time.Now())
	return result.RowsAffected == 1, translate(result.Error)
}
func (r *Repo) CompleteSub2KeyBindingOperation(ctx context.Context, principalID uint, key string, bindingPublicID string) error {
	result := r.db.WithContext(ctx).Model(&model.Sub2KeyBindingOperation{}).Where("principal_id = ? AND idempotency_key = ?", principalID, key).Updates(map[string]any{"state": "completed", "binding_public_id": bindingPublicID})
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}
func translate(err error) error {
	if err == gorm.ErrRecordNotFound {
		return repository.ErrNotFound
	}
	return err
}

func toDomainBinding(item model.Sub2KeyBinding) domainsub2key.Binding {
	return domainsub2key.Binding{ID: item.ID, CreatedAt: item.CreatedAt, PublicID: item.PublicID, PrincipalID: item.PrincipalID, Sub2AccountID: item.Sub2AccountID, RemoteKeyID: item.RemoteKeyID, Ciphertext: item.Ciphertext, Fingerprint: item.Fingerprint, Label: item.Label, MaskedKey: item.MaskedKey, GroupID: item.GroupID, GroupName: item.GroupName, Platform: item.Platform, Status: item.Status, Quota: item.Quota, UsedQuota: item.UsedQuota, ExpiresAt: item.ExpiresAt, Version: item.Version, LastValidatedAt: item.LastValidatedAt}
}

func fromDomainBinding(item domainsub2key.Binding) model.Sub2KeyBinding {
	return model.Sub2KeyBinding{BaseModel: model.BaseModel{ID: item.ID, CreatedAt: item.CreatedAt}, PublicID: item.PublicID, PrincipalID: item.PrincipalID, Sub2AccountID: item.Sub2AccountID, RemoteKeyID: item.RemoteKeyID, Ciphertext: item.Ciphertext, Fingerprint: item.Fingerprint, Label: item.Label, MaskedKey: item.MaskedKey, GroupID: item.GroupID, GroupName: item.GroupName, Platform: item.Platform, Status: item.Status, Quota: item.Quota, UsedQuota: item.UsedQuota, ExpiresAt: item.ExpiresAt, Version: item.Version, LastValidatedAt: item.LastValidatedAt}
}
