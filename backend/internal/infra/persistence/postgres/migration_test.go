package db

import (
	"testing"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestMigrateCreatesCleanSub2Schema(t *testing.T) {
	database := testutil.UnmigratedPostgres(t)
	if err := migrate(database, config.Config{}); err != nil {
		t.Fatalf("migrate empty PostgreSQL schema: %v", err)
	}

	for _, table := range []string{
		"identity_users",
		"identity_sessions",
		"identity_auth_events",
		"sub2_key_bindings",
		"sub2_key_binding_operations",
		"sub2_payment_operations",
	} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("required table %q was not created", table)
		}
	}

	for _, table := range []string{
		"identity_credentials",
		"identity_contact_verifications",
		"identity_providers",
		"billing_plans",
		"billing_subscriptions",
	} {
		if database.Migrator().HasTable(table) {
			t.Fatalf("removed table %q was created", table)
		}
	}
}

func TestMigrateRejectsPopulatedLegacyIdentitySchemaBeforeMutation(t *testing.T) {
	database := testutil.UnmigratedPostgres(t)
	if err := database.Exec(`
		CREATE TABLE identity_users (id bigserial PRIMARY KEY, username text NOT NULL);
		INSERT INTO identity_users (username) VALUES ('legacy-user');
	`).Error; err != nil {
		t.Fatalf("create legacy fixture: %v", err)
	}

	if err := migrate(database, config.Config{}); err == nil {
		t.Fatal("expected populated legacy schema to be rejected")
	}
	if database.Migrator().HasColumn("identity_users", "sub2_instance_id") {
		t.Fatal("migration mutated legacy identity_users before rejecting it")
	}
	if database.Migrator().HasTable("sub2_key_bindings") {
		t.Fatal("migration created new tables before rejecting the legacy schema")
	}
}

func TestPrepareUnifiedConversationExecutionBackfillsCloudWithoutKeepingADefault(t *testing.T) {
	database := testutil.UnmigratedPostgres(t)
	if err := database.Exec(`
		CREATE TABLE chat_conversations (id bigserial PRIMARY KEY, title text NOT NULL);
		INSERT INTO chat_conversations (title) VALUES ('existing chat');
	`).Error; err != nil {
		t.Fatalf("create conversation fixture: %v", err)
	}

	if err := prepareUnifiedConversationExecution(database); err != nil {
		t.Fatalf("prepare unified execution: %v", err)
	}
	var executionType string
	if err := database.Raw(`SELECT execution_type FROM chat_conversations WHERE id = 1`).Scan(&executionType).Error; err != nil {
		t.Fatal(err)
	}
	if executionType != "cloud" {
		t.Fatalf("existing conversation execution = %q", executionType)
	}
	var defaultValue *string
	if err := database.Raw(`
		SELECT column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'chat_conversations' AND column_name = 'execution_type'
	`).Scan(&defaultValue).Error; err != nil {
		t.Fatal(err)
	}
	if defaultValue != nil {
		t.Fatalf("execution_type retained a database default: %q", *defaultValue)
	}
}

func TestConversationExecutionConstraintsRejectMixedTargets(t *testing.T) {
	database := testutil.UnmigratedPostgres(t)
	if err := migrate(database, config.Config{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	base := `INSERT INTO chat_conversations (user_id, public_id, title, labels_json, model, provider, execution_type, execution_device_id, execution_profile_id, execution_workspace_id, session_key, status, context_policy) VALUES `
	if err := database.Exec(base + `
		(1, 'constraint_cloud', 'cloud', '[]', '', '', 'cloud', 'agd_x', '', '', 'session_constraint_cloud', 'active', '')
	`).Error; err == nil {
		t.Fatal("cloud conversation accepted a gateway target")
	}
	if err := database.Exec(base + `
		(1, 'constraint_gateway', 'gateway', '[]', '', 'codex', 'gateway', 'agd_x', 'profile_x', '', 'session_constraint_gateway', 'active', '')
	`).Error; err == nil {
		t.Fatal("gateway conversation accepted an incomplete target")
	}
}

func TestMigrateLegacyRelayPrincipalsPreservesAgentIdentityAndMergesDuplicate(t *testing.T) {
	database := testutil.UnmigratedPostgres(t)
	if err := migrate(database, config.Config{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}

	connector := model.RelayConnector{
		PublicID: "connector-1", Name: "relay", Protocol: "sub2api",
		AccountBaseURL: "https://dash.example.test", ModelBaseURL: "https://dash.example.test",
		ConfigJSON: "{}", Enabled: true,
	}
	if err := database.Create(&connector).Error; err != nil {
		t.Fatal(err)
	}
	canonical := model.User{
		AuthProvider: domainuser.AuthProviderRelay, Sub2InstanceID: legacyRelayInstanceID(connector.AccountBaseURL),
		Sub2UserID: 7, PublicID: "0123456789abcdef0123456789abcdef", Username: "legacy-user",
		DisplayName: "Legacy", Email: "user@example.test", Role: domainuser.RoleUser,
		Status: domainuser.StatusActive, Timezone: "Etc/UTC", Locale: "en-US",
	}
	duplicate := model.User{
		AuthProvider: domainuser.AuthProviderRelay, RelayConnectorID: connector.PublicID, Sub2InstanceID: connector.PublicID,
		Sub2UserID: 7, PublicID: "abcdef0123456789abcdef0123456789", Username: "connector-user",
		DisplayName: "Current", Email: "user@example.test", Role: domainuser.RoleUser,
		Status: domainuser.StatusActive, Timezone: "Etc/UTC", Locale: "en-US",
	}
	if err := database.Create(&canonical).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: canonical.ID, Name: "desktop", Platform: "windows",
		PublicKey: []byte("public-key"), PublicKeyFingerprint: "device-fingerprint", CredentialVersion: 1,
		Status: "active", NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.UserAuthEvent{UserID: duplicate.ID, EventType: "login", Result: "success", OccurredAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.UserSetting{UserID: canonical.ID, Key: "chat.default_model", Value: "old"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.UserSetting{UserID: duplicate.ID, Key: "chat.default_model", Value: "new"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.Sub2KeyBinding{PublicID: "binding-old", PrincipalID: canonical.ID, Sub2AccountID: 7, RemoteKeyID: 1, Fingerprint: "fingerprint-old"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.Sub2KeyBinding{PublicID: "binding-new", PrincipalID: duplicate.ID, Sub2AccountID: 7, RemoteKeyID: 2, Fingerprint: "fingerprint-new"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := database.Create(&model.UserSession{
		SessionID: "duplicate-session", UserID: duplicate.ID, RefreshTokenHash: "hash", AccessJTI: "jti",
		Sub2AccessTokenEncrypted: "access", Sub2RefreshTokenEncrypted: "refresh",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyRelayPrincipals(database); err != nil {
		t.Fatalf("migrate relay principals: %v", err)
	}
	if err := migrateLegacyRelayPrincipals(database); err != nil {
		t.Fatalf("repeat relay principal migration: %v", err)
	}

	var users []model.User
	if err := database.Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != canonical.ID || users[0].PublicID != canonical.PublicID ||
		users[0].Sub2InstanceID != connector.PublicID || users[0].RelayConnectorID != connector.PublicID {
		t.Fatalf("migrated users = %#v", users)
	}
	var migratedDevice model.AgentDevice
	if err := database.First(&migratedDevice, device.ID).Error; err != nil || migratedDevice.UserID != canonical.ID {
		t.Fatalf("migrated device = %#v, %v", migratedDevice, err)
	}
	var event model.UserAuthEvent
	if err := database.First(&event).Error; err != nil || event.UserID != canonical.ID {
		t.Fatalf("migrated event = %#v, %v", event, err)
	}
	var setting model.UserSetting
	if err := database.Where("user_id = ? AND key = ?", canonical.ID, "chat.default_model").First(&setting).Error; err != nil || setting.Value != "new" {
		t.Fatalf("merged setting = %#v, %v", setting, err)
	}
	var bindingCount int64
	if err := database.Model(&model.Sub2KeyBinding{}).Where("principal_id = ?", canonical.ID).Count(&bindingCount).Error; err != nil || bindingCount != 2 {
		t.Fatalf("binding count = %d, %v", bindingCount, err)
	}
	var session model.UserSession
	if err := database.Where("session_id = ?", "duplicate-session").First(&session).Error; err != nil || session.UserID != canonical.ID ||
		session.RevokedAt == nil || session.RevokeReason != "relay_identity_merged" || session.Sub2AccessTokenEncrypted != "" {
		t.Fatalf("merged session = %#v, %v", session, err)
	}
}
