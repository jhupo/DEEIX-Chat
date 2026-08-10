package openwebui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

func TestOpenDBAcceptsPostgresDSN(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("DEEIX_TEST_DATABASE_DSN"))
	if dsn == "" {
		if strings.EqualFold(os.Getenv("CI"), "true") {
			t.Fatal("DEEIX_TEST_DATABASE_DSN is required in CI")
		}
		t.Skip("DEEIX_TEST_DATABASE_DSN is not set")
	}

	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB(%q): %v", dsn, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("resolve SQL DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close SQL DB: %v", err)
	}
}

func TestOpenDBRejectsNonPostgresDSNBeforeOpeningConnection(t *testing.T) {
	for _, dsn := range []string{"", "sqlite://users.db", "users.db", "/tmp/users.db", "mysql://user:pass@host/users", "postgres://"} {
		_, err := openDB(dsn)
		if !errors.Is(err, repository.ErrInvalidInput) {
			t.Errorf("openDB(%q) error = %v, want ErrInvalidInput", dsn, err)
		}
	}
}
