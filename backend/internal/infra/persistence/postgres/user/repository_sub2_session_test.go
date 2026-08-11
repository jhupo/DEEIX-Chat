package user

import (
	"context"
	"errors"
	"testing"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestStageSessionSub2TokensPreservesVerificationAndRejectsRevokedSession(t *testing.T) {
	db := testutil.UnmigratedPostgres(t)
	if err := db.AutoMigrate(&model.UserSession{}); err != nil {
		t.Fatalf("migrate identity sessions: %v", err)
	}
	verifiedAt := time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
	session := model.UserSession{
		UserID:                    1,
		SessionID:                 "active-session",
		Sub2AccessTokenEncrypted:  "old-access",
		Sub2RefreshTokenEncrypted: "old-refresh",
		Sub2AccessExpiresAt:       &verifiedAt,
		Sub2VerifiedAt:            &verifiedAt,
		IssuedAt:                  verifiedAt,
		ExpiresAt:                 verifiedAt.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create active session: %v", err)
	}
	repo := NewRepo(db)
	expiresAt := verifiedAt.Add(2 * time.Hour)
	if err := repo.StageSessionSub2Tokens(context.Background(), repository.UpdateSessionSub2TokensInput{
		UserID: 1, SessionID: "active-session", AccessTokenEncrypted: "new-access", RefreshTokenEncrypted: "new-refresh", AccessExpiresAt: expiresAt,
	}); err != nil {
		t.Fatalf("stage active session: %v", err)
	}
	if err := db.First(&session, session.ID).Error; err != nil {
		t.Fatalf("reload active session: %v", err)
	}
	if session.Sub2AccessTokenEncrypted != "new-access" || session.Sub2RefreshTokenEncrypted != "new-refresh" || !session.Sub2AccessExpiresAt.Equal(expiresAt) || session.Sub2VerifiedAt == nil || !session.Sub2VerifiedAt.Equal(verifiedAt) {
		t.Fatalf("staged session = %#v", session)
	}

	revokedAt := verifiedAt.Add(time.Minute)
	revoked := model.UserSession{UserID: 1, SessionID: "revoked-session", Sub2AccessTokenEncrypted: "old-access", Sub2RefreshTokenEncrypted: "old-refresh", Sub2AccessExpiresAt: &verifiedAt, Sub2VerifiedAt: &verifiedAt, IssuedAt: verifiedAt, ExpiresAt: verifiedAt.Add(time.Hour), RevokedAt: &revokedAt}
	if err := db.Create(&revoked).Error; err != nil {
		t.Fatalf("create revoked session: %v", err)
	}
	err := repo.StageSessionSub2Tokens(context.Background(), repository.UpdateSessionSub2TokensInput{UserID: 1, SessionID: "revoked-session", AccessTokenEncrypted: "new-access", RefreshTokenEncrypted: "new-refresh", AccessExpiresAt: expiresAt})
	if !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("stage revoked session error = %v, want %v", err, repository.ErrInvalidInput)
	}
	if err := db.First(&revoked, revoked.ID).Error; err != nil {
		t.Fatalf("reload revoked session: %v", err)
	}
	if revoked.Sub2AccessTokenEncrypted != "old-access" || revoked.Sub2RefreshTokenEncrypted != "old-refresh" {
		t.Fatalf("staging restored credentials on revoked session: %#v", revoked)
	}
}
