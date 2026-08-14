package auth

import (
	"context"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
)

type LoginOptions struct {
	EmailRegistrationEnabled bool
	EmailVerificationEnabled bool
	TurnstileEnabled         bool
	TurnstileSiteKey         string
}

type EmailRegistrationStartResult struct {
	Sent      bool
	ExpiresAt time.Time
}

// Sub2AccessTokenForSession resolves the current session through the same
// refresh and validation path used by authenticated identity operations.
func (s *Service) Sub2AccessTokenForSession(ctx context.Context, userID uint, sessionID string) (string, error) {
	return s.sub2AccessTokenForSession(ctx, userID, sessionID)
}

// Sub2AccessTokensForUser returns the first usable upstream session for a
// device-authenticated flow that has no browser session of its own.
func (s *Service) Sub2AccessTokensForUser(ctx context.Context, userID uint) ([]string, error) {
	sessions, err := s.repo.ListActiveSessionsByUserID(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		token, resolveErr := s.sub2AccessTokenForSession(ctx, userID, sessions[i].SessionID)
		if resolveErr == nil {
			return []string{token}, nil
		}
	}
	return nil, ErrSessionRevoked
}

// RuntimeUser resolves the same local identity used by browser authentication.
// Internal relations use ID; the bridge challenge exposes only PublicID.
func (s *Service) RuntimeUser(ctx context.Context, userID uint) (string, int64, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return "", 0, err
	}
	if user.Status != domainuser.StatusActive || user.Sub2UserID <= 0 ||
		user.Sub2InstanceID != s.sub2.InstanceID() || user.PublicID == "" {
		return "", 0, repository.ErrNotFound
	}
	return user.PublicID, user.Sub2UserID, nil
}

// RuntimeUserByPublicID resolves an external device-enrollment identity without
// exposing the internal database ID in the client protocol.
func (s *Service) RuntimeUserByPublicID(ctx context.Context, publicID string) (uint, string, int64, error) {
	user, err := s.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return 0, "", 0, err
	}
	if user.Status != domainuser.StatusActive || user.Sub2UserID <= 0 ||
		user.Sub2InstanceID != s.sub2.InstanceID() || user.PublicID == "" {
		return 0, "", 0, repository.ErrNotFound
	}
	return user.ID, user.PublicID, user.Sub2UserID, nil
}

func (s *Service) GetLoginOptions(ctx context.Context) (*LoginOptions, error) {
	settings, err := s.sub2.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return &LoginOptions{
		EmailRegistrationEnabled: settings.RegistrationEnabled,
		EmailVerificationEnabled: settings.EmailVerifyEnabled,
		TurnstileEnabled:         settings.TurnstileEnabled,
		TurnstileSiteKey:         settings.TurnstileSiteKey,
	}, nil
}

func (s *Service) RequestEmailRegistration(ctx context.Context, email, turnstileToken string) (*EmailRegistrationStartResult, error) {
	result, err := s.sub2.SendRegistrationCode(ctx, email, turnstileToken)
	if err != nil {
		return nil, normalizeSub2RegistrationError(err)
	}
	return &EmailRegistrationStartResult{Sent: result.Countdown >= 0, ExpiresAt: time.Now().Add(time.Duration(result.Countdown) * time.Second)}, nil
}

func (s *Service) RegisterWithEmail(ctx context.Context, email, password, code, turnstileToken, requestID string, auditCtx requestmeta.SessionAuditContext) (*LoginResult, error) {
	tokens, err := s.sub2.Register(ctx, email, password, code, turnstileToken)
	if err != nil {
		return nil, normalizeSub2RegistrationError(err)
	}
	return s.finishSub2Login(ctx, *tokens, requestID, auditCtx.Normalize())
}

func (s *Service) ChangePassword(ctx context.Context, userID uint, sessionID, currentPassword, newPassword, requestID string, auditCtx requestmeta.SessionAuditContext) error {
	accessToken, err := s.sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if err = s.sub2.ChangePassword(ctx, accessToken, currentPassword, newPassword); err != nil {
		return normalizeSub2PasswordChangeError(err)
	}
	s.RecordAuthEvent(ctx, userID, requestID, "password_change", "success", "", auditCtx.ClientIP, auditCtx.UserAgent, "")
	return s.repo.RevokeAllSessions(ctx, userID, "password_changed")
}

func (s *Service) VerifyLoginTwoFactor(ctx context.Context, challengeToken, code, requestID string, auditCtx requestmeta.SessionAuditContext) (*LoginResult, error) {
	return s.verifySub2LoginTwoFactor(ctx, challengeToken, code, requestID, auditCtx.Normalize())
}
