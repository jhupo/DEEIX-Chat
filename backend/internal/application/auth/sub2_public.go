package auth

import (
	"context"
	"time"

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
