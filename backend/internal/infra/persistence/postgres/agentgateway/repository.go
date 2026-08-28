package agentgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/agentprotocol"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

const (
	threadStatusDeletingActive   = "deleting_active"
	threadStatusDeletingArchived = "deleting_archived"
	historyProjectionVersion     = 4
)

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func errFor(err error) error {
	if dberror.IsRecordNotFound(err) {
		return repository.ErrNotFound
	}
	if dberror.IsUniqueConstraint(err) {
		return repository.ErrDuplicate
	}
	return err
}

func newRepoPublicID(prefix string) string {
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func toDomainDevice(v model.AgentDevice) *domainagent.Device {
	return &domainagent.Device{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID,
		Name: v.Name, Platform: v.Platform, PublicKey: append([]byte(nil), v.PublicKey...),
		PublicKeyFingerprint: v.PublicKeyFingerprint, CredentialVersion: v.CredentialVersion,
		Status: v.Status, NextServerSeq: v.NextServerSeq,
		LastAckedServerSeq: v.LastAckedServerSeq, LastAckedBridgeSeq: v.LastAckedBridgeSeq,
		LastSeenAt: v.LastSeenAt,
		RevokedAt:  v.RevokedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toModelDevice(v *domainagent.Device) *model.AgentDevice {
	return &model.AgentDevice{
		ControlPlaneModel: model.ControlPlaneModel{ID: v.ID},
		PublicID:          v.PublicID, UserID: v.UserID,
		Name: v.Name, Platform: v.Platform, PublicKey: append([]byte(nil), v.PublicKey...),
		PublicKeyFingerprint: v.PublicKeyFingerprint, CredentialVersion: v.CredentialVersion,
		Status: v.Status, NextServerSeq: v.NextServerSeq, LastSeenAt: v.LastSeenAt, RevokedAt: v.RevokedAt,
		LastAckedServerSeq: v.LastAckedServerSeq, LastAckedBridgeSeq: v.LastAckedBridgeSeq,
	}
}

func toDomainEnrollmentChallenge(v model.AgentDeviceEnrollmentChallenge) *domainagent.DeviceEnrollmentChallenge {
	return &domainagent.DeviceEnrollmentChallenge{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, UserPublicID: v.UserPublicID,
		RemoteUserID: v.RemoteUserID, Name: v.Name, Platform: v.Platform,
		PublicKey: append([]byte(nil), v.PublicKey...), PublicKeyFingerprint: v.PublicKeyFingerprint,
		Nonce: v.Nonce, ExpiresAt: v.ExpiresAt, ConsumedAt: v.ConsumedAt,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toModelEnrollmentChallenge(v *domainagent.DeviceEnrollmentChallenge) *model.AgentDeviceEnrollmentChallenge {
	return &model.AgentDeviceEnrollmentChallenge{
		ControlPlaneModel: model.ControlPlaneModel{ID: v.ID},
		PublicID:          v.PublicID, UserID: v.UserID, UserPublicID: v.UserPublicID,
		RemoteUserID: v.RemoteUserID, Name: v.Name, Platform: v.Platform,
		PublicKey: append([]byte(nil), v.PublicKey...), PublicKeyFingerprint: v.PublicKeyFingerprint,
		Nonce: v.Nonce, ExpiresAt: v.ExpiresAt, ConsumedAt: v.ConsumedAt,
	}
}

func toDomainCommand(v model.AgentCommand) *domainagent.Command {
	return &domainagent.Command{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		RuntimeProfileID: v.RuntimeProfileID, WorkspaceID: v.WorkspaceID, ThreadID: v.ThreadID, TurnID: v.TurnID, InteractionID: v.InteractionID,
		ServerSeq: v.ServerSeq, Kind: v.Kind, PayloadJSON: v.PayloadJSON, State: v.State,
		DeliveredAt: v.DeliveredAt, AckedAt: v.AckedAt, TerminalJSON: v.TerminalJSON,
		CompletedAt: v.CompletedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomainWorkspace(v model.AgentWorkspace) *domainagent.Workspace {
	return &domainagent.Workspace{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID, RuntimeProfileID: v.RuntimeProfileID, Name: v.Name, Managed: v.Managed, Hidden: v.Hidden, Status: v.Status, LastSeenAt: v.LastSeenAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func toDomainResourceSnapshot(v model.AgentResourceSnapshot) *domainagent.ResourceSnapshot {
	return &domainagent.ResourceSnapshot{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		RuntimeProfileID: v.RuntimeProfileID, WorkspaceID: v.WorkspaceID, Name: v.Name,
		DataJSON: v.DataJSON, RefreshedAt: v.RefreshedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomainArtifact(v model.AgentArtifact) *domainagent.Artifact {
	return &domainagent.Artifact{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, WorkspaceID: v.WorkspaceID,
		FileObjectID: v.FileObjectID, FileName: v.FileName, MimeType: v.MimeType,
		SizeBytes: v.SizeBytes, SHA256: v.SHA256, Status: v.Status,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomainThread(v model.AgentThread) *domainagent.Thread {
	return &domainagent.Thread{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		RuntimeProfileID: v.RuntimeProfileID, WorkspaceID: v.WorkspaceID,
		ConversationID:  v.ConversationID,
		SourceThreadRef: v.SourceThreadRef, Title: v.Title, Status: v.Status,
		HistoryStatus: v.HistoryStatus, HistoryError: v.HistoryError,
		GitSHA: v.GitSHA, GitBranch: v.GitBranch, GitOriginURL: v.GitOriginURL,
		LastEventSeq: v.LastEventSeq, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomainTurn(v model.AgentTurn) *domainagent.Turn {
	return &domainagent.Turn{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, ThreadID: v.ThreadID, RunID: v.RunID, SourceTurnRef: v.SourceTurnRef, Status: v.Status, ErrorCode: v.ErrorCode, ErrorMessage: v.ErrorMessage, InputJSON: v.InputJSON, SettingsJSON: v.SettingsJSON, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func toDomainItem(v model.AgentItem) *domainagent.Item {
	return &domainagent.Item{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, ThreadID: v.ThreadID,
		TurnID: v.TurnID, RuntimeProfileID: v.RuntimeProfileID,
		SourceItemRef: v.SourceItemRef, Kind: v.Kind, Status: v.Status,
		DataJSON: v.DataJSON, LastEventSeq: v.LastEventSeq,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomainEvent(v model.AgentEvent) *domainagent.Event {
	return &domainagent.Event{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID, RuntimeProfileID: v.RuntimeProfileID, WorkspaceID: v.WorkspaceID, ThreadID: v.ThreadID, TurnID: v.TurnID, ThreadSeq: v.ThreadSeq, Kind: v.Kind, SourceThreadRef: v.SourceThreadRef, SourceTurnRef: v.SourceTurnRef, SourceItemRef: v.SourceItemRef, SourceRequestRef: v.SourceRequestRef, PayloadJSON: v.PayloadJSON, OccurredAt: v.OccurredAt, CreatedAt: v.CreatedAt}
}

func toDomainInteraction(v model.AgentInteraction) *domainagent.Interaction {
	return &domainagent.Interaction{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, ThreadID: v.ThreadID, TurnID: v.TurnID, RuntimeProfileID: v.RuntimeProfileID, SourceRequestRef: v.SourceRequestRef, Kind: v.Kind, RequestJSON: v.RequestJSON, Status: v.Status, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func toDomainCredential(v model.AgentCredential) *domainagent.Credential {
	return &domainagent.Credential{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		ParentCredentialID: v.ParentCredentialID, Kind: v.Kind, TokenHash: v.TokenHash,
		DerivationKeyVersion: v.DerivationKeyVersion, DeviceCredentialVersion: v.DeviceCredentialVersion,
		ExpiresAt: v.ExpiresAt, ConsumedAt: v.ConsumedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toModelCredential(v *domainagent.Credential) *model.AgentCredential {
	return &model.AgentCredential{
		ControlPlaneModel: model.ControlPlaneModel{ID: v.ID},
		PublicID:          v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		ParentCredentialID: v.ParentCredentialID, Kind: v.Kind, TokenHash: v.TokenHash,
		DerivationKeyVersion: v.DerivationKeyVersion, DeviceCredentialVersion: v.DeviceCredentialVersion,
		ExpiresAt: v.ExpiresAt, ConsumedAt: v.ConsumedAt,
	}
}

func toDomainRuntimeProfile(v model.AgentRuntimeProfile) *domainagent.RuntimeProfile {
	return &domainagent.RuntimeProfile{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		Provider: v.Provider, Status: v.Status, RemoteKeyID: v.RemoteKeyID,
		CredentialHash: v.CredentialHash, ManifestJSON: v.ManifestJSON, VerifiedAt: v.VerifiedAt,
		LeaseExpiresAt: v.LeaseExpiresAt, PresenceExpiresAt: v.PresenceExpiresAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomainRuntimeChallenge(v model.AgentRuntimeProofChallenge) *domainagent.RuntimeProofChallenge {
	return &domainagent.RuntimeProofChallenge{
		ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID,
		ProfileID: v.ProfileID, Nonce: v.Nonce, ExpiresAt: v.ExpiresAt,
		ConsumedAt: v.ConsumedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func (r *Repo) CreateEnrollmentChallenge(ctx context.Context, item *domainagent.DeviceEnrollmentChallenge) error {
	entity := toModelEnrollmentChallenge(item)
	if err := errFor(r.db.WithContext(ctx).Create(entity).Error); err != nil {
		return err
	}
	item.ID, item.CreatedAt, item.UpdatedAt = entity.ID, entity.CreatedAt, entity.UpdatedAt
	return nil
}

func (r *Repo) GetEnrollmentChallenge(ctx context.Context, publicID string) (*domainagent.DeviceEnrollmentChallenge, error) {
	var row model.AgentDeviceEnrollmentChallenge
	if err := r.db.WithContext(ctx).Where("public_id = ?", publicID).First(&row).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainEnrollmentChallenge(row), nil
}

func (r *Repo) ConsumeEnrollmentChallengeAndEnroll(ctx context.Context, challengeID uint, input *domainagent.Device, now time.Time) (*domainagent.Device, error) {
	var result model.AgentDevice
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var challenge model.AgentDeviceEnrollmentChallenge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", challengeID).First(&challenge).Error; err != nil {
			return err
		}
		if challenge.ExpiresAt.Before(now) {
			return repository.ErrConflict
		}
		lookup := tx.Where("public_key_fingerprint = ?", challenge.PublicKeyFingerprint).First(&result)
		if lookup.Error == nil {
			if result.UserID != challenge.UserID || result.PublicKeyFingerprint != input.PublicKeyFingerprint {
				return repository.ErrConflict
			}
			if result.Status != domainagent.DeviceStatusActive {
				return repository.ErrConflict
			}
		} else if !dberror.IsRecordNotFound(lookup.Error) {
			return lookup.Error
		} else {
			result = *toModelDevice(input)
			result.UserID = challenge.UserID
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
		}
		if challenge.ConsumedAt == nil {
			if err := tx.Model(&challenge).Where("consumed_at IS NULL").Update("consumed_at", now).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainDevice(result), nil
}

func (r *Repo) ListDevices(ctx context.Context, userID uint) ([]domainagent.Device, error) {
	type row struct {
		model.AgentDevice
		Online       bool   `gorm:"column:online"`
		AgentVersion string `gorm:"column:agent_version"`
	}
	var rows []row
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Table("agent_devices AS devices").
		Select("devices.*, (devices.status = ? AND EXISTS (SELECT 1 FROM agent_runtime_profiles profiles WHERE profiles.device_id = devices.id AND profiles.status = ? AND profiles.lease_expires_at > ? AND profiles.presence_expires_at > ?)) AS online, COALESCE((SELECT profiles.manifest_json ->> 'agentVersion' FROM agent_runtime_profiles profiles WHERE profiles.device_id = devices.id AND profiles.status = ? ORDER BY profiles.verified_at DESC NULLS LAST, profiles.id DESC LIMIT 1), '') AS agent_version", domainagent.DeviceStatusActive, domainagent.RuntimeStatusReady, now, now, domainagent.RuntimeStatusReady).
		Where("devices.user_id = ?", userID).Order("devices.id DESC").Scan(&rows).Error; err != nil {
		return nil, errFor(err)
	}
	result := make([]domainagent.Device, 0, len(rows))
	for _, row := range rows {
		item := toDomainDevice(row.AgentDevice)
		item.Online = row.Online
		item.AgentVersion = row.AgentVersion
		result = append(result, *item)
	}
	return result, nil
}

func (r *Repo) GetDevice(ctx context.Context, userID uint, publicID string) (*domainagent.Device, error) {
	type row struct {
		model.AgentDevice
		Online       bool   `gorm:"column:online"`
		AgentVersion string `gorm:"column:agent_version"`
	}
	var item row
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Table("agent_devices AS devices").
		Select("devices.*, (devices.status = ? AND EXISTS (SELECT 1 FROM agent_runtime_profiles profiles WHERE profiles.device_id = devices.id AND profiles.status = ? AND profiles.lease_expires_at > ? AND profiles.presence_expires_at > ?)) AS online, COALESCE((SELECT profiles.manifest_json ->> 'agentVersion' FROM agent_runtime_profiles profiles WHERE profiles.device_id = devices.id AND profiles.status = ? ORDER BY profiles.verified_at DESC NULLS LAST, profiles.id DESC LIMIT 1), '') AS agent_version", domainagent.DeviceStatusActive, domainagent.RuntimeStatusReady, now, now, domainagent.RuntimeStatusReady).
		Where("devices.user_id = ? AND devices.public_id = ?", userID, publicID).First(&item).Error; err != nil {
		return nil, errFor(err)
	}
	result := toDomainDevice(item.AgentDevice)
	result.Online = item.Online
	result.AgentVersion = item.AgentVersion
	return result, nil
}

func (r *Repo) GetDeviceByPublicID(ctx context.Context, publicID string) (*domainagent.Device, error) {
	var row model.AgentDevice
	if err := r.db.WithContext(ctx).Where("public_id = ?", publicID).First(&row).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainDevice(row), nil
}

func (r *Repo) RenameDevice(ctx context.Context, userID uint, publicID, name string) (*domainagent.Device, error) {
	result := r.db.WithContext(ctx).Model(&model.AgentDevice{}).
		Where("user_id = ? AND public_id = ?", userID, publicID).Update("name", name)
	if result.Error != nil {
		return nil, errFor(result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, repository.ErrNotFound
	}
	return r.GetDevice(ctx, userID, publicID)
}

func (r *Repo) RevokeDevice(ctx context.Context, userID uint, publicID string, now time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND public_id = ?", userID, publicID).First(&device).Error; err != nil {
			return err
		}
		if device.Status == domainagent.DeviceStatusRevoked {
			return nil
		}
		if err := tx.Model(&device).Updates(map[string]any{
			"status": domainagent.DeviceStatusRevoked, "revoked_at": now,
			"credential_version": gorm.Expr("credential_version + 1"),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentCredential{}).
			Where("device_id = ? AND consumed_at IS NULL", device.ID).Update("consumed_at", now).Error; err != nil {
			return err
		}
		terminalJSON := `{"kind":"error","error":{"message":"device revoked"}}`
		if err := tx.Model(&model.AgentCommand{}).
			Where("device_id = ? AND completed_at IS NULL", device.ID).
			Updates(map[string]any{"state": "failed", "terminal_json": terminalJSON, "completed_at": now}).Error; err != nil {
			return err
		}
		threadIDs := tx.Model(&model.AgentThread{}).Select("id").Where("device_id = ?", device.ID)
		if err := tx.Model(&model.AgentTurn{}).
			Where("thread_id IN (?) AND status IN ?", threadIDs, []string{"awaiting_thread", "queued", "running"}).
			Updates(map[string]any{"status": "failed", "error_code": "device_revoked", "error_message": "device revoked", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentInteraction{}).
			Where("thread_id IN (?) AND status IN ?", threadIDs, []string{"pending", "responding"}).
			Updates(map[string]any{"status": "failed", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentThread{}).Where("device_id = ? AND status = ?", device.ID, "queued").
			Updates(map[string]any{"status": "failed", "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&model.AgentThread{}).
			Where("device_id = ? AND status IN ?", device.ID, []string{threadStatusDeletingActive, threadStatusDeletingArchived}).
			Updates(map[string]any{
				"status":     gorm.Expr("CASE status WHEN ? THEN ? WHEN ? THEN ? END", threadStatusDeletingActive, "active", threadStatusDeletingArchived, "archived"),
				"updated_at": now,
			}).Error
	}))
}

func (r *Repo) CreateDeviceCredential(ctx context.Context, deviceID uint, item *domainagent.Credential) error {
	entity := toModelCredential(item)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, deviceID).Error; err != nil {
			return err
		}
		if device.Status != domainagent.DeviceStatusActive || device.CredentialVersion != item.DeviceCredentialVersion {
			return repository.ErrConflict
		}
		entity.UserID, entity.DeviceID = device.UserID, &device.ID
		return tx.Create(entity).Error
	})
	if err != nil {
		return errFor(err)
	}
	item.ID, item.UserID, item.DeviceID = entity.ID, entity.UserID, entity.DeviceID
	item.CreatedAt, item.UpdatedAt = entity.CreatedAt, entity.UpdatedAt
	return nil
}

func (r *Repo) GetCredential(ctx context.Context, publicID, kind string) (*domainagent.Credential, error) {
	var row model.AgentCredential
	if err := r.db.WithContext(ctx).Where("public_id = ? AND kind = ?", publicID, kind).First(&row).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainCredential(row), nil
}

func (r *Repo) ConsumeChallengeAndCreateConnection(ctx context.Context, deviceID, challengeID uint, input *domainagent.Credential, now time.Time) (*domainagent.Credential, error) {
	var result model.AgentCredential
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, deviceID).Error; err != nil {
			return err
		}
		if device.Status != domainagent.DeviceStatusActive || device.CredentialVersion != input.DeviceCredentialVersion {
			return repository.ErrConflict
		}
		var challenge model.AgentCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND device_id = ? AND kind = ?", challengeID, deviceID, domainagent.CredentialKindChallenge).
			First(&challenge).Error; err != nil {
			return err
		}
		if challenge.ExpiresAt.Before(now) {
			return repository.ErrConflict
		}
		if challenge.ConsumedAt != nil {
			if err := tx.Where("parent_credential_id = ? AND kind = ?", challenge.ID, domainagent.CredentialKindConnection).
				First(&result).Error; err != nil {
				return err
			}
			return nil
		}
		result = *toModelCredential(input)
		result.UserID, result.DeviceID, result.ParentCredentialID = device.UserID, &device.ID, &challenge.ID
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return tx.Model(&challenge).Where("consumed_at IS NULL").Update("consumed_at", now).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCredential(result), nil
}

func (r *Repo) ConsumeConnection(ctx context.Context, tokenHash string, now time.Time) (*domainagent.Device, error) {
	var device model.AgentDevice
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target model.AgentCredential
		if err := tx.Select("device_id").
			Where("kind = ? AND token_hash = ?", domainagent.CredentialKindConnection, tokenHash).
			First(&target).Error; err != nil {
			return err
		}
		if target.DeviceID == nil {
			return repository.ErrConflict
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, *target.DeviceID).Error; err != nil {
			return err
		}
		var credential model.AgentCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("kind = ? AND token_hash = ? AND device_id = ?", domainagent.CredentialKindConnection, tokenHash, device.ID).
			First(&credential).Error; err != nil {
			return err
		}
		if credential.ConsumedAt != nil || !credential.ExpiresAt.After(now) {
			return repository.ErrConflict
		}
		if device.Status != domainagent.DeviceStatusActive || device.CredentialVersion != credential.DeviceCredentialVersion {
			return repository.ErrConflict
		}
		result := tx.Model(&credential).Where("consumed_at IS NULL").Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrConflict
		}
		return tx.Model(&device).Update("last_seen_at", now).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	device.LastSeenAt = &now
	return toDomainDevice(device), nil
}

func (r *Repo) ListCommandsForDelivery(ctx context.Context, deviceID uint, after uint64, limit int) ([]domainagent.Command, error) {
	if limit < 1 || limit > 256 {
		return nil, repository.ErrInvalidInput
	}
	var device model.AgentDevice
	if err := r.db.WithContext(ctx).Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
		return nil, errFor(err)
	}
	var rows []model.AgentCommand
	err := r.db.WithContext(ctx).Where("device_id = ? AND server_seq > ?", deviceID, after).
		Order("server_seq ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, errFor(err)
	}
	result := make([]domainagent.Command, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toDomainCommand(row))
	}
	return result, nil
}

func (r *Repo) GetCommand(ctx context.Context, userID uint, publicID string) (*domainagent.Command, error) {
	var row model.AgentCommand
	if err := r.db.WithContext(ctx).Where("user_id = ? AND public_id = ?", userID, publicID).First(&row).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(row), nil
}

func (r *Repo) MarkCommandDelivered(ctx context.Context, deviceID, commandID uint, now time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		result := tx.Model(&model.AgentCommand{}).
			Where("id = ? AND device_id = ?", commandID, deviceID).
			Updates(map[string]any{
				"delivered_at": gorm.Expr("COALESCE(delivered_at, ?)", now),
				"state":        gorm.Expr("CASE WHEN state = 'queued' THEN 'delivered' ELSE state END"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return repository.ErrNotFound
		}
		return nil
	}))
}

func (r *Repo) AckServerCommands(ctx context.Context, deviceID uint, through uint64, now time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		if through <= device.LastAckedServerSeq {
			return nil
		}
		if through >= device.NextServerSeq {
			return repository.ErrConflict
		}
		var deliveredCount int64
		if err := tx.Model(&model.AgentCommand{}).
			Where("device_id = ? AND server_seq > ? AND server_seq <= ? AND delivered_at IS NOT NULL", deviceID, device.LastAckedServerSeq, through).
			Count(&deliveredCount).Error; err != nil {
			return err
		}
		if uint64(deliveredCount) != through-device.LastAckedServerSeq {
			return repository.ErrConflict
		}
		if err := tx.Model(&model.AgentCommand{}).
			Where("device_id = ? AND server_seq > ? AND server_seq <= ?", deviceID, device.LastAckedServerSeq, through).
			Updates(map[string]any{"acked_at": now, "state": gorm.Expr("CASE WHEN completed_at IS NULL THEN 'acked' ELSE state END")}).Error; err != nil {
			return err
		}
		return tx.Model(&device).Update("last_acked_server_seq", through).Error
	}))
}

func (r *Repo) ApplyTerminalFrame(ctx context.Context, deviceID uint, bridgeSeq, serverSeq uint64, commandPublicID, payloadHash, payloadJSON string, now time.Time) (uint64, error) {
	var acknowledged uint64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		acknowledged = device.LastAckedBridgeSeq
		if bridgeSeq <= device.LastAckedBridgeSeq {
			var existing model.AgentBridgeFrame
			if err := tx.Where("device_id = ? AND bridge_seq = ?", deviceID, bridgeSeq).First(&existing).Error; err != nil {
				return err
			}
			var command model.AgentCommand
			if err := tx.Where("device_id = ? AND public_id = ? AND server_seq = ?", deviceID, commandPublicID, serverSeq).
				First(&command).Error; err != nil {
				return err
			}
			if existing.Kind != "terminal" || existing.CommandID == nil || *existing.CommandID != command.ID ||
				existing.PayloadHash != payloadHash {
				return repository.ErrConflict
			}
			return nil
		}
		if bridgeSeq != device.LastAckedBridgeSeq+1 {
			return repository.ErrConflict
		}
		var command model.AgentCommand
		if err := tx.Where("device_id = ? AND public_id = ? AND server_seq = ?", deviceID, commandPublicID, serverSeq).
			First(&command).Error; err != nil {
			return err
		}
		frame := model.AgentBridgeFrame{
			DeviceID: deviceID, BridgeSeq: bridgeSeq, Kind: "terminal", CommandID: &command.ID,
			PayloadHash: payloadHash, PayloadJSON: payloadJSON, ReceivedAt: now,
		}
		if err := tx.Create(&frame).Error; err != nil {
			return err
		}
		if command.CompletedAt == nil {
			projectionErr := tx.Transaction(func(projectionTx *gorm.DB) error {
				return projectTerminalResult(projectionTx, &device, &frame, &command, payloadJSON, now)
			})
			if projectionErr != nil {
				if command.Kind != "resource.refresh" ||
					(!errors.Is(projectionErr, repository.ErrConflict) && !errors.Is(projectionErr, repository.ErrInvalidInput)) {
					return projectionErr
				}
			}
			updates := map[string]any{"state": "completed", "terminal_json": payloadJSON, "completed_at": now}
			if command.Kind == "workspace.register" {
				updates["payload_json"] = `{"kind":"workspace.register"}`
			}
			if err := tx.Model(&command).Updates(updates).Error; err != nil {
				return err
			}
		} else if command.TerminalJSON != payloadJSON {
			return repository.ErrConflict
		}
		if err := tx.Model(&device).Update("last_acked_bridge_seq", bridgeSeq).Error; err != nil {
			return err
		}
		acknowledged = bridgeSeq
		return nil
	})
	return acknowledged, errFor(err)
}

func projectTerminalResult(tx *gorm.DB, device *model.AgentDevice, frame *model.AgentBridgeFrame, command *model.AgentCommand, payloadJSON string, now time.Time) error {
	var outcome struct {
		Kind  string `json:"kind"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Kind            string          `json:"kind"`
			SourceThreadRef string          `json:"sourceThreadRef"`
			SourceTurnRef   string          `json:"sourceTurnRef"`
			WorkspaceID     string          `json:"workspaceId"`
			Name            string          `json:"name"`
			Resource        string          `json:"resource"`
			Data            json.RawMessage `json:"data"`
			Session         json.RawMessage `json:"session"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(payloadJSON), &outcome) != nil {
		return repository.ErrInvalidInput
	}
	if outcome.Kind == "error" {
		message := strings.TrimSpace(outcome.Error.Message)
		if message == "" {
			message = "local execution failed"
		}
		code := strings.TrimSpace(outcome.Error.Code)
		if code == "" {
			code = "gateway_failed"
		}
		updates := map[string]any{"status": "failed", "updated_at": now}
		if command.InteractionID != nil {
			return tx.Model(&model.AgentInteraction{}).Where("id = ?", *command.InteractionID).Updates(updates).Error
		}
		if command.TurnID != nil && (command.Kind == "thread.create" || command.Kind == "turn.start" || command.Kind == "review.start") {
			turnUpdates := map[string]any{
				"status": "failed", "error_code": code, "error_message": message, "updated_at": now,
			}
			if err := tx.Model(&model.AgentTurn{}).Where("id = ?", *command.TurnID).Updates(turnUpdates).Error; err != nil {
				return err
			}
			if command.Kind == "thread.create" && command.ThreadID != nil {
				if err := tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).Updates(updates).Error; err != nil {
					return err
				}
			}
			payload, _ := json.Marshal(map[string]any{"turn": map[string]any{
				"status": "failed", "error": map[string]string{"code": code, "message": message},
			}})
			event := model.AgentEvent{
				PublicID: newRepoPublicID("agev"), BridgeFrameID: frame.ID, UserID: command.UserID,
				DeviceID: device.ID, RuntimeProfileID: command.RuntimeProfileID, WorkspaceID: command.WorkspaceID,
				ThreadID: command.ThreadID, TurnID: command.TurnID, Kind: "turn/completed",
				PayloadJSON: string(payload), OccurredAt: now,
			}
			return tx.Create(&event).Error
		}
		if command.ThreadID != nil && command.Kind == "thread.create" {
			return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).Updates(updates).Error
		}
		if command.ThreadID != nil && command.Kind == "thread.lifecycle" {
			var payload struct {
				Action string `json:"action"`
			}
			if json.Unmarshal([]byte(command.PayloadJSON), &payload) == nil {
				switch payload.Action {
				case "fork":
					return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).Updates(updates).Error
				case "delete":
					return restoreDeletingThread(tx, *command.ThreadID, now)
				case "archive", "unarchive":
					var thread model.AgentThread
					if err := tx.First(&thread, *command.ThreadID).Error; err != nil {
						return err
					}
					restoredStatus := "active"
					if payload.Action == "unarchive" {
						restoredStatus = "archived"
					}
					return updateThreadConversationStatus(tx, &thread, restoredStatus, now)
				}
			}
		}
		if command.ThreadID != nil && command.Kind == "thread.read" {
			return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).
				Updates(map[string]any{"history_status": "error", "history_error": message, "updated_at": now}).Error
		}
		return nil
	}
	if outcome.Kind != "result" {
		return repository.ErrInvalidInput
	}
	if command.InteractionID != nil {
		return tx.Model(&model.AgentInteraction{}).Where("id = ?", *command.InteractionID).
			Updates(map[string]any{"status": "resolved", "updated_at": now}).Error
	}
	if command.Kind == "workspace.register" && outcome.Result.Kind == "accepted" {
		workspaceID, name := strings.TrimSpace(outcome.Result.WorkspaceID), strings.TrimSpace(outcome.Result.Name)
		if command.RuntimeProfileID == nil || !validRepoRef(workspaceID) || name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 {
			return repository.ErrConflict
		}
		workspace := model.AgentWorkspace{
			PublicID: workspaceID, UserID: command.UserID, DeviceID: device.ID,
			RuntimeProfileID: *command.RuntimeProfileID, Name: name, Managed: true, Status: "available", LastSeenAt: now,
		}
		if err := tx.Where("device_id = ? AND public_id = ?", device.ID, workspaceID).Attrs(workspace).FirstOrCreate(&workspace).Error; err != nil {
			return err
		}
		if workspace.UserID != command.UserID || workspace.RuntimeProfileID != *command.RuntimeProfileID {
			return repository.ErrConflict
		}
		if err := tx.Model(&workspace).Updates(map[string]any{"name": name, "managed": true, "status": "available", "last_seen_at": now}).Error; err != nil {
			return err
		}
		var profile model.AgentRuntimeProfile
		if err := tx.First(&profile, *command.RuntimeProfileID).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "resource.refresh", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "resource": map[string]string{"scope": "workspace", "name": "sessions"},
		})
		if err != nil {
			return err
		}
		refresh := model.AgentCommand{
			PublicID: newRepoPublicID("agcmd"), UserID: command.UserID, DeviceID: device.ID,
			RuntimeProfileID: command.RuntimeProfileID, WorkspaceID: &workspace.ID, ServerSeq: device.NextServerSeq,
			Kind: "resource.refresh", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&refresh).Error; err != nil {
			return err
		}
		device.NextServerSeq++
		return tx.Model(device).Update("next_server_seq", device.NextServerSeq).Error
	}
	if outcome.Result.Kind == "accepted" && (command.Kind == "workspace.rename" || command.Kind == "workspace.unregister") {
		if command.WorkspaceID == nil {
			return repository.ErrConflict
		}
		var workspace model.AgentWorkspace
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ?", *command.WorkspaceID, command.UserID, device.ID).
			First(&workspace).Error; err != nil {
			return err
		}
		if strings.TrimSpace(outcome.Result.WorkspaceID) != workspace.PublicID {
			return repository.ErrConflict
		}
		if command.Kind == "workspace.unregister" {
			return tx.Model(&workspace).Updates(map[string]any{"managed": false, "status": "unavailable", "updated_at": now}).Error
		}
		var payload struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(command.PayloadJSON), &payload) != nil || strings.TrimSpace(payload.Name) == "" ||
			strings.TrimSpace(outcome.Result.Name) != strings.TrimSpace(payload.Name) {
			return repository.ErrConflict
		}
		return tx.Model(&workspace).Updates(map[string]any{"name": strings.TrimSpace(payload.Name), "managed": true, "updated_at": now}).Error
	}
	if outcome.Result.Kind == "accepted" && command.ThreadID != nil {
		switch command.Kind {
		case "thread.rename":
			var payload struct {
				Name string `json:"name"`
			}
			if json.Unmarshal([]byte(command.PayloadJSON), &payload) != nil || strings.TrimSpace(payload.Name) == "" {
				return repository.ErrConflict
			}
			return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).
				Updates(map[string]any{"title": payload.Name, "updated_at": now}).Error
		case "thread.metadata.update":
			var payload map[string]json.RawMessage
			if json.Unmarshal([]byte(command.PayloadJSON), &payload) != nil {
				return repository.ErrConflict
			}
			var gitInfo map[string]json.RawMessage
			if json.Unmarshal(payload["gitInfo"], &gitInfo) != nil || len(gitInfo) == 0 {
				return repository.ErrConflict
			}
			updates := map[string]any{"updated_at": now}
			columns := map[string]string{
				"sha": "git_sha", "branch": "git_branch", "originUrl": "git_origin_url",
			}
			for field := range gitInfo {
				if _, allowed := columns[field]; !allowed {
					return repository.ErrConflict
				}
			}
			for field, column := range columns {
				raw, present := gitInfo[field]
				if !present {
					continue
				}
				if string(raw) == "null" {
					updates[column] = nil
					continue
				}
				var value string
				if json.Unmarshal(raw, &value) != nil || value == "" {
					return repository.ErrConflict
				}
				updates[column] = value
			}
			return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).Updates(updates).Error
		case "thread.lifecycle":
			var payload struct {
				Action string `json:"action"`
			}
			if json.Unmarshal([]byte(command.PayloadJSON), &payload) != nil {
				return repository.ErrConflict
			}
			if payload.Action == "delete" {
				result := tx.Model(&model.AgentThread{}).
					Where("id = ? AND status IN ?", *command.ThreadID, []string{threadStatusDeletingActive, threadStatusDeletingArchived}).
					Updates(map[string]any{"status": "deleted", "updated_at": now})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return repository.ErrConflict
				}
				return finalizeDeletedThreadConversation(tx, *command.ThreadID, now)
			}
			status := map[string]string{"resume": "active", "archive": "archived", "unarchive": "active"}[payload.Action]
			if status == "" {
				return repository.ErrConflict
			}
			var thread model.AgentThread
			if err := tx.First(&thread, *command.ThreadID).Error; err != nil {
				return err
			}
			return updateThreadConversationStatus(tx, &thread, status, now)
		}
	}
	switch outcome.Result.Kind {
	case "thread-read":
		if command.Kind != "thread.read" || command.ThreadID == nil || len(outcome.Result.Session) == 0 {
			return repository.ErrConflict
		}
		return syncThreadHistory(tx, command, outcome.Result.Session, now)
	case "resource":
		if command.RuntimeProfileID == nil || command.Kind != "resource.refresh" || len(outcome.Result.Data) == 0 {
			return repository.ErrConflict
		}
		var requested struct {
			Resource struct {
				Name string `json:"name"`
			} `json:"resource"`
		}
		if json.Unmarshal([]byte(command.PayloadJSON), &requested) != nil || requested.Resource.Name != outcome.Result.Resource {
			return repository.ErrConflict
		}
		workspaceID := uint(0)
		if command.WorkspaceID != nil {
			workspaceID = *command.WorkspaceID
		}
		snapshot := model.AgentResourceSnapshot{
			PublicID: newRepoPublicID("agres"), UserID: command.UserID, DeviceID: device.ID,
			RuntimeProfileID: *command.RuntimeProfileID, WorkspaceID: workspaceID,
			Name: outcome.Result.Resource, DataJSON: string(outcome.Result.Data), RefreshedAt: now,
		}
		var stored model.AgentResourceSnapshot
		if err := tx.Where("runtime_profile_id = ? AND workspace_id = ? AND name = ?", snapshot.RuntimeProfileID, snapshot.WorkspaceID, snapshot.Name).
			Attrs(snapshot).FirstOrCreate(&stored).Error; err != nil {
			return err
		}
		if err := tx.Model(&stored).Updates(map[string]any{"data_json": snapshot.DataJSON, "refreshed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if snapshot.Name == "sessions" && command.WorkspaceID != nil {
			var sessions workspaceSessionSnapshot
			if json.Unmarshal(outcome.Result.Data, &sessions) != nil {
				return repository.ErrConflict
			}
			_, err := syncWorkspaceSessions(tx, device, *command.RuntimeProfileID, *command.WorkspaceID, sessions.Data, now)
			return err
		}
		return nil
	case "thread-created", "thread-forked":
		if command.ThreadID == nil || command.RuntimeProfileID == nil || command.WorkspaceID == nil || !validRepoRef(outcome.Result.SourceThreadRef) {
			return repository.ErrConflict
		}
		var thread model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&thread, *command.ThreadID).Error; err != nil {
			return err
		}
		if thread.SourceThreadRef != nil && *thread.SourceThreadRef != outcome.Result.SourceThreadRef {
			return repository.ErrConflict
		}
		if thread.SourceThreadRef == nil {
			if err := tx.Model(&thread).Updates(map[string]any{"source_thread_ref": outcome.Result.SourceThreadRef, "status": "active", "updated_at": now}).Error; err != nil {
				return err
			}
			thread.SourceThreadRef = &outcome.Result.SourceThreadRef
		}
		if err := projectPendingThreadEvents(tx, &thread, now); err != nil {
			return err
		}
		return enqueueInitialTurn(tx, device, &thread, command, now)
	case "turn-started":
		if command.ThreadID == nil || command.TurnID == nil || !validRepoRef(outcome.Result.SourceTurnRef) {
			return repository.ErrConflict
		}
		result := tx.Model(&model.AgentTurn{}).Where("id = ? AND (source_turn_ref IS NULL OR source_turn_ref = ?)", *command.TurnID, outcome.Result.SourceTurnRef).
			Updates(map[string]any{"source_turn_ref": outcome.Result.SourceTurnRef, "status": "running", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrConflict
		}
		if err := tx.Model(&model.AgentEvent{}).
			Where("thread_id = ? AND source_turn_ref = ? AND turn_id IS NULL", *command.ThreadID, outcome.Result.SourceTurnRef).
			Update("turn_id", *command.TurnID).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentInteraction{}).
			Where("thread_id = ? AND source_request_ref IN (?) AND turn_id IS NULL", *command.ThreadID,
				tx.Model(&model.AgentEvent{}).Select("source_request_ref").Where("thread_id = ? AND source_turn_ref = ? AND source_request_ref <> ''", *command.ThreadID, outcome.Result.SourceTurnRef)).
			Update("turn_id", *command.TurnID).Error; err != nil {
			return err
		}
		var completed model.AgentEvent
		err := tx.Where("thread_id = ? AND source_turn_ref = ? AND kind = ?", *command.ThreadID, outcome.Result.SourceTurnRef, "turn/completed").
			Order("thread_seq DESC, id DESC").First(&completed).Error
		if err == nil {
			status, code, message, parseErr := agentTurnTerminal(completed.PayloadJSON)
			if parseErr != nil {
				return parseErr
			}
			if err := updateAgentTurnTerminal(tx, *command.TurnID, status, code, message, now); err != nil {
				return err
			}
		} else if !dberror.IsRecordNotFound(err) {
			return err
		}
	}
	return nil
}

type workspaceSessionMessage struct {
	Role             string                       `json:"role"`
	Content          string                       `json:"content"`
	ReasoningContent string                       `json:"reasoningContent"`
	SourceTurnRef    string                       `json:"sourceTurnRef"`
	RunID            string                       `json:"-"`
	CreatedAt        int64                        `json:"createdAt"`
	Attachments      []workspaceSessionAttachment `json:"attachments"`
	ExecutionEvents  []workspaceSessionEvent      `json:"executionEvents"`
}

type workspaceSessionEvent struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type workspaceSessionAttachment struct {
	FileID string `json:"fileID"`
}

type workspaceSession struct {
	SourceThreadRef string                    `json:"sourceThreadRef"`
	Preview         string                    `json:"preview"`
	Name            string                    `json:"name"`
	ModelProvider   string                    `json:"modelProvider"`
	Model           string                    `json:"model"`
	ReasoningEffort string                    `json:"reasoningEffort"`
	Status          string                    `json:"status"`
	CreatedAt       int64                     `json:"createdAt"`
	UpdatedAt       int64                     `json:"updatedAt"`
	RecencyAt       int64                     `json:"recencyAt"`
	HistoryLoaded   bool                      `json:"historyLoaded"`
	Messages        []workspaceSessionMessage `json:"messages"`
}

type workspaceSessionSnapshot struct {
	Data []workspaceSession `json:"data"`
}

type workspaceSessionUpdate struct {
	WorkspaceID string             `json:"workspaceId"`
	Revision    string             `json:"revision"`
	Data        []workspaceSession `json:"data"`
}

const (
	maxWorkspaceSessionMessages = 4096
	maxWorkspaceSessionBytes    = 48 << 20
)

func syncWorkspaceSessions(tx *gorm.DB, device *model.AgentDevice, profileID, workspaceID uint, sessions []workspaceSession, now time.Time) ([]string, error) {
	if device == nil || profileID == 0 || workspaceID == 0 || len(sessions) > 1000 {
		return nil, repository.ErrConflict
	}
	var profile model.AgentRuntimeProfile
	if err := tx.Where("id = ? AND user_id = ? AND device_id = ?", profileID, device.UserID, device.ID).First(&profile).Error; err != nil {
		return nil, err
	}
	var workspace model.AgentWorkspace
	if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND runtime_profile_id = ?", workspaceID, device.UserID, device.ID, profile.ID).First(&workspace).Error; err != nil {
		return nil, err
	}
	changedConversations := make(map[string]struct{})
	for _, session := range sessions {
		if !validWorkspaceSession(session, true) || session.HistoryLoaded || len(session.Messages) != 0 {
			return nil, repository.ErrConflict
		}
		var existing model.AgentThread
		err := tx.Where("runtime_profile_id = ? AND source_thread_ref = ?", profile.ID, session.SourceThreadRef).First(&existing).Error
		if err == nil {
			changed, syncErr := syncExistingWorkspaceSession(tx, &existing, &workspace, session, now)
			if syncErr != nil {
				return nil, syncErr
			}
			if changed {
				var conversation model.Conversation
				if err := tx.Select("public_id").Where("id = ? AND user_id = ?", existing.ConversationID, device.UserID).First(&conversation).Error; err != nil {
					return nil, err
				}
				changedConversations[conversation.PublicID] = struct{}{}
			}
			continue
		}
		if !dberror.IsRecordNotFound(err) {
			return nil, err
		}

		title := strings.TrimSpace(session.Name)
		if title == "" {
			title = strings.TrimSpace(session.Preview)
		}
		if title == "" {
			title = "New conversation"
		}
		title = truncateRunes(title, 255)
		createdAt := validSessionTime(session.CreatedAt, now)
		updatedAt := workspaceSessionActivityTime(session, now)
		if updatedAt.Before(createdAt) {
			updatedAt = createdAt
		}
		conversation := model.Conversation{
			UserID: device.UserID, PublicID: newChatPublicID(), Title: title, LabelsJSON: "[]",
			Model: session.Model, ReasoningEffort: session.ReasoningEffort, Provider: profile.Provider, ExecutionType: "gateway", ExecutionDeviceID: device.PublicID,
			ExecutionProfileID: profile.PublicID, ExecutionWorkspaceID: workspace.PublicID,
			SessionKey: uuid.NewString(), MessageCount: len(session.Messages), Status: session.Status, ContextPolicy: "{}",
			BaseModel: model.BaseModel{CreatedAt: createdAt, UpdatedAt: updatedAt},
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return nil, err
		}
		thread := model.AgentThread{
			PublicID: newRepoPublicID("agth"), UserID: device.UserID, DeviceID: device.ID,
			RuntimeProfileID: profile.ID, WorkspaceID: workspace.ID, ConversationID: conversation.ID,
			SourceThreadRef: &session.SourceThreadRef, Title: title, Status: session.Status, HistoryStatus: "unloaded",
		}
		if err := tx.Create(&thread).Error; err != nil {
			return nil, err
		}
		var parentID *uint
		for _, source := range session.Messages {
			createdAt := validSessionTime(source.CreatedAt, conversation.CreatedAt)
			message := model.Message{
				ConversationID: conversation.ID, UserID: device.UserID, PublicID: newChatPublicID(),
				ParentMessageID: parentID, Role: source.Role, ContentType: "text", Content: source.Content,
				ReasoningContent: source.ReasoningContent, BranchReason: "default", Status: "success",
				BaseModel: model.BaseModel{CreatedAt: createdAt, UpdatedAt: createdAt},
			}
			if err := tx.Create(&message).Error; err != nil {
				return nil, err
			}
			parentID = &message.ID
		}
		changedConversations[conversation.PublicID] = struct{}{}
	}
	publicIDs := make([]string, 0, len(changedConversations))
	for publicID := range changedConversations {
		publicIDs = append(publicIDs, publicID)
	}
	sort.Strings(publicIDs)
	return publicIDs, nil
}

func validWorkspaceSession(session workspaceSession, requireStatus bool) bool {
	if !validRepoRef(strings.TrimSpace(session.SourceThreadRef)) ||
		len(session.Messages) > maxWorkspaceSessionMessages || len(session.Name) > 1024 || len(session.Preview) > 4096 ||
		len(session.ModelProvider) > 128 || len(session.Model) > 128 || len(session.ReasoningEffort) > 32 {
		return false
	}
	if requireStatus && session.Status != "active" && session.Status != "archived" {
		return false
	}
	total := 0
	for _, message := range session.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			return false
		}
		if len(message.Attachments) > 32 {
			return false
		}
		if !validRepoRef(message.SourceTurnRef) || len(message.ExecutionEvents) > 1024 ||
			(message.Role != "assistant" && len(message.ExecutionEvents) > 0) {
			return false
		}
		for _, event := range message.ExecutionEvents {
			if !validWorkspaceSessionEvent(event) {
				return false
			}
			total += len(event.Payload)
		}
		seenAttachments := make(map[string]struct{}, len(message.Attachments))
		for _, attachment := range message.Attachments {
			if !strings.HasPrefix(attachment.FileID, "file_") || !validRepoRef(attachment.FileID) {
				return false
			}
			if _, exists := seenAttachments[attachment.FileID]; exists {
				return false
			}
			seenAttachments[attachment.FileID] = struct{}{}
		}
		total += len(message.Content) + len(message.ReasoningContent)
		if strings.TrimSpace(message.Content) == "" || total > maxWorkspaceSessionBytes {
			return false
		}
	}
	return true
}

func validWorkspaceSessionEvent(event workspaceSessionEvent) bool {
	if event.Kind != "turn/started" && event.Kind != "item/completed" && event.Kind != "turn/completed" {
		return false
	}
	if len(event.Payload) == 0 || len(event.Payload) > 1<<20 {
		return false
	}
	var payload map[string]any
	return json.Unmarshal(event.Payload, &payload) == nil
}

func syncExistingWorkspaceSession(tx *gorm.DB, thread *model.AgentThread, workspace *model.AgentWorkspace, session workspaceSession, now time.Time) (bool, error) {
	var conversation model.Conversation
	if err := tx.Where("id = ? AND user_id = ? AND execution_type = ?", thread.ConversationID, thread.UserID, "gateway").First(&conversation).Error; err != nil {
		if dberror.IsRecordNotFound(err) {
			return false, nil
		}
		return false, err
	}
	sessionActivity := workspaceSessionActivityTime(session, conversation.UpdatedAt)
	if sessionActivity.Before(conversation.UpdatedAt) {
		sessionActivity = conversation.UpdatedAt
	}
	activityAdvanced := sessionActivity.After(conversation.UpdatedAt)
	changed := false
	threadUpdates := map[string]any{}
	if (session.Status == "active" || session.Status == "archived") && thread.Status != session.Status {
		threadUpdates["status"] = session.Status
		changed = true
	}
	if thread.WorkspaceID != workspace.ID {
		threadUpdates["workspace_id"] = workspace.ID
		changed = true
	}
	if !session.HistoryLoaded && activityAdvanced {
		threadUpdates["history_status"] = "unloaded"
		threadUpdates["history_error"] = ""
		changed = true
	}
	if len(threadUpdates) > 0 {
		threadUpdates["updated_at"] = now
		if err := tx.Model(thread).Updates(threadUpdates).Error; err != nil {
			return false, err
		}
	}
	conversationUpdates := map[string]any{}
	if (session.Status == "active" || session.Status == "archived") && conversation.Status != session.Status {
		conversationUpdates["status"] = session.Status
		changed = true
	}
	if conversation.ExecutionWorkspaceID != workspace.PublicID {
		conversationUpdates["execution_workspace_id"] = workspace.PublicID
		changed = true
	}
	if title := truncateRunes(strings.TrimSpace(session.Name), 255); title != "" && conversation.Title != title {
		conversationUpdates["title"] = title
		changed = true
	}
	if activityAdvanced {
		conversationUpdates["updated_at"] = sessionActivity
		changed = true
	}
	if session.HistoryLoaded {
		if modelName := strings.TrimSpace(session.Model); modelName != "" && conversation.Model != modelName {
			conversationUpdates["model"] = modelName
			changed = true
		}
		if reasoningEffort := strings.TrimSpace(session.ReasoningEffort); reasoningEffort != "" && conversation.ReasoningEffort != reasoningEffort {
			conversationUpdates["reasoning_effort"] = reasoningEffort
			changed = true
		}
	}
	if len(conversationUpdates) > 0 {
		if err := tx.Model(&conversation).UpdateColumns(conversationUpdates).Error; err != nil {
			return false, err
		}
	}
	if !session.HistoryLoaded {
		return changed, nil
	}
	var activeGatewayRuns int64
	if err := tx.Model(&model.ConversationRun{}).
		Where("conversation_id = ? AND endpoint = ? AND status IN ?", conversation.ID, "local_gateway", []string{"queued", "running"}).
		Count(&activeGatewayRuns).Error; err != nil {
		return false, err
	}
	if activeGatewayRuns > 0 {
		return changed, nil
	}
	if err := resolveWorkspaceSessionRunIDs(tx, thread, session.Messages); err != nil {
		return false, err
	}
	var stored []model.Message
	if err := tx.Where("conversation_id = ?", conversation.ID).Order("created_at ASC, id ASC").Find(&stored).Error; err != nil {
		return false, err
	}

	projectionMatches := len(stored) <= len(session.Messages)
	if len(stored) > 0 {
		for index := range stored {
			if !projectionMatches || !historyMessageMatches(stored[index], session.Messages[index]) {
				projectionMatches = false
				break
			}
		}
		if !projectionMatches {
			if err := tx.Where("conversation_id = ?", conversation.ID).Delete(&model.Attachment{}).Error; err != nil {
				return false, err
			}
			if err := tx.Where("conversation_id = ?", conversation.ID).Delete(&model.Message{}).Error; err != nil {
				return false, err
			}
			stored = nil
		}
	}
	var parentID *uint
	for index, source := range session.Messages {
		if index < len(stored) {
			storedMessage := &stored[index]
			updates := map[string]any{}
			if storedMessage.Content != source.Content {
				updates["content"] = source.Content
			}
			if storedMessage.ReasoningContent != source.ReasoningContent {
				updates["reasoning_content"] = source.ReasoningContent
			}
			if storedMessage.RunID != source.RunID {
				updates["run_id"] = source.RunID
			}
			if (parentID == nil && storedMessage.ParentMessageID != nil) ||
				(parentID != nil && (storedMessage.ParentMessageID == nil || *storedMessage.ParentMessageID != *parentID)) {
				updates["parent_message_id"] = parentID
			}
			if len(updates) > 0 {
				if err := tx.Model(storedMessage).Updates(updates).Error; err != nil {
					return false, err
				}
			}
			if err := syncWorkspaceMessageAttachments(tx, storedMessage, source, now); err != nil {
				return false, err
			}
			parentID = &storedMessage.ID
			continue
		}
		createdAt := validSessionTime(source.CreatedAt, now)
		message := model.Message{
			ConversationID: conversation.ID, UserID: thread.UserID, PublicID: newChatPublicID(),
			ParentMessageID: parentID, Role: source.Role, ContentType: "text", Content: source.Content,
			RunID: source.RunID, ReasoningContent: source.ReasoningContent, BranchReason: "default", Status: "success",
			BaseModel: model.BaseModel{CreatedAt: createdAt, UpdatedAt: createdAt},
		}
		if err := tx.Create(&message).Error; err != nil {
			return false, err
		}
		if err := syncWorkspaceMessageAttachments(tx, &message, source, now); err != nil {
			return false, err
		}
		parentID = &message.ID
	}
	if err := syncWorkspaceSessionExecutionEvents(tx, &conversation, session.Messages, now); err != nil {
		return false, err
	}
	if err := tx.Model(thread).Updates(map[string]any{"history_status": "loaded", "history_error": "", "history_version": historyProjectionVersion, "updated_at": now}).Error; err != nil {
		return false, err
	}
	updates := map[string]any{
		"message_count": len(session.Messages), "updated_at": sessionActivity,
	}
	if modelName := strings.TrimSpace(session.Model); modelName != "" {
		updates["model"] = modelName
	}
	if reasoningEffort := strings.TrimSpace(session.ReasoningEffort); reasoningEffort != "" {
		updates["reasoning_effort"] = reasoningEffort
	}
	if title := strings.TrimSpace(session.Name); title != "" {
		updates["title"] = truncateRunes(title, 255)
	}
	if err := tx.Model(&conversation).Updates(updates).Error; err != nil {
		return false, err
	}
	return true, nil
}

func syncWorkspaceMessageAttachments(tx *gorm.DB, message *model.Message, source workspaceSessionMessage, now time.Time) error {
	if message == nil || len(source.Attachments) == 0 {
		return nil
	}
	fileIDs := make([]string, 0, len(source.Attachments))
	for _, attachment := range source.Attachments {
		fileIDs = append(fileIDs, attachment.FileID)
	}
	var existing []string
	if err := tx.Model(&model.Attachment{}).
		Where("message_id = ? AND user_id = ? AND status <> ? AND file_id IN ?", message.ID, message.UserID, "deleted", fileIDs).
		Pluck("file_id", &existing).Error; err != nil {
		return err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, fileID := range existing {
		existingSet[fileID] = struct{}{}
	}
	missing := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if _, exists := existingSet[fileID]; !exists {
			missing = append(missing, fileID)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var files []model.FileObject
	if err := tx.Where("user_id = ? AND status = ? AND file_id IN ?", message.UserID, "active", missing).Find(&files).Error; err != nil {
		return err
	}
	if len(files) != len(missing) {
		return repository.ErrConflict
	}
	byID := make(map[string]model.FileObject, len(files))
	for _, file := range files {
		byID[file.FileID] = file
	}
	rows := make([]model.Attachment, 0, len(missing))
	for _, fileID := range missing {
		file, exists := byID[fileID]
		if !exists {
			return repository.ErrConflict
		}
		detectedMIME := strings.ToLower(strings.TrimSpace(file.DetectedMIME))
		if detectedMIME == "" {
			detectedMIME = strings.ToLower(strings.TrimSpace(file.MimeType))
		}
		if !strings.HasPrefix(detectedMIME, "image/") || detectedMIME == "image/svg+xml" {
			return repository.ErrConflict
		}
		rows = append(rows, model.Attachment{
			ConversationID: message.ConversationID, MessageID: message.ID, UserID: message.UserID,
			FileID: file.FileID, Kind: "image", FileName: file.FileName, MimeType: file.MimeType,
			FileSize: file.SizeBytes, SHA256: file.SHA256, StoragePath: file.StoragePath,
			Status: "active", MetaJSON: "{}", UploadedAt: now,
		})
	}
	return tx.Create(&rows).Error
}

func historyMessageMatches(stored model.Message, source workspaceSessionMessage) bool {
	return stored.Role == source.Role && stored.Content == source.Content && stored.ReasoningContent == source.ReasoningContent
}

func resolveWorkspaceSessionRunIDs(tx *gorm.DB, thread *model.AgentThread, messages []workspaceSessionMessage) error {
	refs := make([]string, 0, len(messages))
	seenRefs := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if _, exists := seenRefs[message.SourceTurnRef]; exists {
			continue
		}
		seenRefs[message.SourceTurnRef] = struct{}{}
		refs = append(refs, message.SourceTurnRef)
	}
	type turnRun struct {
		SourceTurnRef string
		RunID         string
	}
	rows := make([]turnRun, 0)
	if err := tx.Model(&model.AgentTurn{}).Select("source_turn_ref, run_id").
		Where("thread_id = ? AND source_turn_ref IN ?", thread.ID, refs).Find(&rows).Error; err != nil {
		return err
	}
	byRef := make(map[string]string, len(rows))
	for _, row := range rows {
		byRef[row.SourceTurnRef] = row.RunID
	}
	for index := range messages {
		if runID := byRef[messages[index].SourceTurnRef]; runID != "" {
			messages[index].RunID = runID
			continue
		}
		digest := sha256.Sum256([]byte(messages[index].SourceTurnRef))
		messages[index].RunID = "run_" + hex.EncodeToString(digest[:16])
	}
	return nil
}

func syncWorkspaceSessionExecutionEvents(tx *gorm.DB, conversation *model.Conversation, messages []workspaceSessionMessage, now time.Time) error {
	runIDs := make([]string, 0, len(messages)/2)
	seenRunIDs := make(map[string]struct{}, len(messages)/2)
	for _, message := range messages {
		if message.Role != "assistant" || message.RunID == "" || len(message.ExecutionEvents) == 0 {
			continue
		}
		if _, exists := seenRunIDs[message.RunID]; exists {
			continue
		}
		seenRunIDs[message.RunID] = struct{}{}
		runIDs = append(runIDs, message.RunID)
	}
	if len(runIDs) == 0 {
		return nil
	}
	var existingRunIDs []string
	if err := tx.Model(&model.ConversationExecutionEvent{}).Distinct("run_id").
		Where("conversation_id = ? AND run_id IN ?", conversation.ID, runIDs).Pluck("run_id", &existingRunIDs).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(existingRunIDs))
	for _, runID := range existingRunIDs {
		existing[runID] = struct{}{}
	}

	sequence := conversation.ExecutionEventSeq
	for _, message := range messages {
		if message.Role != "assistant" || message.RunID == "" || len(message.ExecutionEvents) == 0 {
			continue
		}
		if _, exists := existing[message.RunID]; exists {
			continue
		}
		occurredAt := validSessionTime(message.CreatedAt, now)
		rows := make([]model.ConversationExecutionEvent, 0, len(message.ExecutionEvents))
		for index, event := range message.ExecutionEvents {
			sequence++
			rows = append(rows, model.ConversationExecutionEvent{
				ConversationID: conversation.ID, UserID: conversation.UserID, RunID: message.RunID,
				SourceKey: fmt.Sprintf("history:%s:%d", message.RunID, index), Seq: sequence,
				Kind: event.Kind, PayloadJSON: string(event.Payload), OccurredAt: occurredAt,
			})
		}
		if err := tx.CreateInBatches(&rows, 500).Error; err != nil {
			return err
		}
		existing[message.RunID] = struct{}{}
	}
	if sequence == conversation.ExecutionEventSeq {
		return nil
	}
	conversation.ExecutionEventSeq = sequence
	return tx.Model(conversation).Update("execution_event_seq", sequence).Error
}

func syncThreadHistory(tx *gorm.DB, command *model.AgentCommand, raw json.RawMessage, now time.Time) error {
	if command.ThreadID == nil || command.WorkspaceID == nil {
		return repository.ErrConflict
	}
	var session workspaceSession
	if json.Unmarshal(raw, &session) != nil || !session.HistoryLoaded || !validWorkspaceSession(session, false) {
		return repository.ErrConflict
	}
	var thread model.AgentThread
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&thread, *command.ThreadID).Error; err != nil {
		return err
	}
	if thread.SourceThreadRef == nil || *thread.SourceThreadRef != session.SourceThreadRef || thread.WorkspaceID != *command.WorkspaceID {
		return repository.ErrConflict
	}
	var workspace model.AgentWorkspace
	if err := tx.First(&workspace, thread.WorkspaceID).Error; err != nil {
		return err
	}
	session.Status = thread.Status
	_, err := syncExistingWorkspaceSession(tx, &thread, &workspace, session, now)
	return err
}

func newChatPublicID() string { return strings.ReplaceAll(uuid.NewString(), "-", "") }

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func validSessionTime(seconds int64, fallback time.Time) time.Time {
	if seconds <= 0 {
		return fallback
	}
	value := time.Unix(seconds, 0).UTC()
	if value.After(fallback.UTC().Add(24 * time.Hour)) {
		return fallback
	}
	return value
}

func workspaceSessionActivityTime(session workspaceSession, fallback time.Time) time.Time {
	updatedAt := validSessionTime(session.UpdatedAt, fallback)
	if session.RecencyAt <= 0 {
		return updatedAt
	}
	recencyAt := validSessionTime(session.RecencyAt, updatedAt)
	if recencyAt.After(updatedAt) {
		return recencyAt
	}
	return updatedAt
}

func enqueueInitialTurn(tx *gorm.DB, device *model.AgentDevice, thread *model.AgentThread, createCommand *model.AgentCommand, now time.Time) error {
	var turn model.AgentTurn
	if err := tx.Where("thread_id = ? AND status = ?", thread.ID, "awaiting_thread").Order("id ASC").First(&turn).Error; err != nil {
		if dberror.IsRecordNotFound(err) {
			return nil
		}
		return err
	}
	var count int64
	if err := tx.Model(&model.AgentCommand{}).Where("turn_id = ? AND kind = ?", turn.ID, "turn.start").Count(&count).Error; err != nil || count > 0 {
		return err
	}
	var profile model.AgentRuntimeProfile
	if err := tx.First(&profile, *createCommand.RuntimeProfileID).Error; err != nil {
		return err
	}
	var workspace model.AgentWorkspace
	if err := tx.First(&workspace, *createCommand.WorkspaceID).Error; err != nil {
		return err
	}
	if err := validateCommandArtifacts(tx, thread.UserID, workspace.ID, turn.InputJSON); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"kind": "turn.start", "deviceId": device.PublicID, "profileId": profile.PublicID,
		"workspaceId": workspace.PublicID, "threadId": thread.PublicID,
		"sourceThreadRef": *thread.SourceThreadRef, "input": json.RawMessage(turn.InputJSON),
		"settings": json.RawMessage(turn.SettingsJSON),
	})
	if err != nil {
		return err
	}
	command := model.AgentCommand{
		PublicID: newRepoPublicID("agcmd"), UserID: thread.UserID, DeviceID: device.ID,
		RuntimeProfileID: createCommand.RuntimeProfileID, WorkspaceID: createCommand.WorkspaceID,
		ThreadID: &thread.ID, TurnID: &turn.ID, ServerSeq: device.NextServerSeq,
		Kind: "turn.start", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
	}
	if err := tx.Create(&command).Error; err != nil {
		return err
	}
	if err := tx.Model(device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
		return err
	}
	return tx.Model(&turn).Updates(map[string]any{"status": "queued", "updated_at": now}).Error
}

func projectPendingThreadEvents(tx *gorm.DB, thread *model.AgentThread, projectedAt time.Time) error {
	var events []model.AgentEvent
	if err := tx.Where("runtime_profile_id = ? AND source_thread_ref = ? AND thread_id IS NULL", thread.RuntimeProfileID, *thread.SourceThreadRef).
		Order("bridge_frame_id ASC").Find(&events).Error; err != nil {
		return err
	}
	for i := range events {
		if err := projectAgentEvent(tx, &events[i]); err != nil {
			return err
		}
		updates := map[string]any{
			"thread_id": events[i].ThreadID, "workspace_id": events[i].WorkspaceID,
			"turn_id": events[i].TurnID, "thread_seq": events[i].ThreadSeq,
		}
		if isThreadOnlyEvent(&events[i]) {
			updates["conversation_projected_at"] = projectedAt
		}
		if err := tx.Model(&events[i]).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func isThreadOnlyEvent(event *model.AgentEvent) bool {
	return event.ThreadID != nil && event.SourceTurnRef == ""
}

func validRepoRef(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || (index > 0 && strings.ContainsRune("._:-", character)) {
			continue
		}
		return false
	}
	return true
}

func manifestSupportsCommand(value, command string) bool {
	var manifest struct {
		Commands []string `json:"commands"`
	}
	if json.Unmarshal([]byte(value), &manifest) != nil {
		return false
	}
	return slices.Contains(manifest.Commands, command)
}

func manifestSupportsSettings(manifestJSON, settingsJSON string) bool {
	var manifest struct {
		ThreadSettings struct {
			Model             bool     `json:"model"`
			ReasoningEffort   []string `json:"reasoningEffort"`
			ApprovalPolicy    []string `json:"approvalPolicy"`
			ApprovalsReviewer []string `json:"approvalsReviewer"`
			SandboxPolicy     []string `json:"sandboxPolicy"`
		} `json:"threadSettings"`
	}
	var settings map[string]string
	if json.Unmarshal([]byte(manifestJSON), &manifest) != nil || json.Unmarshal([]byte(settingsJSON), &settings) != nil {
		return false
	}
	if len(settings) != 5 || !validRepoApprovalMode(settings["approvalPolicy"], settings["approvalsReviewer"], settings["sandboxPolicy"]) {
		return false
	}
	for key, value := range settings {
		switch key {
		case "model":
			if !manifest.ThreadSettings.Model || strings.TrimSpace(value) == "" {
				return false
			}
		case "reasoningEffort":
			if !slices.Contains(manifest.ThreadSettings.ReasoningEffort, value) {
				return false
			}
		case "approvalPolicy":
			if !slices.Contains(manifest.ThreadSettings.ApprovalPolicy, value) {
				return false
			}
		case "approvalsReviewer":
			if !slices.Contains(manifest.ThreadSettings.ApprovalsReviewer, value) {
				return false
			}
		case "sandboxPolicy":
			if !slices.Contains(manifest.ThreadSettings.SandboxPolicy, value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validRepoApprovalMode(approvalPolicy, approvalsReviewer, sandboxPolicy string) bool {
	return approvalPolicy == "on-request" && approvalsReviewer == "user" && sandboxPolicy == "workspace-write" ||
		approvalPolicy == "on-request" && approvalsReviewer == "auto_review" && sandboxPolicy == "workspace-write" ||
		approvalPolicy == "never" && approvalsReviewer == "user" && sandboxPolicy == "danger-full-access"
}

func (r *Repo) ApplyEventFrame(ctx context.Context, deviceID, runtimeProfileID uint, bridgeSeq uint64, payloadHash string, event *domainagent.Event, now time.Time) (*domainagent.AppliedEventFrame, error) {
	var applied domainagent.AppliedEventFrame
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		applied.Acknowledged = device.LastAckedBridgeSeq
		if bridgeSeq <= device.LastAckedBridgeSeq {
			var existing model.AgentBridgeFrame
			if err := tx.Where("device_id = ? AND bridge_seq = ?", deviceID, bridgeSeq).First(&existing).Error; err != nil {
				return err
			}
			if existing.Kind != "event" || existing.PayloadHash != payloadHash {
				return repository.ErrConflict
			}
			return loadAppliedEventFrame(tx, existing.ID, device.LastAckedBridgeSeq, &applied)
		}
		if bridgeSeq != device.LastAckedBridgeSeq+1 {
			return repository.ErrConflict
		}
		frame := model.AgentBridgeFrame{
			DeviceID: deviceID, BridgeSeq: bridgeSeq, Kind: "event",
			PayloadHash: payloadHash, PayloadJSON: event.PayloadJSON, ReceivedAt: now,
		}
		if err := tx.Create(&frame).Error; err != nil {
			return err
		}
		projected := model.AgentEvent{
			PublicID: event.PublicID, BridgeFrameID: frame.ID, UserID: device.UserID, DeviceID: deviceID,
			RuntimeProfileID: &runtimeProfileID, Kind: event.Kind, SourceThreadRef: event.SourceThreadRef,
			SourceTurnRef: event.SourceTurnRef, SourceItemRef: event.SourceItemRef,
			SourceRequestRef: event.SourceRequestRef, PayloadJSON: event.PayloadJSON, OccurredAt: event.OccurredAt,
		}
		if projected.Kind == agentprotocol.SessionSnapshotEventKind {
			var snapshot workspaceSessionUpdate
			if json.Unmarshal([]byte(projected.PayloadJSON), &snapshot) != nil || !validSessionSnapshotRevision(snapshot.Revision) {
				return repository.ErrConflict
			}
			var workspace model.AgentWorkspace
			if err := tx.Where("public_id = ? AND user_id = ? AND device_id = ? AND runtime_profile_id = ?", snapshot.WorkspaceID, device.UserID, device.ID, runtimeProfileID).First(&workspace).Error; err != nil {
				return err
			}
			changed, err := syncWorkspaceSessions(tx, &device, runtimeProfileID, workspace.ID, snapshot.Data, now)
			if err != nil {
				return err
			}
			projected.WorkspaceID = &workspace.ID
			projected.ConversationProjectedAt = &now
			applied.ConversationPublicIDs = changed
		} else {
			if err := projectAgentEvent(tx, &projected); err != nil {
				return err
			}
			if isThreadOnlyEvent(&projected) {
				projected.ConversationProjectedAt = &now
			}
		}
		if err := tx.Create(&projected).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("last_acked_bridge_seq", bridgeSeq).Error; err != nil {
			return err
		}
		return loadAppliedEventFrame(tx, frame.ID, bridgeSeq, &applied)
	})
	if err != nil {
		return nil, errFor(err)
	}
	return &applied, nil
}

func validSessionSnapshotRevision(value string) bool {
	if len(value) != 24 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func loadAppliedEventFrame(tx *gorm.DB, bridgeFrameID uint, acknowledged uint64, result *domainagent.AppliedEventFrame) error {
	var event model.AgentEvent
	if err := tx.Where("bridge_frame_id = ?", bridgeFrameID).First(&event).Error; err != nil {
		return err
	}
	result.Acknowledged = acknowledged
	result.Event = *toDomainEvent(event)
	if event.ConversationProjectedAt != nil {
		return nil
	}
	if event.ThreadID == nil {
		return nil
	}
	var thread model.AgentThread
	if err := tx.Select("conversation_id").First(&thread, *event.ThreadID).Error; err != nil {
		return err
	}
	result.ConversationID = thread.ConversationID
	if event.TurnID == nil {
		return nil
	}
	var turn model.AgentTurn
	if err := tx.Select("run_id").First(&turn, *event.TurnID).Error; err != nil {
		return err
	}
	result.RunID = turn.RunID
	return nil
}

func (r *Repo) ListPendingConversationEvents(ctx context.Context, deviceID uint, limit int) ([]domainagent.AppliedEventFrame, error) {
	if deviceID == 0 || limit < 1 || limit > 1000 {
		return nil, repository.ErrInvalidInput
	}
	type row struct {
		model.AgentEvent
		ConversationID uint
		RunID          string
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Table("agent_events AS events").
		Select("events.*, threads.conversation_id, turns.run_id").
		Joins("JOIN agent_threads AS threads ON threads.id = events.thread_id").
		Joins("JOIN agent_turns AS turns ON turns.id = events.turn_id").
		Where("events.device_id = ? AND events.conversation_projected_at IS NULL AND threads.conversation_id > 0 AND turns.run_id <> ''", deviceID).
		Order("events.id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, errFor(err)
	}
	result := make([]domainagent.AppliedEventFrame, 0, len(rows))
	for _, item := range rows {
		result = append(result, domainagent.AppliedEventFrame{
			Event: *toDomainEvent(item.AgentEvent), ConversationID: item.ConversationID, RunID: item.RunID,
		})
	}
	return result, nil
}

func (r *Repo) MarkConversationEventProjected(ctx context.Context, eventID uint, projectedAt time.Time) error {
	if eventID == 0 || projectedAt.IsZero() {
		return repository.ErrInvalidInput
	}
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event model.AgentEvent
		if err := tx.Select("id", "thread_id", "conversation_projected_at").First(&event, eventID).Error; err != nil {
			return err
		}
		if event.ConversationProjectedAt == nil {
			result := tx.Model(&model.AgentEvent{}).
				Where("id = ? AND conversation_projected_at IS NULL", eventID).
				Update("conversation_projected_at", projectedAt)
			if result.Error != nil {
				return result.Error
			}
		}
		if event.ThreadID == nil {
			return nil
		}
		return finalizeDeletedThreadConversation(tx, *event.ThreadID, projectedAt)
	}))
}

func projectAgentEvent(tx *gorm.DB, event *model.AgentEvent) error {
	if event.SourceThreadRef == "" || event.RuntimeProfileID == nil {
		return nil
	}
	var thread model.AgentThread
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("runtime_profile_id = ? AND source_thread_ref = ?", *event.RuntimeProfileID, event.SourceThreadRef).
		First(&thread).Error; err != nil {
		if dberror.IsRecordNotFound(err) {
			return nil
		}
		return err
	}
	next := thread.LastEventSeq + 1
	event.ThreadID, event.WorkspaceID, event.ThreadSeq = &thread.ID, &thread.WorkspaceID, &next
	if event.Kind == "thread/archived" || event.Kind == "thread/unarchived" {
		status := "active"
		if event.Kind == "thread/archived" {
			status = "archived"
		}
		if err := updateThreadConversationStatus(tx, &thread, status, event.OccurredAt); err != nil {
			return err
		}
	}
	if event.SourceTurnRef != "" {
		var turn model.AgentTurn
		if err := tx.Where("thread_id = ? AND source_turn_ref = ?", thread.ID, event.SourceTurnRef).First(&turn).Error; err == nil {
			event.TurnID = &turn.ID
			if event.Kind == "turn/started" {
				if err := tx.Model(&turn).
					Where("status NOT IN ?", []string{"completed", "interrupted", "failed"}).
					Updates(map[string]any{"status": "running", "error_code": "", "error_message": ""}).Error; err != nil {
					return err
				}
			}
			if event.Kind == "turn/completed" {
				status, code, message, err := agentTurnTerminal(event.PayloadJSON)
				if err != nil {
					return err
				}
				if err := updateAgentTurnTerminal(tx, turn.ID, status, code, message, event.OccurredAt); err != nil {
					return err
				}
				if err := resolveTurnInteractions(tx, turn.ID, event.OccurredAt); err != nil {
					return err
				}
			}
		} else if !dberror.IsRecordNotFound(err) {
			return err
		}
	}
	if err := tx.Model(&thread).Update("last_event_seq", next).Error; err != nil {
		return err
	}
	if (event.Kind == "item/started" || event.Kind == "item/completed") && event.SourceItemRef != "" {
		var payload struct {
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return repository.ErrConflict
		}
		if payload.Item.Type == "" {
			var projected struct {
				Item struct {
					Kind string `json:"kind"`
				} `json:"item"`
			}
			if json.Unmarshal([]byte(event.PayloadJSON), &projected) != nil || projected.Item.Kind == "" {
				return repository.ErrConflict
			}
			payload.Item.Type = projected.Item.Kind
		}
		status := "running"
		if event.Kind == "item/completed" {
			status = "completed"
		}
		item := model.AgentItem{
			PublicID: newRepoPublicID("agit"), UserID: event.UserID, ThreadID: thread.ID,
			TurnID: event.TurnID, RuntimeProfileID: *event.RuntimeProfileID,
			SourceItemRef: event.SourceItemRef, Kind: payload.Item.Type, Status: status,
			DataJSON: event.PayloadJSON, LastEventSeq: next,
		}
		var stored model.AgentItem
		result := tx.Where("runtime_profile_id = ? AND source_item_ref = ?", item.RuntimeProfileID, item.SourceItemRef).
			Attrs(item).FirstOrCreate(&stored)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if stored.ThreadID != thread.ID || stored.UserID != event.UserID ||
				(event.TurnID != nil && (stored.TurnID == nil || *stored.TurnID != *event.TurnID)) {
				return repository.ErrConflict
			}
			if event.Kind == "item/started" && stored.Status == "completed" {
				return repository.ErrConflict
			}
			if err := tx.Model(&stored).Updates(map[string]any{
				"turn_id": event.TurnID, "kind": item.Kind, "status": status,
				"data_json": item.DataJSON, "last_event_seq": next,
			}).Error; err != nil {
				return err
			}
		}
	}
	if event.Kind == "interaction.requested" && event.SourceRequestRef != "" {
		kind, requestJSON, err := projectInteractionRequest(event.PayloadJSON)
		if err != nil {
			return err
		}
		interaction := model.AgentInteraction{
			PublicID: newRepoPublicID("agint"), UserID: event.UserID, ThreadID: thread.ID,
			TurnID: event.TurnID, RuntimeProfileID: *event.RuntimeProfileID,
			SourceRequestRef: event.SourceRequestRef, Kind: kind,
			RequestJSON: requestJSON, Status: "pending",
		}
		if event.TurnID != nil {
			var status string
			if err := tx.Model(&model.AgentTurn{}).Select("status").Where("id = ?", *event.TurnID).Scan(&status).Error; err != nil {
				return err
			}
			if status == "completed" || status == "interrupted" || status == "failed" {
				interaction.Status = "resolved"
			}
		}
		if err := tx.Where("runtime_profile_id = ? AND source_request_ref = ?", interaction.RuntimeProfileID, interaction.SourceRequestRef).
			Attrs(interaction).FirstOrCreate(&interaction).Error; err != nil {
			return err
		}
	}
	if event.Kind == "serverRequest/resolved" && event.SourceRequestRef != "" {
		if err := tx.Model(&model.AgentInteraction{}).
			Where("runtime_profile_id = ? AND source_request_ref = ?", *event.RuntimeProfileID, event.SourceRequestRef).
			Update("status", "resolved").Error; err != nil {
			return err
		}
	}
	return nil
}

func agentTurnTerminal(payloadJSON string) (string, string, string, error) {
	var payload struct {
		Turn struct {
			Status string `json:"status"`
			Error  *struct {
				Message        string          `json:"message"`
				CodexErrorInfo json.RawMessage `json:"codexErrorInfo"`
			} `json:"error"`
		} `json:"turn"`
	}
	if json.Unmarshal([]byte(payloadJSON), &payload) != nil {
		return "", "", "", repository.ErrConflict
	}
	switch payload.Turn.Status {
	case "completed", "interrupted":
		return payload.Turn.Status, "", "", nil
	case "failed":
		code := "gateway_failed"
		message := "local execution failed"
		if payload.Turn.Error != nil {
			if value := strings.TrimSpace(payload.Turn.Error.Message); value != "" {
				message = truncateRunes(value, 4096)
			}
			if value := codexErrorCode(payload.Turn.Error.CodexErrorInfo); value != "" {
				code = value
			}
		}
		return payload.Turn.Status, code, message, nil
	default:
		return "", "", "", repository.ErrConflict
	}
}

func restoreDeletingThread(tx *gorm.DB, threadID uint, now time.Time) error {
	return tx.Model(&model.AgentThread{}).
		Where("id = ? AND status IN ?", threadID, []string{threadStatusDeletingActive, threadStatusDeletingArchived}).
		Updates(map[string]any{
			"status":     gorm.Expr("CASE status WHEN ? THEN ? WHEN ? THEN ? END", threadStatusDeletingActive, "active", threadStatusDeletingArchived, "archived"),
			"updated_at": now,
		}).Error
}

func updateThreadConversationStatus(tx *gorm.DB, thread *model.AgentThread, status string, now time.Time) error {
	if thread == nil || thread.ID == 0 || (status != "active" && status != "archived") {
		return repository.ErrInvalidInput
	}
	if err := tx.Model(thread).Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
		return err
	}
	result := tx.Model(&model.Conversation{}).
		Where("id = ? AND user_id = ? AND execution_type = ?", thread.ConversationID, thread.UserID, "gateway").
		Updates(map[string]any{"status": status, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return repository.ErrConflict
	}
	thread.Status = status
	return nil
}

func finalizeDeletedThreadConversation(tx *gorm.DB, threadID uint, now time.Time) error {
	var thread model.AgentThread
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "conversation_id", "status").First(&thread, threadID).Error; err != nil {
		return err
	}
	if thread.Status != "deleted" {
		return nil
	}
	var pendingEvents int64
	if err := tx.Model(&model.AgentEvent{}).
		Where("thread_id = ? AND conversation_projected_at IS NULL", thread.ID).
		Count(&pendingEvents).Error; err != nil {
		return err
	}
	if pendingEvents != 0 {
		return nil
	}
	return tx.Model(&model.Conversation{}).Where("id = ?", thread.ConversationID).
		Update("deleted_at", now).Error
}

func codexErrorCode(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		value = strings.TrimSpace(value)
		if value != "" && len(value) <= 128 {
			return value
		}
		return ""
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != 1 {
		return ""
	}
	for key := range object {
		key = strings.TrimSpace(key)
		if key != "" && len(key) <= 128 {
			return key
		}
	}
	return ""
}

func updateAgentTurnTerminal(tx *gorm.DB, turnID uint, status, code, message string, updatedAt time.Time) error {
	return tx.Model(&model.AgentTurn{}).
		Where("id = ? AND status NOT IN ?", turnID, []string{"completed", "interrupted", "failed"}).
		Updates(map[string]any{
			"status": status, "error_code": code, "error_message": message, "updated_at": updatedAt,
		}).Error
}

func resolveTurnInteractions(tx *gorm.DB, turnID uint, updatedAt time.Time) error {
	return tx.Model(&model.AgentInteraction{}).
		Where("turn_id = ? AND status IN ?", turnID, []string{"pending", "responding"}).
		Updates(map[string]any{"status": "resolved", "updated_at": updatedAt}).Error
}

func projectInteractionRequest(payloadJSON string) (string, string, error) {
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(payloadJSON), &payload) != nil || len(payload) != 2 {
		return "", "", repository.ErrConflict
	}
	var method string
	if json.Unmarshal(payload["method"], &method) != nil {
		return "", "", repository.ErrConflict
	}
	kind := map[string]string{
		"item/commandExecution/requestApproval": "command_approval",
		"item/fileChange/requestApproval":       "file_approval",
		"item/tool/requestUserInput":            "user_input",
		"item/permissions/requestApproval":      "permission",
		"mcpServer/elicitation/request":         "mcp_elicitation",
		"item/tool/call":                        "dynamic_tool",
	}[method]
	request := payload["request"]
	var object map[string]any
	if kind == "" || len(request) == 0 || json.Unmarshal(request, &object) != nil || object == nil {
		return "", "", repository.ErrConflict
	}
	return kind, string(request), nil
}

func interactionResponseMatchesRequest(kind, requestJSON string, response json.RawMessage) bool {
	var payload map[string]json.RawMessage
	if json.Unmarshal(response, &payload) != nil {
		return false
	}
	var responseKind string
	if json.Unmarshal(payload["kind"], &responseKind) != nil {
		return false
	}
	expected := map[string]string{
		"command_approval": "approval",
		"file_approval":    "approval",
		"user_input":       "user-input",
		"permission":       "permission",
		"mcp_elicitation":  "mcp-elicitation",
		"dynamic_tool":     "dynamic-tool",
	}[kind]
	if expected == "" || responseKind != expected {
		return false
	}
	switch kind {
	case "user_input":
		return userInputResponseMatchesRequest(requestJSON, payload["answers"])
	case "permission":
		return permissionResponseMatchesRequest(requestJSON, payload["decision"], payload["scope"])
	case "mcp_elicitation":
		return mcpResponseMatchesRequest(requestJSON, payload)
	case "dynamic_tool":
		return dynamicToolResponseMatchesRequest(requestJSON, payload["content"])
	default:
		return true
	}
}

func userInputResponseMatchesRequest(requestJSON string, rawAnswers json.RawMessage) bool {
	var request struct {
		Questions []struct {
			QuestionRef   string `json:"questionRef"`
			Required      bool   `json:"required"`
			AllowFreeform bool   `json:"allowFreeform"`
			Options       []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	var answers map[string]string
	if json.Unmarshal([]byte(requestJSON), &request) != nil || len(request.Questions) == 0 ||
		json.Unmarshal(rawAnswers, &answers) != nil || answers == nil {
		return false
	}
	type questionConstraint struct {
		required      bool
		allowFreeform bool
		options       []string
	}
	known := make(map[string]questionConstraint, len(request.Questions))
	for _, question := range request.Questions {
		if _, exists := known[question.QuestionRef]; question.QuestionRef == "" || exists {
			return false
		}
		constraint := questionConstraint{required: question.Required, allowFreeform: question.AllowFreeform}
		for _, option := range question.Options {
			if option.Label == "" || slices.Contains(constraint.options, option.Label) {
				return false
			}
			constraint.options = append(constraint.options, option.Label)
		}
		known[question.QuestionRef] = constraint
	}
	for questionRef, answer := range answers {
		constraint, exists := known[questionRef]
		if !exists || len(constraint.options) > 0 && !constraint.allowFreeform && !slices.Contains(constraint.options, answer) {
			return false
		}
	}
	for questionRef, constraint := range known {
		if constraint.required && strings.TrimSpace(answers[questionRef]) == "" {
			return false
		}
	}
	return true
}

func permissionResponseMatchesRequest(requestJSON string, rawDecision, rawScope json.RawMessage) bool {
	var request struct {
		AllowedScopes []string `json:"allowedScopes"`
	}
	var decision string
	if json.Unmarshal(rawDecision, &decision) != nil {
		return false
	}
	if decision == "decline" {
		return true
	}
	if decision != "accept" || json.Unmarshal([]byte(requestJSON), &request) != nil || len(request.AllowedScopes) == 0 {
		return false
	}
	scope := "turn"
	if len(rawScope) > 0 && json.Unmarshal(rawScope, &scope) != nil {
		return false
	}
	return slices.Contains(request.AllowedScopes, scope)
}

type interactionSchemaField struct {
	Type string            `json:"type"`
	Enum []json.RawMessage `json:"enum"`
}

func mcpResponseMatchesRequest(requestJSON string, payload map[string]json.RawMessage) bool {
	var request struct {
		RequestedSchema *struct {
			Properties map[string]interactionSchemaField `json:"properties"`
			Required   []string                          `json:"required"`
		} `json:"requestedSchema"`
	}
	var decision string
	if json.Unmarshal([]byte(requestJSON), &request) != nil || json.Unmarshal(payload["decision"], &decision) != nil {
		return false
	}
	if decision == "decline" {
		return len(payload["content"]) == 0
	}
	if decision != "accept" {
		return false
	}
	var content map[string]json.RawMessage
	if rawContent := payload["content"]; len(rawContent) > 0 {
		if json.Unmarshal(rawContent, &content) != nil || content == nil {
			return false
		}
	}
	properties := map[string]interactionSchemaField{}
	required := []string{}
	if request.RequestedSchema != nil {
		properties = request.RequestedSchema.Properties
		required = request.RequestedSchema.Required
	}
	for name, value := range content {
		field, exists := properties[name]
		if !exists || !interactionValueMatchesSchema(value, field) {
			return false
		}
	}
	seenRequired := make(map[string]bool, len(required))
	for _, name := range required {
		if _, exists := properties[name]; !exists || seenRequired[name] {
			return false
		}
		seenRequired[name] = true
		if _, exists := content[name]; !exists {
			return false
		}
	}
	return true
}

func interactionValueMatchesSchema(value json.RawMessage, field interactionSchemaField) bool {
	if !interactionValueMatchesType(value, field.Type) {
		return false
	}
	if len(field.Enum) == 0 {
		return true
	}
	for _, candidate := range field.Enum {
		if interactionValuesEqual(value, candidate, field.Type) {
			return true
		}
	}
	return false
}

func interactionValueMatchesType(value json.RawMessage, fieldType string) bool {
	switch fieldType {
	case "string":
		var typed string
		return json.Unmarshal(value, &typed) == nil
	case "boolean":
		var typed bool
		return json.Unmarshal(value, &typed) == nil
	case "number", "integer":
		var typed float64
		if json.Unmarshal(value, &typed) != nil || math.IsInf(typed, 0) || math.IsNaN(typed) {
			return false
		}
		return fieldType == "number" || math.Trunc(typed) == typed
	default:
		return false
	}
}

func interactionValuesEqual(value, candidate json.RawMessage, fieldType string) bool {
	if !interactionValueMatchesType(candidate, fieldType) {
		return false
	}
	switch fieldType {
	case "string":
		var left, right string
		return json.Unmarshal(value, &left) == nil && json.Unmarshal(candidate, &right) == nil && left == right
	case "boolean":
		var left, right bool
		return json.Unmarshal(value, &left) == nil && json.Unmarshal(candidate, &right) == nil && left == right
	default:
		var left, right float64
		return json.Unmarshal(value, &left) == nil && json.Unmarshal(candidate, &right) == nil && left == right
	}
}

func dynamicToolResponseMatchesRequest(requestJSON string, rawContent json.RawMessage) bool {
	var request struct {
		AcceptedContentKinds []string `json:"acceptedContentKinds"`
	}
	var content []struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal([]byte(requestJSON), &request) != nil || json.Unmarshal(rawContent, &content) != nil || content == nil {
		return false
	}
	for _, item := range content {
		if !slices.Contains(request.AcceptedContentKinds, item.Kind) {
			return false
		}
	}
	return true
}

func (r *Repo) BeginRuntimeProof(
	ctx context.Context,
	deviceID uint,
	profilePublicID string,
	profile *domainagent.RuntimeProfile,
	challenge *domainagent.RuntimeProofChallenge,
	now time.Time,
) (*domainagent.RuntimeProfile, *domainagent.RuntimeProofChallenge, error) {
	var runtime model.AgentRuntimeProfile
	var proof model.AgentRuntimeProofChallenge
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		if err := tx.Where("device_id = ? AND public_id = ?", deviceID, profilePublicID).
			Attrs(model.AgentRuntimeProfile{
				PublicID: profilePublicID, UserID: device.UserID, DeviceID: deviceID,
				Provider: profile.Provider, Status: domainagent.RuntimeStatusProving,
			}).FirstOrCreate(&runtime).Error; err != nil {
			return err
		}
		if runtime.UserID != device.UserID || runtime.Provider != profile.Provider {
			return repository.ErrConflict
		}
		if err := tx.Model(&runtime).Updates(map[string]any{
			"status":              domainagent.RuntimeStatusProving,
			"remote_key_id":       nil,
			"credential_hash":     "",
			"verified_at":         nil,
			"lease_expires_at":    nil,
			"presence_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		runtime.Status = domainagent.RuntimeStatusProving
		runtime.RemoteKeyID = nil
		runtime.CredentialHash = ""
		runtime.VerifiedAt = nil
		runtime.LeaseExpiresAt = nil
		proof = model.AgentRuntimeProofChallenge{
			PublicID: challenge.PublicID, UserID: device.UserID, DeviceID: deviceID,
			ProfileID: runtime.ID, Nonce: challenge.Nonce, ExpiresAt: challenge.ExpiresAt,
		}
		return tx.Create(&proof).Error
	})
	if err != nil {
		return nil, nil, errFor(err)
	}
	return toDomainRuntimeProfile(runtime), toDomainRuntimeChallenge(proof), nil
}

func (r *Repo) CompleteRuntimeProof(
	ctx context.Context,
	deviceID, profileID, challengeID uint,
	remoteKeyID int64,
	credentialHash, manifestJSON string,
	now, leaseExpiresAt time.Time,
) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentRuntimeProofChallenge{}).
			Where("id = ? AND device_id = ? AND profile_id = ? AND consumed_at IS NULL AND expires_at > ?", challengeID, deviceID, profileID, now).
			Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrConflict
		}
		result = tx.Model(&model.AgentRuntimeProfile{}).
			Where("id = ? AND device_id = ?", profileID, deviceID).
			Updates(map[string]any{
				"status": domainagent.RuntimeStatusReady, "remote_key_id": remoteKeyID,
				"credential_hash": credentialHash, "manifest_json": manifestJSON, "verified_at": now,
				"lease_expires_at": leaseExpiresAt, "presence_expires_at": nil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrNotFound
		}
		return nil
	}))
}

func (r *Repo) RenewRuntimeLease(ctx context.Context, deviceID, profileID uint, now, leaseExpiresAt, presenceExpiresAt time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AgentRuntimeProfile{}).
			Where("id = ? AND device_id = ? AND status = ? AND lease_expires_at > ?", profileID, deviceID, domainagent.RuntimeStatusReady, now).
			Updates(map[string]any{"lease_expires_at": leaseExpiresAt, "presence_expires_at": presenceExpiresAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrNotFound
		}
		return tx.Model(&model.AgentDevice{}).Where("id = ? AND status = ?", deviceID, domainagent.DeviceStatusActive).
			Update("last_seen_at", now).Error
	}))
}

func (r *Repo) SyncWorkspaces(ctx context.Context, userID, deviceID, profileID uint, items []domainagent.Workspace, now time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND status = ? AND lease_expires_at > ?", profileID, userID, deviceID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		publicIDs := make([]string, 0, len(items))
		for _, item := range items {
			workspace := model.AgentWorkspace{PublicID: item.PublicID, UserID: userID, DeviceID: deviceID, RuntimeProfileID: profileID, Name: item.Name, Managed: item.Managed, Hidden: item.Hidden, Status: "available", LastSeenAt: now}
			if err := tx.Where("device_id = ? AND public_id = ?", deviceID, item.PublicID).Attrs(workspace).FirstOrCreate(&workspace).Error; err != nil {
				return err
			}
			if workspace.UserID != userID || workspace.RuntimeProfileID != profileID {
				return repository.ErrConflict
			}
			if err := tx.Model(&workspace).Updates(map[string]any{"name": item.Name, "managed": item.Managed, "hidden": item.Hidden, "status": "available", "last_seen_at": now}).Error; err != nil {
				return err
			}
			publicIDs = append(publicIDs, item.PublicID)
		}
		stale := tx.Model(&model.AgentWorkspace{}).
			Where("user_id = ? AND device_id = ? AND runtime_profile_id = ?", userID, deviceID, profileID)
		if len(publicIDs) > 0 {
			stale = stale.Where("public_id NOT IN ?", publicIDs)
		}
		return stale.Update("status", "unavailable").Error
	}))
}

func (r *Repo) ListRuntimeProfiles(ctx context.Context, userID uint, devicePublicID string) ([]domainagent.RuntimeProfile, error) {
	var rows []model.AgentRuntimeProfile
	err := r.db.WithContext(ctx).Table("agent_runtime_profiles AS profiles").
		Joins("JOIN agent_devices AS devices ON devices.id = profiles.device_id").
		Where("profiles.user_id = ? AND devices.public_id = ?", userID, devicePublicID).
		Order("profiles.updated_at DESC").Find(&rows).Error
	result := make([]domainagent.RuntimeProfile, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toDomainRuntimeProfile(row))
	}
	return result, errFor(err)
}

func (r *Repo) ListWorkspaces(ctx context.Context, userID uint, devicePublicID string) ([]domainagent.Workspace, error) {
	type row struct {
		model.AgentWorkspace
		ProfilePublicID string    `gorm:"column:profile_public_id"`
		LastActivityAt  time.Time `gorm:"column:last_activity_at"`
	}
	var rows []row
	lastActivitySQL := `COALESCE((
		SELECT MAX(conversations.updated_at)
		FROM chat_conversations AS conversations
		WHERE conversations.user_id = workspaces.user_id
			AND conversations.execution_type = 'gateway'
			AND conversations.execution_device_id = devices.public_id
			AND conversations.execution_workspace_id = workspaces.public_id
			AND conversations.status <> 'archived'
			AND conversations.deleted_at IS NULL
	), workspaces.created_at)`
	err := r.db.WithContext(ctx).Table("agent_workspaces AS workspaces").
		Select("workspaces.*, profiles.public_id AS profile_public_id, "+lastActivitySQL+" AS last_activity_at").
		Joins("JOIN agent_devices AS devices ON devices.id = workspaces.device_id").
		Joins("JOIN agent_runtime_profiles AS profiles ON profiles.id = workspaces.runtime_profile_id").
		Where("workspaces.user_id = ? AND devices.public_id = ? AND workspaces.status = ? AND workspaces.hidden = ?", userID, devicePublicID, "available", false).
		Order("last_activity_at DESC").
		Order("workspaces.name ASC").
		Order("workspaces.public_id ASC").Scan(&rows).Error
	result := make([]domainagent.Workspace, 0, len(rows))
	for _, row := range rows {
		item := toDomainWorkspace(row.AgentWorkspace)
		item.DevicePublicID, item.ProfilePublicID = devicePublicID, row.ProfilePublicID
		item.LastActivityAt = row.LastActivityAt
		result = append(result, *item)
	}
	return result, errFor(err)
}

func (r *Repo) ResolveExecutionTarget(ctx context.Context, userID uint, devicePublicID, profilePublicID, workspacePublicID string, now time.Time) (string, error) {
	var target struct{ Provider string }
	result := r.db.WithContext(ctx).Table("agent_workspaces AS workspaces").
		Select("profiles.provider AS provider").
		Joins("JOIN agent_devices AS devices ON devices.id = workspaces.device_id").
		Joins("JOIN agent_runtime_profiles AS profiles ON profiles.id = workspaces.runtime_profile_id").
		Where("workspaces.user_id = ? AND devices.public_id = ? AND devices.status = ?", userID, devicePublicID, domainagent.DeviceStatusActive).
		Where("profiles.public_id = ? AND profiles.status = ? AND profiles.lease_expires_at > ?", profilePublicID, domainagent.RuntimeStatusReady, now).
		Where("workspaces.public_id = ? AND workspaces.status = ?", workspacePublicID, "available").
		Limit(1).Scan(&target)
	if result.Error != nil {
		return "", errFor(result.Error)
	}
	if result.RowsAffected != 1 || strings.TrimSpace(target.Provider) == "" {
		return "", repository.ErrNotFound
	}
	return target.Provider, nil
}

func (r *Repo) CreateArtifact(ctx context.Context, userID uint, workspacePublicID, fileID string, input *domainagent.Artifact) (*domainagent.Artifact, error) {
	var artifact model.AgentArtifact
	var workspaceID uint
	var resolvedFileID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var workspace model.AgentWorkspace
		if err := tx.Where("user_id = ? AND public_id = ? AND status = ?", userID, workspacePublicID, "available").First(&workspace).Error; err != nil {
			return err
		}
		var file model.FileObject
		if err := tx.Where("user_id = ? AND file_id = ? AND status = ?", userID, fileID, "active").First(&file).Error; err != nil {
			return err
		}
		mimeType := strings.TrimSpace(file.DetectedMIME)
		if mimeType == "" {
			mimeType = strings.TrimSpace(file.MimeType)
		}
		if mimeType == "" || file.SizeBytes < 1 || file.SizeBytes > 100*1024*1024 {
			return repository.ErrInvalidInput
		}
		created := model.AgentArtifact{
			PublicID: input.PublicID, UserID: userID, WorkspaceID: workspace.ID,
			FileObjectID: file.ID, FileName: file.FileName, MimeType: mimeType,
			SizeBytes: file.SizeBytes, SHA256: file.SHA256, Status: "ready",
		}
		if err := tx.Where("workspace_id = ? AND file_object_id = ?", workspace.ID, file.ID).
			Attrs(created).FirstOrCreate(&artifact).Error; err != nil {
			return err
		}
		workspaceID, resolvedFileID = workspace.ID, file.FileID
		return nil
	})
	if err != nil {
		return nil, errFor(err)
	}
	result := toDomainArtifact(artifact)
	result.WorkspaceID, result.WorkspacePublicID, result.FileID = workspaceID, workspacePublicID, resolvedFileID
	return result, nil
}

func (r *Repo) ListArtifactsForCommand(ctx context.Context, deviceID, commandID uint, refs []string) ([]domainagent.Artifact, error) {
	if len(refs) == 0 {
		return []domainagent.Artifact{}, nil
	}
	var command model.AgentCommand
	if err := r.db.WithContext(ctx).Where("id = ? AND device_id = ?", commandID, deviceID).First(&command).Error; err != nil {
		return nil, errFor(err)
	}
	if command.WorkspaceID == nil {
		return nil, repository.ErrConflict
	}
	type row struct {
		model.AgentArtifact
		FileID string `gorm:"column:file_id"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("agent_artifacts AS artifacts").
		Select("artifacts.*, files.file_id").
		Joins("JOIN file_objects AS files ON files.id = artifacts.file_object_id").
		Where("artifacts.user_id = ? AND artifacts.workspace_id = ? AND artifacts.status = ? AND artifacts.public_id IN ?", command.UserID, *command.WorkspaceID, "ready", refs).
		Find(&rows).Error
	if err != nil {
		return nil, errFor(err)
	}
	if len(rows) != len(refs) {
		return nil, repository.ErrConflict
	}
	byRef := make(map[string]domainagent.Artifact, len(rows))
	for _, row := range rows {
		item := toDomainArtifact(row.AgentArtifact)
		item.FileID = row.FileID
		byRef[item.PublicID] = *item
	}
	result := make([]domainagent.Artifact, 0, len(refs))
	for _, ref := range refs {
		item, ok := byRef[ref]
		if !ok {
			return nil, repository.ErrConflict
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repo) GetArtifactForCommand(ctx context.Context, artifactPublicID, commandPublicID string) (*domainagent.Artifact, *domainagent.Command, error) {
	var command model.AgentCommand
	if err := r.db.WithContext(ctx).Where("public_id = ?", commandPublicID).First(&command).Error; err != nil {
		return nil, nil, errFor(err)
	}
	if command.WorkspaceID == nil {
		return nil, nil, repository.ErrConflict
	}
	type row struct {
		model.AgentArtifact
		FileID string `gorm:"column:file_id"`
	}
	var value row
	err := r.db.WithContext(ctx).Table("agent_artifacts AS artifacts").
		Select("artifacts.*, files.file_id").
		Joins("JOIN file_objects AS files ON files.id = artifacts.file_object_id").
		Where("artifacts.public_id = ? AND artifacts.user_id = ? AND artifacts.workspace_id = ? AND artifacts.status = ?", artifactPublicID, command.UserID, *command.WorkspaceID, "ready").
		First(&value).Error
	if err != nil {
		return nil, nil, errFor(err)
	}
	artifact := toDomainArtifact(value.AgentArtifact)
	artifact.FileID = value.FileID
	return artifact, toDomainCommand(command), nil
}

func (r *Repo) QueueResourceRefresh(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	devicePublicID, profilePublicID, workspacePublicID, resourceName string,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Command, error) {
	var created model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operationName := "resource.refresh.profile." + resourceName
		if workspacePublicID != "" {
			operationName = "resource.refresh.workspace." + resourceName
		}
		var operation model.AgentIdempotencyRecord
		claim := model.AgentIdempotencyRecord{UserID: userID, Operation: operationName, Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, operationName, idempotencyKey).
			Attrs(claim).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&created).Error
		}

		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND public_id = ? AND status = ?", userID, devicePublicID, domainagent.DeviceStatusActive).
			First(&device).Error; err != nil {
			return err
		}
		var profile model.AgentRuntimeProfile
		var workspaceID *uint
		if workspacePublicID == "" {
			if err := tx.Where("user_id = ? AND device_id = ? AND public_id = ? AND status = ? AND lease_expires_at > ?", userID, device.ID, profilePublicID, domainagent.RuntimeStatusReady, now).
				First(&profile).Error; err != nil {
				return err
			}
		} else {
			var workspace model.AgentWorkspace
			if err := tx.Where("user_id = ? AND device_id = ? AND public_id = ? AND status = ?", userID, device.ID, workspacePublicID, "available").First(&workspace).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ? AND status = ? AND lease_expires_at > ?", workspace.RuntimeProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
				return err
			}
			workspaceID = &workspace.ID
		}
		pending := tx.Where(
			"device_id = ? AND runtime_profile_id = ? AND kind = ? AND completed_at IS NULL AND payload_json -> 'resource' ->> 'name' = ?",
			device.ID, profile.ID, "resource.refresh", resourceName,
		)
		if workspaceID == nil {
			pending = pending.Where("workspace_id IS NULL")
		} else {
			pending = pending.Where("workspace_id = ?", *workspaceID)
		}
		if err := pending.Order("server_seq DESC").First(&created).Error; err == nil {
			return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
		} else if !dberror.IsRecordNotFound(err) {
			return err
		}
		payload := map[string]any{
			"kind": "resource.refresh", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"resource": map[string]string{"scope": "profile", "name": resourceName},
		}
		if workspaceID != nil {
			payload["workspaceId"] = workspacePublicID
			payload["resource"] = map[string]string{"scope": "workspace", "name": resourceName}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: workspaceID, ServerSeq: device.NextServerSeq,
			Kind: "resource.refresh", PayloadJSON: string(encoded), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(created), nil
}

func (r *Repo) QueueAgentUpdate(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	devicePublicID, targetVersion string,
	command *domainagent.Command,
	_ time.Time,
) (*domainagent.Command, error) {
	var created model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		claim := model.AgentIdempotencyRecord{UserID: userID, Operation: "agent.update", Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, claim.Operation, idempotencyKey).
			Attrs(claim).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&created).Error
		}

		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND public_id = ? AND status = ?", userID, devicePublicID, domainagent.DeviceStatusActive).
			First(&device).Error; err != nil {
			return err
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where(
			"user_id = ? AND device_id = ? AND status = ? AND manifest_json @> CAST(? AS jsonb)",
			userID, device.ID, domainagent.RuntimeStatusReady, `{"commands":["agent.update"]}`,
		).Order("verified_at DESC NULLS LAST, id DESC").First(&profile).Error; err != nil {
			return err
		}
		if err := tx.Where("device_id = ? AND kind = ? AND completed_at IS NULL", device.ID, "agent.update").
			Order("server_seq DESC").First(&created).Error; err == nil {
			return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
		} else if !dberror.IsRecordNotFound(err) {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "agent.update", "deviceId": device.PublicID, "profileId": profile.PublicID, "targetVersion": targetVersion,
		})
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID, RuntimeProfileID: &profile.ID,
			ServerSeq: device.NextServerSeq, Kind: "agent.update", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(created), nil
}

func (r *Repo) QueueWorkspaceRegistration(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	devicePublicID, profilePublicID, path string,
	create bool,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Command, error) {
	var created model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		claim := model.AgentIdempotencyRecord{UserID: userID, Operation: "workspace.register", Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, claim.Operation, idempotencyKey).
			Attrs(claim).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&created).Error
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND public_id = ? AND status = ?", userID, devicePublicID, domainagent.DeviceStatusActive).
			First(&device).Error; err != nil {
			return err
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("user_id = ? AND device_id = ? AND public_id = ? AND status = ? AND lease_expires_at > ?", userID, device.ID, profilePublicID, domainagent.RuntimeStatusReady, now).
			First(&profile).Error; err != nil {
			return err
		}
		if !manifestSupportsCommand(profile.ManifestJSON, "workspace.register") {
			return repository.ErrConflict
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "workspace.register", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"path": path, "create": create,
		})
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID, RuntimeProfileID: &profile.ID,
			ServerSeq: device.NextServerSeq, Kind: command.Kind, PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(created), nil
}

func (r *Repo) QueueWorkspaceMutation(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	devicePublicID, workspacePublicID, kind, name string,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Command, error) {
	if kind != "workspace.rename" && kind != "workspace.unregister" {
		return nil, repository.ErrInvalidInput
	}
	var created model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		claim := model.AgentIdempotencyRecord{UserID: userID, Operation: kind, Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, kind, idempotencyKey).
			Attrs(claim).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&created).Error
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND public_id = ? AND status = ?", userID, devicePublicID, domainagent.DeviceStatusActive).
			First(&device).Error; err != nil {
			return err
		}
		var workspaceLookup model.AgentWorkspace
		if err := tx.Where("user_id = ? AND device_id = ? AND public_id = ? AND status = ?", userID, device.ID, workspacePublicID, "available").
			First(&workspaceLookup).Error; err != nil {
			return err
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND device_id = ? AND status = ? AND lease_expires_at > ?", workspaceLookup.RuntimeProfileID, userID, device.ID, domainagent.RuntimeStatusReady, now).
			First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND device_id = ? AND runtime_profile_id = ? AND status = ?", workspaceLookup.ID, userID, device.ID, profile.ID, "available").
			First(&workspace).Error; err != nil {
			return err
		}
		if !manifestSupportsCommand(profile.ManifestJSON, kind) {
			return repository.ErrConflict
		}
		payload := map[string]any{
			"kind": kind, "deviceId": device.PublicID, "profileId": profile.PublicID, "workspaceId": workspace.PublicID,
		}
		if kind == "workspace.rename" {
			payload["name"] = name
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID,
			ServerSeq: device.NextServerSeq, Kind: kind, PayloadJSON: string(encoded), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(created), nil
}

func (r *Repo) GetResourceSnapshot(ctx context.Context, userID uint, devicePublicID, profilePublicID, workspacePublicID, resourceName string) (*domainagent.ResourceSnapshot, error) {
	type row struct {
		model.AgentResourceSnapshot
		DevicePublicID, ProfilePublicID, WorkspacePublicID string
	}
	var value row
	query := r.db.WithContext(ctx).Table("agent_resource_snapshots AS snapshots").
		Select("snapshots.*, devices.public_id AS device_public_id, profiles.public_id AS profile_public_id, COALESCE(workspaces.public_id, '') AS workspace_public_id").
		Joins("JOIN agent_devices AS devices ON devices.id = snapshots.device_id").
		Joins("JOIN agent_runtime_profiles AS profiles ON profiles.id = snapshots.runtime_profile_id").
		Joins("LEFT JOIN agent_workspaces AS workspaces ON workspaces.id = snapshots.workspace_id").
		Where("snapshots.user_id = ? AND devices.public_id = ? AND snapshots.name = ?", userID, devicePublicID, resourceName)
	if workspacePublicID == "" {
		query = query.Where("profiles.public_id = ? AND snapshots.workspace_id = 0", profilePublicID)
	} else {
		query = query.Where("workspaces.public_id = ?", workspacePublicID)
	}
	if err := query.First(&value).Error; err != nil {
		return nil, errFor(err)
	}
	result := toDomainResourceSnapshot(value.AgentResourceSnapshot)
	result.DevicePublicID, result.ProfilePublicID, result.WorkspacePublicID = value.DevicePublicID, value.ProfilePublicID, value.WorkspacePublicID
	return result, nil
}

func (r *Repo) QueueTurnInterrupt(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	turnPublicID string,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Command, error) {
	var created model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		claim := model.AgentIdempotencyRecord{UserID: userID, Operation: command.Kind, Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, command.Kind, idempotencyKey).
			Attrs(claim).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&created).Error
		}

		var turnTarget model.AgentTurn
		if err := tx.Select("id", "thread_id").Where("user_id = ? AND public_id = ?", userID, turnPublicID).First(&turnTarget).Error; err != nil {
			return err
		}
		var threadTarget model.AgentThread
		if err := tx.Select("id", "device_id").First(&threadTarget, turnTarget.ThreadID).Error; err != nil {
			return err
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", threadTarget.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		var thread model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND device_id = ?", threadTarget.ID, userID, device.ID).First(&thread).Error; err != nil {
			return err
		}
		var turn model.AgentTurn
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND thread_id = ?", turnTarget.ID, userID, thread.ID).First(&turn).Error; err != nil {
			return err
		}
		if thread.SourceThreadRef == nil || turn.SourceTurnRef == nil || thread.Status != "active" || turn.Status != "running" {
			return repository.ErrConflict
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND status = ? AND lease_expires_at > ?", thread.RuntimeProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.Where("id = ? AND status = ?", thread.WorkspaceID, "available").First(&workspace).Error; err != nil {
			return err
		}
		payload := map[string]any{
			"kind": "turn.interrupt", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID, "sourceThreadRef": *thread.SourceThreadRef,
			"turnId": turn.PublicID, "sourceTurnRef": *turn.SourceTurnRef,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, TurnID: &turn.ID,
			ServerSeq: device.NextServerSeq, Kind: "turn.interrupt", PayloadJSON: string(encoded), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := updateAgentTurnTerminal(tx, turn.ID, "interrupted", "", "", now); err != nil {
			return err
		}
		if err := resolveTurnInteractions(tx, turn.ID, now); err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", created.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(created), nil
}

func (r *Repo) QueueThreadLifecycle(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	threadPublicID string,
	action string,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Command, error) {
	if action != "archive" && action != "unarchive" && action != "delete" {
		return nil, repository.ErrInvalidInput
	}
	var created model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		claim := model.AgentIdempotencyRecord{UserID: userID, Operation: command.Kind, Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, command.Kind, idempotencyKey).
			Attrs(claim).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&created).Error
		}

		var target model.AgentThread
		if err := tx.Select("id", "device_id").Where("user_id = ? AND public_id = ?", userID, threadPublicID).First(&target).Error; err != nil {
			return err
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", target.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		var thread model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND device_id = ?", target.ID, userID, device.ID).First(&thread).Error; err != nil {
			return err
		}
		if thread.SourceThreadRef == nil ||
			(action == "archive" && thread.Status != "active") ||
			(action == "unarchive" && thread.Status != "archived") ||
			(action == "delete" && thread.Status != "active" && thread.Status != "archived") {
			return repository.ErrConflict
		}
		var pendingLifecycle int64
		if err := tx.Model(&model.AgentCommand{}).
			Where("thread_id = ? AND kind = ? AND completed_at IS NULL", thread.ID, "thread.lifecycle").
			Count(&pendingLifecycle).Error; err != nil {
			return err
		}
		if pendingLifecycle > 0 {
			return repository.ErrConflict
		}
		var activeTurns int64
		if err := tx.Model(&model.AgentTurn{}).Where("thread_id = ? AND status IN ?", thread.ID, []string{"awaiting_thread", "queued", "running"}).Count(&activeTurns).Error; err != nil {
			return err
		}
		if activeTurns > 0 {
			return repository.ErrConflict
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND status = ?", thread.RuntimeProfileID, domainagent.RuntimeStatusReady).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.First(&workspace, thread.WorkspaceID).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "thread.lifecycle", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID,
			"sourceThreadRef": *thread.SourceThreadRef, "action": action,
		})
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID,
			ServerSeq: device.NextServerSeq, Kind: command.Kind, PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		if err := tx.Model(&operation).Update("result_public_id", created.PublicID).Error; err != nil {
			return err
		}
		if action == "delete" {
			deletingStatus := threadStatusDeletingActive
			if thread.Status == "archived" {
				deletingStatus = threadStatusDeletingArchived
			}
			return tx.Model(&thread).Updates(map[string]any{"status": deletingStatus, "updated_at": now}).Error
		}
		nextStatus := "active"
		if action == "archive" {
			nextStatus = "archived"
		}
		return updateThreadConversationStatus(tx, &thread, nextStatus, now)
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(created), nil
}

func updateGatewayConversationSettings(tx *gorm.DB, userID, conversationID uint, provider string, raw json.RawMessage) error {
	var settings struct {
		Model             string `json:"model"`
		ReasoningEffort   string `json:"reasoningEffort"`
		ApprovalPolicy    string `json:"approvalPolicy"`
		ApprovalsReviewer string `json:"approvalsReviewer"`
		SandboxPolicy     string `json:"sandboxPolicy"`
	}
	if json.Unmarshal(raw, &settings) != nil || len(settings.Model) > 128 || len(settings.ReasoningEffort) > 32 ||
		!validRepoApprovalMode(settings.ApprovalPolicy, settings.ApprovalsReviewer, settings.SandboxPolicy) {
		return repository.ErrInvalidInput
	}
	result := tx.Model(&model.Conversation{}).
		Where("id = ? AND user_id = ? AND execution_type = ?", conversationID, userID, "gateway").
		Updates(map[string]any{
			"model": strings.TrimSpace(settings.Model), "reasoning_effort": strings.TrimSpace(settings.ReasoningEffort),
			"approval_policy": settings.ApprovalPolicy, "approvals_reviewer": settings.ApprovalsReviewer, "sandbox_policy": settings.SandboxPolicy,
			"provider": strings.TrimSpace(provider),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return repository.ErrConflict
	}
	return nil
}

func (r *Repo) StartThread(ctx context.Context, idempotencyKey, requestHash string, input *domainagent.Thread, initialTurn *domainagent.Turn, command *domainagent.Command, now time.Time) (*domainagent.Thread, *domainagent.Turn, error) {
	var thread model.AgentThread
	var turn *model.AgentTurn
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		created := model.AgentIdempotencyRecord{UserID: input.UserID, Operation: "thread.create", Key: idempotencyKey, RequestHash: requestHash}
		result := tx.Where("user_id = ? AND operation = ? AND key = ?", input.UserID, created.Operation, idempotencyKey).Attrs(created).FirstOrCreate(&operation)
		if result.Error != nil {
			return result.Error
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			if err := tx.Where("user_id = ? AND public_id = ?", input.UserID, operation.ResultPublicID).First(&thread).Error; err != nil {
				return err
			}
			if operation.SecondaryPublicID != "" {
				var existing model.AgentTurn
				if err := tx.Where("user_id = ? AND public_id = ?", input.UserID, operation.SecondaryPublicID).First(&existing).Error; err != nil {
					return err
				}
				turn = &existing
			}
			return nil
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ? AND status = ?", input.UserID, commandDeviceID(command.PayloadJSON), domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		var target struct {
			ProfileID   string          `json:"profileId"`
			WorkspaceID string          `json:"workspaceId"`
			Settings    json.RawMessage `json:"settings"`
		}
		if err := json.Unmarshal([]byte(command.PayloadJSON), &target); err != nil {
			return repository.ErrInvalidInput
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("user_id = ? AND device_id = ? AND public_id = ? AND status = ? AND lease_expires_at > ?", input.UserID, device.ID, target.ProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if !manifestSupportsSettings(profile.ManifestJSON, string(target.Settings)) {
			return repository.ErrConflict
		}
		if err := tx.Where("user_id = ? AND device_id = ? AND runtime_profile_id = ? AND public_id = ? AND status = ?", input.UserID, device.ID, profile.ID, target.WorkspaceID, "available").First(&workspace).Error; err != nil {
			return err
		}
		if err := updateGatewayConversationSettings(tx, input.UserID, input.ConversationID, profile.Provider, target.Settings); err != nil {
			return err
		}
		if initialTurn != nil {
			if err := validateCommandArtifacts(tx, input.UserID, workspace.ID, initialTurn.InputJSON); err != nil {
				return err
			}
		}
		var existing model.AgentThread
		existingErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND conversation_id = ?", input.UserID, input.ConversationID).First(&existing).Error
		switch {
		case existingErr == nil:
			if existing.Status != "failed" || existing.SourceThreadRef != nil || existing.DeviceID != device.ID ||
				existing.RuntimeProfileID != profile.ID || existing.WorkspaceID != workspace.ID {
				return repository.ErrConflict
			}
			var failedTurn model.AgentTurn
			if err := tx.Where("thread_id = ? AND status = ?", existing.ID, "failed").Order("id DESC").First(&failedTurn).Error; err != nil {
				return err
			}
			if !retriableThreadCreateFailure(failedTurn.ErrorCode) {
				return repository.ErrConflict
			}
			thread = existing
			if err := tx.Model(&thread).Updates(map[string]any{"status": input.Status, "title": input.Title, "updated_at": now}).Error; err != nil {
				return err
			}
		case errors.Is(existingErr, gorm.ErrRecordNotFound):
			thread = model.AgentThread{PublicID: input.PublicID, UserID: input.UserID, DeviceID: device.ID, RuntimeProfileID: profile.ID, WorkspaceID: workspace.ID, ConversationID: input.ConversationID, Title: input.Title, Status: input.Status}
			if err := tx.Create(&thread).Error; err != nil {
				return err
			}
		default:
			return existingErr
		}
		if initialTurn != nil {
			createdTurn := model.AgentTurn{PublicID: initialTurn.PublicID, UserID: input.UserID, ThreadID: thread.ID, RunID: initialTurn.RunID, Status: initialTurn.Status, InputJSON: initialTurn.InputJSON, SettingsJSON: initialTurn.SettingsJSON}
			if err := tx.Create(&createdTurn).Error; err != nil {
				return err
			}
			turn = &createdTurn
		}
		commandRow := model.AgentCommand{PublicID: command.PublicID, UserID: input.UserID, DeviceID: device.ID, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, ServerSeq: device.NextServerSeq, Kind: command.Kind, PayloadJSON: command.PayloadJSON, State: "queued", TerminalJSON: "{}"}
		if turn != nil {
			commandRow.TurnID = &turn.ID
		}
		if err := tx.Create(&commandRow).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		secondary := ""
		if turn != nil {
			secondary = turn.PublicID
		}
		return tx.Model(&operation).Updates(map[string]any{"result_public_id": thread.PublicID, "secondary_public_id": secondary}).Error
	})
	if err != nil {
		return nil, nil, errFor(err)
	}
	resultThread := toDomainThread(thread)
	var resultTurn *domainagent.Turn
	if turn != nil {
		resultTurn = toDomainTurn(*turn)
	}
	return resultThread, resultTurn, nil
}

func retriableThreadCreateFailure(code string) bool {
	code = strings.TrimSpace(code)
	return code != "" && code != "timeout" && code != "outcome_unknown"
}

func commandDeviceID(payload string) string {
	var target struct {
		DeviceID string `json:"deviceId"`
	}
	_ = json.Unmarshal([]byte(payload), &target)
	return target.DeviceID
}

func validateCommandArtifacts(tx *gorm.DB, userID, workspaceID uint, inputJSON string) error {
	var input []struct {
		Kind        string `json:"kind"`
		ArtifactRef string `json:"artifactRef"`
	}
	if json.Unmarshal([]byte(inputJSON), &input) != nil {
		return repository.ErrInvalidInput
	}
	refs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range input {
		if item.Kind != "artifact" {
			continue
		}
		if !validArtifactRef(item.ArtifactRef) {
			return repository.ErrInvalidInput
		}
		if _, exists := seen[item.ArtifactRef]; exists {
			continue
		}
		seen[item.ArtifactRef] = struct{}{}
		refs = append(refs, item.ArtifactRef)
	}
	if len(refs) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&model.AgentArtifact{}).
		Where("user_id = ? AND workspace_id = ? AND status = ? AND public_id IN ?", userID, workspaceID, "ready", refs).
		Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(refs)) {
		return repository.ErrInvalidInput
	}
	return nil
}

func validArtifactRef(value string) bool {
	if len(value) != len("agart_")+32 || !strings.HasPrefix(value, "agart_") {
		return false
	}
	for _, character := range value[len("agart_"):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (r *Repo) GetThreadByConversation(ctx context.Context, userID, conversationID uint) (*domainagent.Thread, error) {
	type row struct {
		model.AgentThread
		DevicePublicID, ProfilePublicID, WorkspacePublicID string
	}
	var value row
	err := r.db.WithContext(ctx).Table("agent_threads AS threads").
		Select("threads.*, devices.public_id AS device_public_id, profiles.public_id AS profile_public_id, workspaces.public_id AS workspace_public_id").
		Joins("JOIN agent_devices AS devices ON devices.id = threads.device_id").
		Joins("JOIN agent_runtime_profiles AS profiles ON profiles.id = threads.runtime_profile_id").
		Joins("JOIN agent_workspaces AS workspaces ON workspaces.id = threads.workspace_id").
		Where("threads.user_id = ? AND threads.conversation_id = ?", userID, conversationID).First(&value).Error
	if err != nil {
		return nil, errFor(err)
	}
	item := toDomainThread(value.AgentThread)
	item.DevicePublicID, item.ProfilePublicID, item.WorkspacePublicID = value.DevicePublicID, value.ProfilePublicID, value.WorkspacePublicID
	return item, nil
}

func (r *Repo) QueueThreadHistory(ctx context.Context, userID, conversationID uint, command *domainagent.Command, now time.Time) (*domainagent.Thread, *domainagent.Command, error) {
	if command == nil || command.Kind != "thread.read" {
		return nil, nil, repository.ErrInvalidInput
	}
	var target model.AgentThread
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND conversation_id = ?", userID, conversationID).First(&target).Error; err != nil {
		return nil, nil, errFor(err)
	}
	if threadHistoryLoaded(target) {
		target.HistoryStatus = "loaded"
		return toDomainThread(target), nil, nil
	}
	var thread model.AgentThread
	var queued *model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND status = ?", target.DeviceID, userID, domainagent.DeviceStatusActive).
			First(&device).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND conversation_id = ?", target.ID, userID, conversationID).
			First(&thread).Error; err != nil {
			return err
		}
		if threadHistoryLoaded(thread) {
			thread.HistoryStatus = "loaded"
			return nil
		}
		var pending model.AgentCommand
		err := tx.Where("thread_id = ? AND kind = ? AND completed_at IS NULL", thread.ID, "thread.read").
			Order("id DESC").First(&pending).Error
		if err == nil {
			queued = &pending
			return nil
		}
		if !dberror.IsRecordNotFound(err) {
			return err
		}
		if thread.SourceThreadRef == nil || !validRepoRef(*thread.SourceThreadRef) ||
			(thread.Status != "active" && thread.Status != "archived") {
			return repository.ErrConflict
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND status = ? AND lease_expires_at > ?",
			thread.RuntimeProfileID, userID, device.ID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		if !manifestSupportsCommand(profile.ManifestJSON, "thread.read") {
			return repository.ErrConflict
		}
		var workspace model.AgentWorkspace
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND runtime_profile_id = ? AND status = ?",
			thread.WorkspaceID, userID, device.ID, profile.ID, "available").First(&workspace).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "thread.read", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID, "sourceThreadRef": *thread.SourceThreadRef,
		})
		if err != nil {
			return err
		}
		created := model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID,
			ServerSeq: device.NextServerSeq, Kind: "thread.read", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		if err := tx.Model(&thread).Updates(map[string]any{"history_status": "loading", "history_error": "", "updated_at": now}).Error; err != nil {
			return err
		}
		thread.HistoryStatus, thread.HistoryError = "loading", ""
		queued = &created
		return nil
	})
	if err != nil {
		return nil, nil, errFor(err)
	}
	var resultCommand *domainagent.Command
	if queued != nil {
		resultCommand = toDomainCommand(*queued)
	}
	return toDomainThread(thread), resultCommand, nil
}

func threadHistoryLoaded(thread model.AgentThread) bool {
	return (thread.HistoryStatus == "" || thread.HistoryStatus == "loaded") && thread.HistoryVersion >= historyProjectionVersion
}

func (r *Repo) StartTurn(ctx context.Context, idempotencyKey, requestHash string, input *domainagent.Turn, command *domainagent.Command, now time.Time) (*domainagent.Turn, error) {
	var turn model.AgentTurn
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		created := model.AgentIdempotencyRecord{UserID: input.UserID, Operation: "turn.start", Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", input.UserID, created.Operation, idempotencyKey).Attrs(created).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", input.UserID, operation.ResultPublicID).First(&turn).Error
		}
		var target model.AgentThread
		if err := tx.Select("id", "device_id").Where("user_id = ? AND public_id = ?", input.UserID, input.ThreadPublicID).First(&target).Error; err != nil {
			return err
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", target.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		var thread model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND device_id = ?", target.ID, input.UserID, device.ID).First(&thread).Error; err != nil {
			return err
		}
		if thread.SourceThreadRef == nil || thread.Status != "active" ||
			(thread.HistoryStatus != "" && thread.HistoryStatus != "loaded") {
			return repository.ErrConflict
		}
		var activeTurns int64
		if err := tx.Model(&model.AgentTurn{}).Where("thread_id = ? AND status IN ?", thread.ID, []string{"awaiting_thread", "queued", "running"}).Count(&activeTurns).Error; err != nil {
			return err
		}
		if activeTurns > 0 {
			return repository.ErrConflict
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND status = ? AND lease_expires_at > ?", thread.RuntimeProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if !manifestSupportsSettings(profile.ManifestJSON, input.SettingsJSON) {
			return repository.ErrConflict
		}
		if err := tx.First(&workspace, thread.WorkspaceID).Error; err != nil {
			return err
		}
		if err := validateCommandArtifacts(tx, input.UserID, workspace.ID, input.InputJSON); err != nil {
			return err
		}
		if err := updateGatewayConversationSettings(tx, input.UserID, thread.ConversationID, profile.Provider, json.RawMessage(input.SettingsJSON)); err != nil {
			return err
		}
		turn = model.AgentTurn{PublicID: input.PublicID, UserID: input.UserID, ThreadID: thread.ID, RunID: input.RunID, Status: input.Status, InputJSON: input.InputJSON, SettingsJSON: input.SettingsJSON}
		if err := tx.Create(&turn).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "turn.start", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID,
			"sourceThreadRef": *thread.SourceThreadRef, "input": json.RawMessage(input.InputJSON),
			"settings": json.RawMessage(input.SettingsJSON),
		})
		if err != nil {
			return err
		}
		commandRow := model.AgentCommand{PublicID: command.PublicID, UserID: input.UserID, DeviceID: device.ID, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, TurnID: &turn.ID, ServerSeq: device.NextServerSeq, Kind: "turn.start", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}"}
		if err := tx.Create(&commandRow).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", turn.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainTurn(turn), nil
}

func (r *Repo) QueueTurnSteer(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	runID string,
	input json.RawMessage,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Command, error) {
	var queued model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		created := model.AgentIdempotencyRecord{UserID: userID, Operation: "turn.steer", Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, created.Operation, idempotencyKey).
			Attrs(created).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&queued).Error
		}

		var turn model.AgentTurn
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND run_id = ?", userID, runID).First(&turn).Error; err != nil {
			return err
		}
		if turn.Status != "running" || turn.SourceTurnRef == nil || !validRepoRef(*turn.SourceTurnRef) {
			return repository.ErrConflict
		}
		var thread model.AgentThread
		if err := tx.Where("id = ? AND user_id = ? AND status = ?", turn.ThreadID, userID, "active").First(&thread).Error; err != nil {
			return err
		}
		if thread.SourceThreadRef == nil || !validRepoRef(*thread.SourceThreadRef) {
			return repository.ErrConflict
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND status = ?", thread.DeviceID, userID, domainagent.DeviceStatusActive).
			First(&device).Error; err != nil {
			return err
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND status = ? AND lease_expires_at > ?",
			thread.RuntimeProfileID, userID, device.ID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		if !manifestSupportsCommand(profile.ManifestJSON, "turn.steer") {
			return repository.ErrConflict
		}
		var workspace model.AgentWorkspace
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND runtime_profile_id = ? AND status = ?",
			thread.WorkspaceID, userID, device.ID, profile.ID, "available").First(&workspace).Error; err != nil {
			return err
		}
		if err := validateCommandArtifacts(tx, userID, workspace.ID, string(input)); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "turn.steer", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID,
			"sourceThreadRef": *thread.SourceThreadRef, "turnId": turn.PublicID,
			"sourceTurnRef": *turn.SourceTurnRef, "input": input,
		})
		if err != nil {
			return err
		}
		queued = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, TurnID: &turn.ID,
			ServerSeq: device.NextServerSeq, Kind: "turn.steer", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&queued).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&operation).Update("result_public_id", queued.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(queued), nil
}

func (r *Repo) GetTurnByRunID(ctx context.Context, userID uint, runID string) (*domainagent.Turn, error) {
	var row model.AgentTurn
	if err := r.db.WithContext(ctx).Where("user_id = ? AND run_id = ?", userID, runID).First(&row).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainTurn(row), nil
}

func (r *Repo) ListInteractions(ctx context.Context, userID uint, threadPublicID, status string, limit int) ([]domainagent.Interaction, error) {
	type row struct {
		model.AgentInteraction
		TurnPublicID string `gorm:"column:turn_public_id"`
		RunID        string `gorm:"column:run_id"`
	}
	var rows []row
	query := r.db.WithContext(ctx).Table("agent_interactions AS interactions").
		Select("interactions.*, COALESCE(turns.public_id, '') AS turn_public_id, COALESCE(turns.run_id, '') AS run_id").
		Joins("JOIN agent_threads AS threads ON threads.id = interactions.thread_id").
		Joins("LEFT JOIN agent_turns AS turns ON turns.id = interactions.turn_id").
		Where("interactions.user_id = ? AND threads.public_id = ?", userID, threadPublicID)
	if status != "" {
		query = query.Where("interactions.status = ?", status)
	}
	err := query.Order("interactions.id ASC").Limit(limit).Find(&rows).Error
	result := make([]domainagent.Interaction, 0, len(rows))
	for _, row := range rows {
		item := toDomainInteraction(row.AgentInteraction)
		item.TurnPublicID, item.RunID = row.TurnPublicID, row.RunID
		result = append(result, *item)
	}
	return result, errFor(err)
}

func (r *Repo) RespondInteraction(ctx context.Context, idempotencyKey, requestHash string, userID uint, interactionPublicID string, response json.RawMessage, command *domainagent.Command, now time.Time) (*domainagent.Interaction, error) {
	var interaction model.AgentInteraction
	var thread model.AgentThread
	var turnPublicID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var operation model.AgentIdempotencyRecord
		created := model.AgentIdempotencyRecord{UserID: userID, Operation: "interaction.respond", Key: idempotencyKey, RequestHash: requestHash}
		if err := tx.Where("user_id = ? AND operation = ? AND key = ?", userID, created.Operation, idempotencyKey).Attrs(created).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if operation.RequestHash != requestHash {
			return repository.ErrConflict
		}
		if operation.ResultPublicID != "" {
			if err := tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&interaction).Error; err != nil {
				return err
			}
			if err := tx.First(&thread, interaction.ThreadID).Error; err != nil {
				return err
			}
			if interaction.TurnID != nil {
				var turn model.AgentTurn
				if err := tx.First(&turn, *interaction.TurnID).Error; err != nil {
					return err
				}
				turnPublicID = turn.PublicID
			}
			return nil
		}
		var target model.AgentInteraction
		if err := tx.Select("id", "thread_id").Where("user_id = ? AND public_id = ?", userID, interactionPublicID).First(&target).Error; err != nil {
			return err
		}
		var targetThread model.AgentThread
		if err := tx.Select("id", "device_id").First(&targetThread, target.ThreadID).Error; err != nil {
			return err
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", targetThread.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&thread, target.ThreadID).Error; err != nil || thread.DeviceID != device.ID || thread.SourceThreadRef == nil {
			if err != nil {
				return err
			}
			return repository.ErrConflict
		}
		if err := tx.Select("id", "thread_id", "turn_id").First(&target, target.ID).Error; err != nil || target.ThreadID != thread.ID {
			if err != nil {
				return err
			}
			return repository.ErrConflict
		}
		var turn model.AgentTurn
		if target.TurnID != nil {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND thread_id = ?", *target.TurnID, thread.ID).First(&turn).Error; err != nil || turn.SourceTurnRef == nil {
				if err != nil {
					return err
				}
				return repository.ErrConflict
			}
			if turn.Status != "running" {
				return repository.ErrConflict
			}
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND public_id = ?", target.ID, userID, interactionPublicID).First(&interaction).Error; err != nil {
			return err
		}
		if interaction.ThreadID != thread.ID || (interaction.TurnID == nil) != (target.TurnID == nil) ||
			(interaction.TurnID != nil && *interaction.TurnID != *target.TurnID) || interaction.Status != "pending" {
			return repository.ErrConflict
		}
		if !interactionResponseMatchesRequest(interaction.Kind, interaction.RequestJSON, response) {
			return repository.ErrInvalidInput
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND status = ? AND lease_expires_at > ?", interaction.RuntimeProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.First(&workspace, thread.WorkspaceID).Error; err != nil {
			return err
		}
		payload := map[string]any{
			"kind": "interaction.respond", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID,
			"sourceThreadRef": *thread.SourceThreadRef, "interactionId": interaction.PublicID,
			"sourceRequestRef": interaction.SourceRequestRef, "scope": "thread", "response": response,
		}
		if interaction.TurnID != nil {
			turnPublicID = turn.PublicID
			payload["scope"], payload["turnId"], payload["sourceTurnRef"] = "turn", turn.PublicID, *turn.SourceTurnRef
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		commandRow := model.AgentCommand{PublicID: command.PublicID, UserID: userID, DeviceID: device.ID, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, TurnID: interaction.TurnID, InteractionID: &interaction.ID, ServerSeq: device.NextServerSeq, Kind: command.Kind, PayloadJSON: string(encoded), State: "queued", TerminalJSON: "{}"}
		if err := tx.Create(&commandRow).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		if err := tx.Model(&interaction).Update("status", "responding").Error; err != nil {
			return err
		}
		interaction.Status = "responding"
		return tx.Model(&operation).Update("result_public_id", interaction.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	result := toDomainInteraction(interaction)
	result.ThreadPublicID, result.TurnPublicID = thread.PublicID, turnPublicID
	if interaction.TurnID != nil {
		var turn model.AgentTurn
		if err := r.db.WithContext(ctx).Select("run_id").First(&turn, *interaction.TurnID).Error; err != nil {
			return nil, errFor(err)
		}
		result.RunID = turn.RunID
	}
	return result, nil
}
