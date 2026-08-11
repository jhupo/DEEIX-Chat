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
