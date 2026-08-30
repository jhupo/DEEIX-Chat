package db

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/gorm"
)

// migrateLegacyRelayPrincipals moves identities from the former account-origin
// namespace to the immutable connector namespace. The oldest identity remains
// canonical so enrolled Agents keep their existing public user ID.
func migrateLegacyRelayPrincipals(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var connectors []model.RelayConnector
		if err := tx.Find(&connectors).Error; err != nil {
			return err
		}
		for _, connector := range connectors {
			connectorID := strings.TrimSpace(connector.PublicID)
			legacyInstanceID := legacyRelayInstanceID(connector.AccountBaseURL)
			if connectorID == "" || legacyInstanceID == "" || connectorID == legacyInstanceID {
				continue
			}

			var legacyUsers []model.User
			if err := tx.Where(
				"auth_provider = ? AND sub2_instance_id = ? AND sub2_user_id > 0",
				domainuser.AuthProviderRelay,
				legacyInstanceID,
			).Order("id ASC").Find(&legacyUsers).Error; err != nil {
				return err
			}
			for i := range legacyUsers {
				if err := migrateLegacyRelayPrincipal(tx, &legacyUsers[i], connectorID); err != nil {
					return fmt.Errorf("migrate relay principal %d to connector %s: %w", legacyUsers[i].ID, connectorID, err)
				}
			}
		}
		return nil
	})
}

func legacyRelayInstanceID(accountBaseURL string) string {
	value := strings.TrimSpace(accountBaseURL)
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func migrateLegacyRelayPrincipal(tx *gorm.DB, canonical *model.User, connectorID string) error {
	var duplicate model.User
	err := tx.Where(
		"sub2_instance_id = ? AND sub2_user_id = ?",
		connectorID,
		canonical.Sub2UserID,
	).First(&duplicate).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == nil && duplicate.ID != canonical.ID {
		if err = mergeRelayPrincipalData(tx, canonical, &duplicate); err != nil {
			return err
		}
	}

	updates := map[string]any{
		"auth_provider":      domainuser.AuthProviderRelay,
		"relay_connector_id": connectorID,
		"sub2_instance_id":   connectorID,
	}
	if err == nil {
		updates["email"] = duplicate.Email
		updates["role"] = duplicate.Role
		updates["status"] = duplicate.Status
		if strings.TrimSpace(duplicate.DisplayName) != "" {
			updates["display_name"] = duplicate.DisplayName
		}
		if strings.TrimSpace(duplicate.AvatarURL) != "" {
			updates["avatar_url"] = duplicate.AvatarURL
		}
		updates["last_login_at"] = laterTime(canonical.LastLoginAt, duplicate.LastLoginAt)
	}
	return tx.Model(&model.User{}).Where("id = ?", canonical.ID).Updates(updates).Error
}

func laterTime(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right != nil && right.After(*left) {
		return right
	}
	return left
}

func mergeRelayPrincipalData(tx *gorm.DB, canonical, duplicate *model.User) error {
	if err := mergeSub2Bindings(tx, canonical.ID, duplicate.ID); err != nil {
		return err
	}

	for _, spec := range []ownedRowsMerge{
		{table: "user_settings", owner: "user_id", keys: []string{"key"}, sourceWins: true},
		{table: "user_memories", owner: "user_id", keys: []string{"memory_key"}},
		{table: "announcement_user_states", owner: "user_id", keys: []string{"announcement_id", "announcement_updated_at"}, sourceWins: true},
		{table: "chat_feedback", owner: "user_id", keys: []string{"message_id"}, sourceWins: true},
		{table: "permission_group_user_access", owner: "user_id", keys: []string{"group_id"}},
		{table: "agent_idempotency_records", owner: "user_id", keys: []string{"operation", "key"}},
		{table: "prompt_presets", owner: "owner_user_id", keys: []string{"scope", "trigger"}},
		{table: "skills", owner: "owner_user_id", keys: []string{"scope", "trigger"}},
		{table: "sub2_key_binding_operations", owner: "principal_id", keys: []string{"idempotency_key"}},
		{table: "sub2_payment_operations", owner: "principal_id", keys: []string{"idempotency_key"}},
	} {
		if err := mergeOwnedRows(tx, spec, canonical.ID, duplicate.ID); err != nil {
			return err
		}
	}

	if err := mergeStorageQuota(tx, canonical.ID, duplicate.ID); err != nil {
		return err
	}
	if err := revokeMergedSessions(tx, canonical.ID, duplicate.ID); err != nil {
		return err
	}
	if err := tx.Exec(`
		UPDATE agent_device_enrollment_challenges
		SET user_id = ?, user_public_id = ?
		WHERE user_id = ?
	`, canonical.ID, canonical.PublicID, duplicate.ID).Error; err != nil {
		return err
	}

	for _, table := range relayPrincipalUserTables {
		if err := updateOwnerColumn(tx, table, "user_id", canonical.ID, duplicate.ID); err != nil {
			return err
		}
	}
	for _, ref := range relayPrincipalReferenceColumns {
		if err := updateOwnerColumnIfPresent(tx, ref.table, ref.column, canonical.ID, duplicate.ID); err != nil {
			return err
		}
	}

	return tx.Unscoped().Delete(&model.User{}, duplicate.ID).Error
}

var relayPrincipalUserTables = []string{
	"agent_artifacts",
	"agent_commands",
	"agent_credentials",
	"agent_devices",
	"agent_events",
	"agent_interactions",
	"agent_items",
	"agent_resource_snapshots",
	"agent_runtime_profiles",
	"agent_runtime_proof_challenges",
	"agent_threads",
	"agent_turns",
	"agent_workspaces",
	"chat_attachments",
	"chat_context_records",
	"chat_conversation_projects",
	"chat_conversation_shares",
	"chat_conversations",
	"chat_message_chunks",
	"chat_messages",
	"chat_run_events",
	"chat_runs",
	"content_moderation_events",
	"conversation_execution_events",
	"file_chunks",
	"file_objects",
	"identity_auth_events",
}

var relayPrincipalReferenceColumns = []struct{ table, column string }{
	{table: "audit_logs", column: "actor_user_id"},
	{table: "prompt_presets", column: "created_by_user_id"},
	{table: "prompt_presets", column: "updated_by_user_id"},
	{table: "skills", column: "created_by_user_id"},
	{table: "skills", column: "updated_by_user_id"},
	{table: "system_announcements", column: "created_by_user_id"},
	{table: "knowledge_bases", column: "owner_user_id"},
	{table: "knowledge_bases", column: "created_by_user_id"},
	{table: "knowledge_bases", column: "updated_by_user_id"},
	{table: "knowledge_base_files", column: "added_by_user_id"},
	{table: "llm_model_icon_assets", column: "created_by_user_id"},
}

type ownedRowsMerge struct {
	table      string
	owner      string
	keys       []string
	sourceWins bool
}

func mergeOwnedRows(tx *gorm.DB, spec ownedRowsMerge, canonicalID, duplicateID uint) error {
	table := quoteIdentifier(spec.table)
	owner := quoteIdentifier(spec.owner)
	conditions := make([]string, 0, len(spec.keys))
	for _, key := range spec.keys {
		quoted := quoteIdentifier(key)
		conditions = append(conditions, "target."+quoted+" = source."+quoted)
	}
	conflict := strings.Join(conditions, " AND ")
	if spec.sourceWins {
		if err := tx.Exec(
			fmt.Sprintf("DELETE FROM %s AS target USING %s AS source WHERE target.%s = ? AND source.%s = ? AND %s", table, table, owner, owner, conflict),
			canonicalID,
			duplicateID,
		).Error; err != nil {
			return err
		}
	} else {
		if err := tx.Exec(
			fmt.Sprintf("DELETE FROM %s AS source USING %s AS target WHERE source.%s = ? AND target.%s = ? AND %s", table, table, owner, owner, conflict),
			duplicateID,
			canonicalID,
		).Error; err != nil {
			return err
		}
	}
	return updateOwnerColumn(tx, spec.table, spec.owner, canonicalID, duplicateID)
}

func mergeSub2Bindings(tx *gorm.DB, canonicalID, duplicateID uint) error {
	statements := []struct {
		query string
		args  []any
	}{
		{query: `UPDATE user_settings AS settings
		 SET value = target.public_id
		 FROM sub2_key_bindings AS source
		 JOIN sub2_key_bindings AS target ON target.principal_id = ? AND target.remote_key_id = source.remote_key_id
		 WHERE source.principal_id = ? AND settings.user_id = ? AND settings.value = source.public_id`, args: []any{canonicalID, duplicateID, duplicateID}},
		{query: `UPDATE chat_runs AS runs
		 SET key_binding_public_id = target.public_id
		 FROM sub2_key_bindings AS source
		 JOIN sub2_key_bindings AS target ON target.principal_id = ? AND target.remote_key_id = source.remote_key_id
		 WHERE source.principal_id = ? AND runs.key_binding_public_id = source.public_id`, args: []any{canonicalID, duplicateID}},
		{query: `UPDATE sub2_key_binding_operations AS operations
		 SET binding_public_id = target.public_id
		 FROM sub2_key_bindings AS source
		 JOIN sub2_key_bindings AS target ON target.principal_id = ? AND target.remote_key_id = source.remote_key_id
		 WHERE source.principal_id = ? AND operations.binding_public_id = source.public_id`, args: []any{canonicalID, duplicateID}},
	}
	for _, statement := range statements {
		if err := tx.Exec(statement.query, statement.args...).Error; err != nil {
			return err
		}
	}
	if err := tx.Exec(`
		DELETE FROM sub2_key_bindings AS source
		USING sub2_key_bindings AS target
		WHERE source.principal_id = ? AND target.principal_id = ?
			AND source.remote_key_id = target.remote_key_id
	`, duplicateID, canonicalID).Error; err != nil {
		return err
	}
	return updateOwnerColumn(tx, "sub2_key_bindings", "principal_id", canonicalID, duplicateID)
}

func mergeStorageQuota(tx *gorm.DB, canonicalID, duplicateID uint) error {
	if err := tx.Exec(`
		UPDATE file_storage_quotas AS target
		SET quota_bytes = CASE
				WHEN target.quota_bytes = 0 OR source.quota_bytes = 0 THEN 0
				ELSE GREATEST(target.quota_bytes, source.quota_bytes)
			END,
			used_bytes = target.used_bytes + source.used_bytes,
			reserved_bytes = target.reserved_bytes + source.reserved_bytes
		FROM file_storage_quotas AS source
		WHERE target.user_id = ? AND source.user_id = ?
	`, canonicalID, duplicateID).Error; err != nil {
		return err
	}
	if err := tx.Exec(`
		DELETE FROM file_storage_quotas AS source
		USING file_storage_quotas AS target
		WHERE source.user_id = ? AND target.user_id = ?
	`, duplicateID, canonicalID).Error; err != nil {
		return err
	}
	return updateOwnerColumn(tx, "file_storage_quotas", "user_id", canonicalID, duplicateID)
}

func revokeMergedSessions(tx *gorm.DB, canonicalID, duplicateID uint) error {
	return tx.Exec(`
		UPDATE identity_sessions
		SET user_id = ?,
			revoked_at = COALESCE(revoked_at, NOW()),
			revoke_reason = CASE WHEN revoked_at IS NULL THEN 'relay_identity_merged' ELSE revoke_reason END,
			sub2_access_token_encrypted = '',
			sub2_refresh_token_encrypted = '',
			sub2_access_expires_at = NULL,
			sub2_verified_at = NULL
		WHERE user_id = ?
	`, canonicalID, duplicateID).Error
}

func updateOwnerColumn(tx *gorm.DB, table, column string, canonicalID, duplicateID uint) error {
	return tx.Exec(
		fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", quoteIdentifier(table), quoteIdentifier(column), quoteIdentifier(column)),
		canonicalID,
		duplicateID,
	).Error
}

func updateOwnerColumnIfPresent(tx *gorm.DB, table, column string, canonicalID, duplicateID uint) error {
	if !tx.Migrator().HasTable(table) || !tx.Migrator().HasColumn(table, column) {
		return nil
	}
	return updateOwnerColumn(tx, table, column, canonicalID, duplicateID)
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
