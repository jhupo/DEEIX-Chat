package agentgateway

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

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
	return &domainagent.Workspace{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID, RuntimeProfileID: v.RuntimeProfileID, Name: v.Name, Status: v.Status, LastSeenAt: v.LastSeenAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
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
		LeaseExpiresAt: v.LeaseExpiresAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
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
	var rows []model.AgentDevice
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, errFor(err)
	}
	result := make([]domainagent.Device, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toDomainDevice(row))
	}
	return result, nil
}

func (r *Repo) GetDevice(ctx context.Context, userID uint, publicID string) (*domainagent.Device, error) {
	var row model.AgentDevice
	if err := r.db.WithContext(ctx).Where("user_id = ? AND public_id = ?", userID, publicID).First(&row).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainDevice(row), nil
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
		return tx.Model(&model.AgentCredential{}).
			Where("device_id = ? AND consumed_at IS NULL", device.ID).Update("consumed_at", now).Error
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
		var credential model.AgentCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("kind = ? AND token_hash = ?", domainagent.CredentialKindConnection, tokenHash).
			First(&credential).Error; err != nil {
			return err
		}
		if credential.DeviceID == nil || credential.ConsumedAt != nil || !credential.ExpiresAt.After(now) {
			return repository.ErrConflict
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, *credential.DeviceID).Error; err != nil {
			return err
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

func (r *Repo) MarkCommandDelivered(ctx context.Context, deviceID, commandID uint, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&model.AgentCommand{}).
		Where("id = ? AND device_id = ?", commandID, deviceID).
		Updates(map[string]any{
			"delivered_at": gorm.Expr("COALESCE(delivered_at, ?)", now),
			"state":        gorm.Expr("CASE WHEN state = 'queued' THEN 'delivered' ELSE state END"),
		})
	if result.Error != nil {
		return errFor(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *Repo) AckServerCommands(ctx context.Context, deviceID uint, through uint64, now time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, deviceID).Error; err != nil {
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, deviceID).Error; err != nil {
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
			if err := projectTerminalResult(tx, &device, &command, payloadJSON, now); err != nil {
				return err
			}
			if err := tx.Model(&command).Updates(map[string]any{
				"state": "completed", "terminal_json": payloadJSON, "completed_at": now,
			}).Error; err != nil {
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

func projectTerminalResult(tx *gorm.DB, device *model.AgentDevice, command *model.AgentCommand, payloadJSON string, now time.Time) error {
	var outcome struct {
		Kind   string `json:"kind"`
		Result struct {
			Kind            string          `json:"kind"`
			SourceThreadRef string          `json:"sourceThreadRef"`
			SourceTurnRef   string          `json:"sourceTurnRef"`
			Resource        string          `json:"resource"`
			Data            json.RawMessage `json:"data"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(payloadJSON), &outcome) != nil {
		return repository.ErrInvalidInput
	}
	if outcome.Kind == "error" {
		updates := map[string]any{"status": "failed", "updated_at": now}
		if command.InteractionID != nil {
			return tx.Model(&model.AgentInteraction{}).Where("id = ?", *command.InteractionID).Updates(updates).Error
		}
		if command.TurnID != nil && (command.Kind == "turn.start" || command.Kind == "review.start") {
			return tx.Model(&model.AgentTurn{}).Where("id = ?", *command.TurnID).Updates(updates).Error
		}
		if command.ThreadID != nil && command.Kind == "thread.create" {
			return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).Updates(updates).Error
		}
		if command.ThreadID != nil && command.Kind == "thread.lifecycle" {
			var payload struct {
				Action string `json:"action"`
			}
			if json.Unmarshal([]byte(command.PayloadJSON), &payload) == nil && payload.Action == "fork" {
				return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).Updates(updates).Error
			}
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
			status := map[string]string{"resume": "active", "archive": "archived", "unarchive": "active", "delete": "deleted"}[payload.Action]
			if status == "" {
				return repository.ErrConflict
			}
			return tx.Model(&model.AgentThread{}).Where("id = ?", *command.ThreadID).
				Updates(map[string]any{"status": status, "updated_at": now}).Error
		}
	}
	switch outcome.Result.Kind {
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
			return syncWorkspaceSessions(tx, device, command, outcome.Result.Data, now)
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
		if err := projectPendingThreadEvents(tx, &thread); err != nil {
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
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoningContent"`
	CreatedAt        int64  `json:"createdAt"`
}

type workspaceSession struct {
	SourceThreadRef string                    `json:"sourceThreadRef"`
	Preview         string                    `json:"preview"`
	Name            string                    `json:"name"`
	ModelProvider   string                    `json:"modelProvider"`
	CreatedAt       int64                     `json:"createdAt"`
	UpdatedAt       int64                     `json:"updatedAt"`
	Messages        []workspaceSessionMessage `json:"messages"`
}

type workspaceSessionSnapshot struct {
	Data []workspaceSession `json:"data"`
}

func syncWorkspaceSessions(tx *gorm.DB, device *model.AgentDevice, command *model.AgentCommand, raw json.RawMessage, now time.Time) error {
	if command.RuntimeProfileID == nil || command.WorkspaceID == nil {
		return repository.ErrConflict
	}
	var snapshot workspaceSessionSnapshot
	if json.Unmarshal(raw, &snapshot) != nil || len(snapshot.Data) > 30 {
		return repository.ErrConflict
	}
	var profile model.AgentRuntimeProfile
	if err := tx.First(&profile, *command.RuntimeProfileID).Error; err != nil {
		return err
	}
	var workspace model.AgentWorkspace
	if err := tx.First(&workspace, *command.WorkspaceID).Error; err != nil {
		return err
	}
	for _, session := range snapshot.Data {
		if !validWorkspaceSession(session) {
			return repository.ErrConflict
		}
		var existing model.AgentThread
		err := tx.Where("runtime_profile_id = ? AND source_thread_ref = ?", profile.ID, session.SourceThreadRef).First(&existing).Error
		if err == nil {
			if err := syncExistingWorkspaceSession(tx, &existing, &workspace, session, now); err != nil {
				return err
			}
			continue
		}
		if !dberror.IsRecordNotFound(err) {
			return err
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
		updatedAt := validSessionTime(session.UpdatedAt, now)
		if updatedAt.Before(createdAt) {
			updatedAt = createdAt
		}
		conversation := model.Conversation{
			UserID: device.UserID, PublicID: newChatPublicID(), Title: title, LabelsJSON: "[]",
			Provider: profile.Provider, ExecutionType: "gateway", ExecutionDeviceID: device.PublicID,
			ExecutionProfileID: profile.PublicID, ExecutionWorkspaceID: workspace.PublicID,
			SessionKey: uuid.NewString(), MessageCount: len(session.Messages), Status: "active", ContextPolicy: "{}",
			BaseModel: model.BaseModel{CreatedAt: createdAt, UpdatedAt: updatedAt},
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		thread := model.AgentThread{
			PublicID: newRepoPublicID("agth"), UserID: device.UserID, DeviceID: device.ID,
			RuntimeProfileID: profile.ID, WorkspaceID: workspace.ID, ConversationID: conversation.ID,
			SourceThreadRef: &session.SourceThreadRef, Title: title, Status: "active",
		}
		if err := tx.Create(&thread).Error; err != nil {
			return err
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
				return err
			}
			parentID = &message.ID
		}
	}
	return nil
}

func validWorkspaceSession(session workspaceSession) bool {
	if !validRepoRef(strings.TrimSpace(session.SourceThreadRef)) || len(session.Messages) > 200 || len(session.Name) > 1024 || len(session.Preview) > 4096 {
		return false
	}
	total := 0
	for _, message := range session.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			return false
		}
		total += len(message.Content) + len(message.ReasoningContent)
		if strings.TrimSpace(message.Content) == "" || total > 64*1024 {
			return false
		}
	}
	return true
}

func syncExistingWorkspaceSession(tx *gorm.DB, thread *model.AgentThread, workspace *model.AgentWorkspace, session workspaceSession, now time.Time) error {
	var conversation model.Conversation
	if err := tx.Where("id = ? AND user_id = ? AND execution_type = ?", thread.ConversationID, thread.UserID, "gateway").First(&conversation).Error; err != nil {
		if dberror.IsRecordNotFound(err) {
			return nil
		}
		return err
	}
	if thread.WorkspaceID != workspace.ID {
		if err := tx.Model(thread).Updates(map[string]any{"workspace_id": workspace.ID, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	if conversation.ExecutionWorkspaceID != workspace.PublicID {
		if err := tx.Model(&conversation).UpdateColumn("execution_workspace_id", workspace.PublicID).Error; err != nil {
			return err
		}
	}
	var stored []model.Message
	if err := tx.Where("conversation_id = ?", conversation.ID).Order("created_at ASC, id ASC").Find(&stored).Error; err != nil {
		return err
	}
	if len(stored) > len(session.Messages) {
		return nil
	}
	for index := range stored {
		if stored[index].Role != session.Messages[index].Role || stored[index].Content != session.Messages[index].Content || stored[index].ReasoningContent != session.Messages[index].ReasoningContent {
			return nil
		}
	}
	var parentID *uint
	if len(stored) > 0 {
		parentID = &stored[len(stored)-1].ID
	}
	for _, source := range session.Messages[len(stored):] {
		createdAt := validSessionTime(source.CreatedAt, now)
		message := model.Message{
			ConversationID: conversation.ID, UserID: thread.UserID, PublicID: newChatPublicID(),
			ParentMessageID: parentID, Role: source.Role, ContentType: "text", Content: source.Content,
			ReasoningContent: source.ReasoningContent, BranchReason: "default", Status: "success",
			BaseModel: model.BaseModel{CreatedAt: createdAt, UpdatedAt: createdAt},
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		parentID = &message.ID
	}
	updates := map[string]any{"message_count": len(session.Messages), "updated_at": validSessionTime(session.UpdatedAt, now)}
	if title := strings.TrimSpace(session.Name); title != "" {
		updates["title"] = truncateRunes(title, 255)
	}
	return tx.Model(&conversation).Updates(updates).Error
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

func projectPendingThreadEvents(tx *gorm.DB, thread *model.AgentThread) error {
	var events []model.AgentEvent
	if err := tx.Where("runtime_profile_id = ? AND source_thread_ref = ? AND thread_id IS NULL", thread.RuntimeProfileID, *thread.SourceThreadRef).
		Order("bridge_frame_id ASC").Find(&events).Error; err != nil {
		return err
	}
	for i := range events {
		if err := projectAgentEvent(tx, &events[i]); err != nil {
			return err
		}
		if err := tx.Model(&events[i]).Updates(map[string]any{
			"thread_id": events[i].ThreadID, "workspace_id": events[i].WorkspaceID,
			"turn_id": events[i].TurnID, "thread_seq": events[i].ThreadSeq,
		}).Error; err != nil {
			return err
		}
	}
	return nil
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

func (r *Repo) ApplyEventFrame(ctx context.Context, deviceID, runtimeProfileID uint, bridgeSeq uint64, payloadHash string, event *domainagent.Event, now time.Time) (*domainagent.AppliedEventFrame, error) {
	var applied domainagent.AppliedEventFrame
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, deviceID).Error; err != nil {
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
		if err := projectAgentEvent(tx, &projected); err != nil {
			return err
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

func loadAppliedEventFrame(tx *gorm.DB, bridgeFrameID uint, acknowledged uint64, result *domainagent.AppliedEventFrame) error {
	var event model.AgentEvent
	if err := tx.Where("bridge_frame_id = ?", bridgeFrameID).First(&event).Error; err != nil {
		return err
	}
	result.Acknowledged = acknowledged
	result.Event = *toDomainEvent(event)
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
	result := r.db.WithContext(ctx).Model(&model.AgentEvent{}).
		Where("id = ? AND conversation_projected_at IS NULL", eventID).
		Update("conversation_projected_at", projectedAt)
	if result.Error != nil {
		return errFor(result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&model.AgentEvent{}).Where("id = ? AND conversation_projected_at IS NOT NULL", eventID).Count(&count).Error; err != nil {
			return errFor(err)
		}
		if count == 0 {
			return repository.ErrNotFound
		}
	}
	return nil
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
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil || payload.Item.Type == "" {
			return repository.ErrConflict
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
	return tx.Model(&model.AgentTurn{}).Where("id = ?", turnID).Updates(map[string]any{
		"status": status, "error_code": code, "error_message": message, "updated_at": updatedAt,
	}).Error
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

func interactionResponseMatchesKind(kind string, response json.RawMessage) bool {
	var payload struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(response, &payload) != nil {
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
	return expected != "" && payload.Kind == expected
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
			"status":           domainagent.RuntimeStatusProving,
			"remote_key_id":    nil,
			"credential_hash":  "",
			"verified_at":      nil,
			"lease_expires_at": nil,
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
				"lease_expires_at": leaseExpiresAt,
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

func (r *Repo) SyncWorkspaces(ctx context.Context, userID, deviceID, profileID uint, items []domainagent.Workspace, now time.Time) error {
	return errFor(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND user_id = ? AND device_id = ? AND status = ? AND lease_expires_at > ?", profileID, userID, deviceID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		publicIDs := make([]string, 0, len(items))
		for _, item := range items {
			workspace := model.AgentWorkspace{PublicID: item.PublicID, UserID: userID, DeviceID: deviceID, RuntimeProfileID: profileID, Name: item.Name, Status: "available", LastSeenAt: now}
			if err := tx.Where("device_id = ? AND public_id = ?", deviceID, item.PublicID).Attrs(workspace).FirstOrCreate(&workspace).Error; err != nil {
				return err
			}
			if workspace.UserID != userID || workspace.RuntimeProfileID != profileID {
				return repository.ErrConflict
			}
			if err := tx.Model(&workspace).Updates(map[string]any{"name": item.Name, "status": "available", "last_seen_at": now}).Error; err != nil {
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
		ProfilePublicID string `gorm:"column:profile_public_id"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("agent_workspaces AS workspaces").
		Select("workspaces.*, profiles.public_id AS profile_public_id").
		Joins("JOIN agent_devices AS devices ON devices.id = workspaces.device_id").
		Joins("JOIN agent_runtime_profiles AS profiles ON profiles.id = workspaces.runtime_profile_id").
		Where("workspaces.user_id = ? AND devices.public_id = ? AND workspaces.status = ?", userID, devicePublicID, "available").
		Order("workspaces.name ASC").Scan(&rows).Error
	result := make([]domainagent.Workspace, 0, len(rows))
	for _, row := range rows {
		item := toDomainWorkspace(row.AgentWorkspace)
		item.DevicePublicID, item.ProfilePublicID = devicePublicID, row.ProfilePublicID
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
		if !strings.HasPrefix(mimeType, "image/") && !strings.HasPrefix(mimeType, "audio/") {
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

		var turn model.AgentTurn
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ?", userID, turnPublicID).First(&turn).Error; err != nil {
			return err
		}
		var thread model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&thread, turn.ThreadID).Error; err != nil {
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
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", thread.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
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
			ProfileID   string `json:"profileId"`
			WorkspaceID string `json:"workspaceId"`
		}
		if err := json.Unmarshal([]byte(command.PayloadJSON), &target); err != nil {
			return repository.ErrInvalidInput
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("user_id = ? AND device_id = ? AND public_id = ? AND status = ? AND lease_expires_at > ?", input.UserID, device.ID, target.ProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.Where("user_id = ? AND device_id = ? AND runtime_profile_id = ? AND public_id = ? AND status = ?", input.UserID, device.ID, profile.ID, target.WorkspaceID, "available").First(&workspace).Error; err != nil {
			return err
		}
		if initialTurn != nil {
			if err := validateCommandArtifacts(tx, input.UserID, workspace.ID, initialTurn.InputJSON); err != nil {
				return err
			}
		}
		thread = model.AgentThread{PublicID: input.PublicID, UserID: input.UserID, DeviceID: device.ID, RuntimeProfileID: profile.ID, WorkspaceID: workspace.ID, ConversationID: input.ConversationID, Title: input.Title, Status: input.Status}
		if err := tx.Create(&thread).Error; err != nil {
			return err
		}
		if initialTurn != nil {
			createdTurn := model.AgentTurn{PublicID: initialTurn.PublicID, UserID: input.UserID, ThreadID: thread.ID, RunID: initialTurn.RunID, Status: initialTurn.Status, InputJSON: initialTurn.InputJSON, SettingsJSON: initialTurn.SettingsJSON}
			if err := tx.Create(&createdTurn).Error; err != nil {
				return err
			}
			turn = &createdTurn
		}
		commandRow := model.AgentCommand{PublicID: command.PublicID, UserID: input.UserID, DeviceID: device.ID, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, ServerSeq: device.NextServerSeq, Kind: command.Kind, PayloadJSON: command.PayloadJSON, State: "queued", TerminalJSON: "{}"}
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
		var thread model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ?", input.UserID, input.ThreadPublicID).First(&thread).Error; err != nil {
			return err
		}
		if thread.SourceThreadRef == nil || thread.Status != "active" {
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
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", thread.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.First(&workspace, thread.WorkspaceID).Error; err != nil {
			return err
		}
		if err := validateCommandArtifacts(tx, input.UserID, workspace.ID, input.InputJSON); err != nil {
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ?", userID, interactionPublicID).First(&interaction).Error; err != nil {
			return err
		}
		if interaction.Status != "pending" {
			return repository.ErrConflict
		}
		if !interactionResponseMatchesKind(interaction.Kind, response) {
			return repository.ErrInvalidInput
		}
		if err := tx.First(&thread, interaction.ThreadID).Error; err != nil || thread.SourceThreadRef == nil {
			if err != nil {
				return err
			}
			return repository.ErrConflict
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND status = ? AND lease_expires_at > ?", interaction.RuntimeProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.First(&workspace, thread.WorkspaceID).Error; err != nil {
			return err
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", thread.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		payload := map[string]any{
			"kind": "interaction.respond", "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID,
			"sourceThreadRef": *thread.SourceThreadRef, "interactionId": interaction.PublicID,
			"sourceRequestRef": interaction.SourceRequestRef, "scope": "thread", "response": response,
		}
		if interaction.TurnID != nil {
			var turn model.AgentTurn
			if err := tx.First(&turn, *interaction.TurnID).Error; err != nil || turn.SourceTurnRef == nil {
				if err != nil {
					return err
				}
				return repository.ErrConflict
			}
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
