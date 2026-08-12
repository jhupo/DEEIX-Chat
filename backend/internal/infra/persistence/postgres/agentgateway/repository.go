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
		EnrollmentCredentialID: v.EnrollmentCredentialID,
		Name:                   v.Name, Platform: v.Platform, PublicKey: append([]byte(nil), v.PublicKey...),
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
		PublicID:          v.PublicID, UserID: v.UserID, EnrollmentCredentialID: v.EnrollmentCredentialID,
		Name: v.Name, Platform: v.Platform, PublicKey: append([]byte(nil), v.PublicKey...),
		PublicKeyFingerprint: v.PublicKeyFingerprint, CredentialVersion: v.CredentialVersion,
		Status: v.Status, NextServerSeq: v.NextServerSeq, LastSeenAt: v.LastSeenAt, RevokedAt: v.RevokedAt,
		LastAckedServerSeq: v.LastAckedServerSeq, LastAckedBridgeSeq: v.LastAckedBridgeSeq,
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

func toDomainThread(v model.AgentThread) *domainagent.Thread {
	return &domainagent.Thread{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, DeviceID: v.DeviceID, RuntimeProfileID: v.RuntimeProfileID, WorkspaceID: v.WorkspaceID, SourceThreadRef: v.SourceThreadRef, Title: v.Title, Status: v.Status, LastEventSeq: v.LastEventSeq, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func toDomainTurn(v model.AgentTurn) *domainagent.Turn {
	return &domainagent.Turn{ID: v.ID, PublicID: v.PublicID, UserID: v.UserID, ThreadID: v.ThreadID, SourceTurnRef: v.SourceTurnRef, Status: v.Status, InputJSON: v.InputJSON, SettingsJSON: v.SettingsJSON, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
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
		CredentialHash: v.CredentialHash, VerifiedAt: v.VerifiedAt,
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

func (r *Repo) CreateCredential(ctx context.Context, item *domainagent.Credential) error {
	entity := toModelCredential(item)
	if err := errFor(r.db.WithContext(ctx).Create(entity).Error); err != nil {
		return err
	}
	item.ID, item.CreatedAt, item.UpdatedAt = entity.ID, entity.CreatedAt, entity.UpdatedAt
	return nil
}

func (r *Repo) EnrollDevice(ctx context.Context, tokenHash string, input *domainagent.Device, now time.Time) (*domainagent.Device, error) {
	var result model.AgentDevice
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var credential model.AgentCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("kind = ? AND token_hash = ?", domainagent.CredentialKindEnrollment, tokenHash).
			First(&credential).Error; err != nil {
			return err
		}
		if credential.ExpiresAt.Before(now) {
			return repository.ErrConflict
		}
		if credential.ConsumedAt != nil {
			if err := tx.Where("enrollment_credential_id = ?", credential.ID).First(&result).Error; err != nil {
				return err
			}
			if result.PublicKeyFingerprint != input.PublicKeyFingerprint {
				return repository.ErrConflict
			}
			return nil
		}
		result = *toModelDevice(input)
		result.UserID = credential.UserID
		result.EnrollmentCredentialID = credential.ID
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return tx.Model(&credential).Where("consumed_at IS NULL").Update("consumed_at", now).Error
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

func (r *Repo) EnqueueCommand(ctx context.Context, userID uint, devicePublicID string, input *domainagent.Command) (*domainagent.Command, error) {
	var result model.AgentCommand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND public_id = ?", userID, devicePublicID).First(&device).Error; err != nil {
			return err
		}
		if device.Status != domainagent.DeviceStatusActive {
			return repository.ErrConflict
		}
		result = model.AgentCommand{
			PublicID: input.PublicID, UserID: userID, DeviceID: device.ID,
			ServerSeq: device.NextServerSeq, Kind: input.Kind,
			PayloadJSON: input.PayloadJSON, State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return toDomainCommand(result), nil
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
				existing.PayloadHash != payloadHash || existing.PayloadJSON != payloadJSON {
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
		return tx.Model(&stored).Updates(map[string]any{"data_json": snapshot.DataJSON, "refreshed_at": now, "updated_at": now}).Error
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
		var completedCount int64
		if err := tx.Model(&model.AgentEvent{}).
			Where("thread_id = ? AND source_turn_ref = ? AND kind = ?", *command.ThreadID, outcome.Result.SourceTurnRef, "turn/completed").
			Count(&completedCount).Error; err != nil {
			return err
		}
		if completedCount > 0 {
			if err := tx.Model(&model.AgentTurn{}).Where("id = ?", *command.TurnID).Update("status", "completed").Error; err != nil {
				return err
			}
		}
	}
	return nil
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

func (r *Repo) ApplyEventFrame(ctx context.Context, deviceID, runtimeProfileID uint, bridgeSeq uint64, payloadHash string, event *domainagent.Event, now time.Time) (uint64, error) {
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
			if existing.Kind != "event" || existing.PayloadHash != payloadHash {
				return repository.ErrConflict
			}
			return nil
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
		acknowledged = bridgeSeq
		return nil
	})
	return acknowledged, errFor(err)
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
				if err := tx.Model(&turn).Update("status", "running").Error; err != nil {
					return err
				}
			}
			if event.Kind == "turn/completed" {
				if err := tx.Model(&turn).Update("status", "completed").Error; err != nil {
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
	if event.Kind == "interaction.requested" && event.SourceRequestRef != "" {
		interaction := model.AgentInteraction{
			PublicID: newRepoPublicID("agint"), UserID: event.UserID, ThreadID: thread.ID,
			TurnID: event.TurnID, RuntimeProfileID: *event.RuntimeProfileID,
			SourceRequestRef: event.SourceRequestRef, Kind: event.Kind,
			RequestJSON: event.PayloadJSON, Status: "pending",
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
	credentialHash string,
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
				"credential_hash": credentialHash, "verified_at": now,
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
		return tx.Model(&model.AgentWorkspace{}).
			Where("user_id = ? AND device_id = ? AND runtime_profile_id = ? AND public_id NOT IN ?", userID, deviceID, profileID, publicIDs).
			Update("status", "unavailable").Error
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
		Where("workspaces.user_id = ? AND devices.public_id = ?", userID, devicePublicID).
		Order("workspaces.name ASC").Scan(&rows).Error
	result := make([]domainagent.Workspace, 0, len(rows))
	for _, row := range rows {
		item := toDomainWorkspace(row.AgentWorkspace)
		item.DevicePublicID, item.ProfilePublicID = devicePublicID, row.ProfilePublicID
		result = append(result, *item)
	}
	return result, errFor(err)
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

func (r *Repo) QueueThreadCommand(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	threadPublicID, turnPublicID string,
	parameters json.RawMessage,
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

		var thread model.AgentThread
		var turn *model.AgentTurn
		if threadPublicID != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ?", userID, threadPublicID).First(&thread).Error; err != nil {
				return err
			}
		} else {
			var existingTurn model.AgentTurn
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ?", userID, turnPublicID).First(&existingTurn).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&thread, existingTurn.ThreadID).Error; err != nil {
				return err
			}
			turn = &existingTurn
		}
		if thread.SourceThreadRef == nil {
			return repository.ErrConflict
		}
		var typed map[string]any
		if json.Unmarshal(parameters, &typed) != nil {
			return repository.ErrInvalidInput
		}
		if turn != nil && (thread.Status != "active" || turn.SourceTurnRef == nil || turn.Status != "running") {
			return repository.ErrConflict
		}
		if turn == nil {
			switch command.Kind {
			case "review.start", "thread.compact":
				if thread.Status != "active" {
					return repository.ErrConflict
				}
				var activeTurns int64
				if err := tx.Model(&model.AgentTurn{}).Where("thread_id = ? AND status IN ?", thread.ID, []string{"awaiting_thread", "queued", "running"}).Count(&activeTurns).Error; err != nil {
					return err
				}
				if activeTurns > 0 {
					return repository.ErrConflict
				}
			case "thread.rename":
				if thread.Status == "failed" || thread.Status == "deleted" {
					return repository.ErrConflict
				}
			case "thread.lifecycle":
				action, _ := typed["action"].(string)
				validState := (action == "archive" && thread.Status == "active") ||
					(action == "unarchive" && thread.Status == "archived") ||
					(action == "resume" && thread.Status == "active") ||
					(action == "delete" && (thread.Status == "active" || thread.Status == "archived"))
				if !validState {
					return repository.ErrConflict
				}
			}
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
			"kind": command.Kind, "deviceId": device.PublicID, "profileId": profile.PublicID,
			"workspaceId": workspace.PublicID, "threadId": thread.PublicID, "sourceThreadRef": *thread.SourceThreadRef,
		}
		for key, value := range typed {
			payload[key] = value
		}
		var turnID *uint
		if turn != nil {
			turnID = &turn.ID
			payload["turnId"], payload["sourceTurnRef"] = turn.PublicID, *turn.SourceTurnRef
		} else if command.Kind == "review.start" {
			reviewTurn := model.AgentTurn{
				PublicID: newRepoPublicID("agturn"), UserID: userID, ThreadID: thread.ID,
				Status: "queued", InputJSON: "[]", SettingsJSON: "{}",
			}
			if err := tx.Create(&reviewTurn).Error; err != nil {
				return err
			}
			turnID = &reviewTurn.ID
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		created = model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &thread.ID, TurnID: turnID,
			ServerSeq: device.NextServerSeq, Kind: command.Kind, PayloadJSON: string(encoded), State: "queued", TerminalJSON: "{}",
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

func (r *Repo) ForkThread(
	ctx context.Context,
	idempotencyKey, requestHash string,
	userID uint,
	parentThreadPublicID string,
	input *domainagent.Thread,
	command *domainagent.Command,
	now time.Time,
) (*domainagent.Thread, error) {
	var child model.AgentThread
	var devicePublicID, profilePublicID, workspacePublicID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		const operationName = "thread.fork"
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
			return tx.Where("user_id = ? AND public_id = ?", userID, operation.ResultPublicID).First(&child).Error
		}
		var parent model.AgentThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND public_id = ?", userID, parentThreadPublicID).First(&parent).Error; err != nil {
			return err
		}
		if parent.SourceThreadRef == nil || parent.Status == "failed" || parent.Status == "deleted" {
			return repository.ErrConflict
		}
		var profile model.AgentRuntimeProfile
		if err := tx.Where("id = ? AND status = ? AND lease_expires_at > ?", parent.RuntimeProfileID, domainagent.RuntimeStatusReady, now).First(&profile).Error; err != nil {
			return err
		}
		var workspace model.AgentWorkspace
		if err := tx.Where("id = ? AND status = ?", parent.WorkspaceID, "available").First(&workspace).Error; err != nil {
			return err
		}
		var device model.AgentDevice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", parent.DeviceID, domainagent.DeviceStatusActive).First(&device).Error; err != nil {
			return err
		}
		child = model.AgentThread{
			PublicID: input.PublicID, UserID: userID, DeviceID: device.ID, RuntimeProfileID: profile.ID,
			WorkspaceID: workspace.ID, Title: parent.Title, Status: "queued",
		}
		if err := tx.Create(&child).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "thread.lifecycle", "action": "fork", "deviceId": device.PublicID,
			"profileId": profile.PublicID, "workspaceId": workspace.PublicID,
			"threadId": parent.PublicID, "sourceThreadRef": *parent.SourceThreadRef,
		})
		if err != nil {
			return err
		}
		created := model.AgentCommand{
			PublicID: command.PublicID, UserID: userID, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &child.ID,
			ServerSeq: device.NextServerSeq, Kind: "thread.lifecycle", PayloadJSON: string(payload), State: "queued", TerminalJSON: "{}",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Model(&device).Update("next_server_seq", gorm.Expr("next_server_seq + 1")).Error; err != nil {
			return err
		}
		devicePublicID, profilePublicID, workspacePublicID = device.PublicID, profile.PublicID, workspace.PublicID
		return tx.Model(&operation).Update("result_public_id", child.PublicID).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	if devicePublicID == "" {
		item, err := r.GetThread(ctx, userID, child.PublicID)
		if err != nil {
			return nil, err
		}
		return item, nil
	}
	result := toDomainThread(child)
	result.DevicePublicID, result.ProfilePublicID, result.WorkspacePublicID = devicePublicID, profilePublicID, workspacePublicID
	return result, nil
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
		thread = model.AgentThread{PublicID: input.PublicID, UserID: input.UserID, DeviceID: device.ID, RuntimeProfileID: profile.ID, WorkspaceID: workspace.ID, Title: input.Title, Status: input.Status}
		if err := tx.Create(&thread).Error; err != nil {
			return err
		}
		if initialTurn != nil {
			createdTurn := model.AgentTurn{PublicID: initialTurn.PublicID, UserID: input.UserID, ThreadID: thread.ID, Status: initialTurn.Status, InputJSON: initialTurn.InputJSON, SettingsJSON: initialTurn.SettingsJSON}
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

func (r *Repo) ListThreads(ctx context.Context, userID uint, limit int) ([]domainagent.Thread, error) {
	type row struct {
		model.AgentThread
		DevicePublicID, ProfilePublicID, WorkspacePublicID string
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("agent_threads AS threads").
		Select("threads.*, devices.public_id AS device_public_id, profiles.public_id AS profile_public_id, workspaces.public_id AS workspace_public_id").
		Joins("JOIN agent_devices AS devices ON devices.id = threads.device_id").
		Joins("JOIN agent_runtime_profiles AS profiles ON profiles.id = threads.runtime_profile_id").
		Joins("JOIN agent_workspaces AS workspaces ON workspaces.id = threads.workspace_id").
		Where("threads.user_id = ?", userID).Order("threads.updated_at DESC").Limit(limit).Scan(&rows).Error
	result := make([]domainagent.Thread, 0, len(rows))
	for _, row := range rows {
		item := toDomainThread(row.AgentThread)
		item.DevicePublicID, item.ProfilePublicID, item.WorkspacePublicID = row.DevicePublicID, row.ProfilePublicID, row.WorkspacePublicID
		result = append(result, *item)
	}
	return result, errFor(err)
}

func (r *Repo) GetThread(ctx context.Context, userID uint, publicID string) (*domainagent.Thread, error) {
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
		Where("threads.user_id = ? AND threads.public_id = ?", userID, publicID).First(&value).Error
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
		turn = model.AgentTurn{PublicID: input.PublicID, UserID: input.UserID, ThreadID: thread.ID, Status: input.Status, InputJSON: input.InputJSON, SettingsJSON: input.SettingsJSON}
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

func (r *Repo) ListTurns(ctx context.Context, userID uint, threadPublicID string, limit int) ([]domainagent.Turn, error) {
	var rows []model.AgentTurn
	err := r.db.WithContext(ctx).Table("agent_turns AS turns").
		Joins("JOIN agent_threads AS threads ON threads.id = turns.thread_id").
		Select("turns.*").Where("turns.user_id = ? AND threads.public_id = ?", userID, threadPublicID).
		Order("turns.id ASC").Limit(limit).Find(&rows).Error
	result := make([]domainagent.Turn, 0, len(rows))
	for _, row := range rows {
		item := toDomainTurn(row)
		item.ThreadPublicID = threadPublicID
		result = append(result, *item)
	}
	return result, errFor(err)
}

func (r *Repo) ListEvents(ctx context.Context, userID uint, threadPublicID string, after uint64, limit int) ([]domainagent.Event, error) {
	type row struct {
		model.AgentEvent
		TurnPublicID string `gorm:"column:turn_public_id"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Table("agent_events AS events").
		Select("events.*, COALESCE(turns.public_id, '') AS turn_public_id").
		Joins("JOIN agent_threads AS threads ON threads.id = events.thread_id").
		Joins("LEFT JOIN agent_turns AS turns ON turns.id = events.turn_id").
		Where("events.user_id = ? AND threads.public_id = ? AND events.thread_seq > ?", userID, threadPublicID, after).
		Order("events.thread_seq ASC").Limit(limit).Find(&rows).Error
	result := make([]domainagent.Event, 0, len(rows))
	for _, row := range rows {
		item := toDomainEvent(row.AgentEvent)
		item.TurnPublicID = row.TurnPublicID
		result = append(result, *item)
	}
	return result, errFor(err)
}

func (r *Repo) ListInteractions(ctx context.Context, userID uint, threadPublicID, status string, limit int) ([]domainagent.Interaction, error) {
	type row struct {
		model.AgentInteraction
		TurnPublicID string `gorm:"column:turn_public_id"`
	}
	var rows []row
	query := r.db.WithContext(ctx).Table("agent_interactions AS interactions").
		Select("interactions.*, COALESCE(turns.public_id, '') AS turn_public_id").
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
		item.TurnPublicID = row.TurnPublicID
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
	return result, nil
}
