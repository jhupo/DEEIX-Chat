package db

import (
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
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
