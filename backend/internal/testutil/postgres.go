// Package testutil provides PostgreSQL integration-test setup shared by persistence packages.
package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Postgres creates an isolated PostgreSQL schema. Callers migrate only the models
// needed by their behavior test, keeping test setup explicit and focused.
func Postgres(t *testing.T) *gorm.DB {
	return openPostgres(t)
}

// UnmigratedPostgres creates an isolated schema for migration tests that need to
// establish a pre-migration state themselves.
func UnmigratedPostgres(t *testing.T) *gorm.DB {
	return openPostgres(t)
}

func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("DEEIX_TEST_DATABASE_DSN"))
	if dsn == "" {
		if strings.EqualFold(os.Getenv("CI"), "true") {
			t.Fatal("DEEIX_TEST_DATABASE_DSN is required in CI")
		}
		t.Skip("DEEIX_TEST_DATABASE_DSN is not set")
	}

	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL admin connection: %v", err)
	}
	if err = admin.Exec(`CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`).Error; err != nil {
		_ = adminSQL.Close()
		t.Fatalf("enable pgvector in public schema: %v", err)
	}
	schemaName := "deeix_test_" + randomSuffix(t)
	if err = admin.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		_ = adminSQL.Close()
		t.Fatalf("create PostgreSQL test schema: %v", err)
	}

	db, err := gorm.Open(postgres.Open(dsnWithSearchPath(dsn, schemaName)), &gorm.Config{})
	if err != nil {
		_ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		_ = adminSQL.Close()
		t.Fatalf("open PostgreSQL test schema: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		_ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		_ = adminSQL.Close()
		t.Fatalf("get PostgreSQL test connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err = db.Exec(`SET search_path TO "` + schemaName + `", public`).Error; err != nil {
		_ = sqlDB.Close()
		_ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		_ = adminSQL.Close()
		t.Fatalf("set PostgreSQL test search path: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		_ = adminSQL.Close()
	})
	return db
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate PostgreSQL test schema name: %v", err)
	}
	return hex.EncodeToString(buf)
}

func dsnWithSearchPath(dsn string, schemaName string) string {
	if parsed, err := url.Parse(dsn); err == nil && parsed.Host != "" && parsed.Scheme != "" {
		query := parsed.Query()
		query.Set("search_path", schemaName+",public")
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	return dsn + " search_path='" + schemaName + ",public'"
}
