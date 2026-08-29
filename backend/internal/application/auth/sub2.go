package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/conv"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/secretbox"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	"github.com/google/uuid"
)

const sub2ChallengeTTL = 10 * time.Minute

type sub2SessionCredentials struct {
	AccessTokenEncrypted  string
	RefreshTokenEncrypted string
	AccessExpiresAt       time.Time
	VerifiedAt            time.Time
}

type sub2Challenge struct {
	TempToken string `json:"tempToken"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Service) loginWithSub2(
	ctx context.Context,
	email string,
	password string,
	turnstileToken string,
	requestID string,
	auditCtx requestmeta.SessionAuditContext,
) (*LoginResult, error) {
	result, err := s.sub2.Login(ctx, email, password, turnstileToken)
	if err != nil {
		s.RecordAuthEvent(ctx, 0, requestID, "login", "failure", "sub2_rejected", auditCtx.ClientIP, auditCtx.UserAgent, "")
		return nil, normalizeSub2LoginError(err)
	}
	if result.Requires2FA {
		challengeToken, sealErr := s.sealSub2Challenge(result.TempToken)
		if sealErr != nil {
			return nil, sealErr
		}
		return &LoginResult{
			TwoFactorRequired:       true,
			TwoFactorChallengeToken: challengeToken,
		}, nil
	}
	return s.finishSub2Login(ctx, result.TokenPair, requestID, auditCtx)
}

func (s *Service) verifySub2LoginTwoFactor(
	ctx context.Context,
	challengeToken string,
	code string,
	requestID string,
	auditCtx requestmeta.SessionAuditContext,
) (*LoginResult, error) {
	challenge, err := s.openSub2Challenge(challengeToken)
	if err != nil {
		return nil, err
	}
	tokens, err := s.sub2.VerifyLogin2FA(ctx, challenge.TempToken, code)
	if err != nil {
		s.RecordAuthEvent(ctx, 0, requestID, "two_factor_verify", "failure", "sub2_rejected", auditCtx.ClientIP, auditCtx.UserAgent, "")
		return nil, normalizeSub2LoginError(err)
	}
	return s.finishSub2Login(ctx, *tokens, requestID, auditCtx)
}

func (s *Service) finishSub2Login(
	ctx context.Context,
	tokens sub2api.TokenPair,
	requestID string,
	auditCtx requestmeta.SessionAuditContext,
) (*LoginResult, error) {
	if strings.TrimSpace(tokens.AccessToken) == "" || strings.TrimSpace(tokens.RefreshToken) == "" || tokens.ExpiresIn <= 0 {
		return nil, errors.New("Sub2API returned an incomplete token pair")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.sub2.Logout(cleanupCtx, tokens.RefreshToken)
	}()
	remoteUser, err := s.sub2.Me(ctx, tokens.AccessToken)
	if err != nil {
		return nil, normalizeSub2LoginError(err)
	}
	now := time.Now()
	principal, err := s.upsertSub2Principal(ctx, remoteUser, now)
	if err != nil {
		return nil, err
	}
	if principal.Status != domainuser.StatusActive {
		return nil, ErrInvalidCredentials
	}
	credentials, err := s.encryptSub2SessionCredentials(tokens, now)
	if err != nil {
		return nil, err
	}
	result, err := s.issueLoginResultWithSub2(ctx, principal, auditCtx, now, credentials)
	if err != nil {
		return nil, err
	}
	committed = true
	s.RecordAuthEvent(ctx, principal.ID, requestID, "login", "success", "", auditCtx.ClientIP, auditCtx.UserAgent, marshalAuthEventDetail(map[string]interface{}{
		"session_id": result.SessionID,
		"authority":  "sub2",
	}))
	return result, nil
}

func (s *Service) upsertSub2Principal(ctx context.Context, remote *sub2api.User, now time.Time) (*domainuser.User, error) {
	if remote == nil || remote.ID <= 0 {
		return nil, ErrInvalidCredentials
	}
	role, err := mapSub2Role(remote.Role)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	status, err := mapSub2Status(remote.Status)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	displayName := strings.TrimSpace(remote.Username)
	if displayName == "" {
		displayName = strings.TrimSpace(remote.Email)
	}
	if displayName == "" {
		displayName = "Sub2 User"
	}
	return s.repo.UpsertSub2Principal(ctx, &domainuser.User{
		Sub2InstanceID: s.sub2.InstanceID(),
		Sub2UserID:     remote.ID,
		PublicID:       conv.NormalizePublicID(uuid.NewString()),
		Username:       sub2PrincipalUsername(s.sub2.InstanceID(), remote.ID),
		DisplayName:    displayName,
		AvatarURL:      strings.TrimSpace(remote.AvatarURL),
		Email:          strings.ToLower(strings.TrimSpace(remote.Email)),
		Role:           role,
		Status:         status,
		Timezone:       "Etc/UTC",
		Locale:         "en-US",
	})
}

func mapSub2Status(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case domainuser.StatusActive:
		return domainuser.StatusActive, nil
	case domainuser.StatusDisabled:
		return domainuser.StatusDisabled, nil
	default:
		return "", fmt.Errorf("unsupported Sub2 status %q", status)
	}
}

func mapSub2Role(role string) (string, error) {
	// Sub2 admin is its highest authority, so DEEIX represents it as superadmin.
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return domainuser.RoleSuperAdmin, nil
	case "user":
		return domainuser.RoleUser, nil
	default:
		return "", fmt.Errorf("unsupported Sub2 role %q", role)
	}
}

func sub2PrincipalUsername(instanceID string, remoteUserID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", instanceID, remoteUserID)))
	return "sub2_" + hex.EncodeToString(sum[:8])
}

func (s *Service) encryptSub2SessionCredentials(tokens sub2api.TokenPair, now time.Time) (*sub2SessionCredentials, error) {
	secret := s.cfg.Snapshot().DataEncryptionKey
	accessToken, err := secretbox.EncryptString(secret, tokens.AccessToken)
	if err != nil {
		return nil, err
	}
	refreshToken, err := secretbox.EncryptString(secret, tokens.RefreshToken)
	if err != nil {
		return nil, err
	}
	return &sub2SessionCredentials{
		AccessTokenEncrypted:  accessToken,
		RefreshTokenEncrypted: refreshToken,
		AccessExpiresAt:       now.Add(time.Duration(tokens.ExpiresIn) * time.Second),
		VerifiedAt:            now,
	}, nil
}

func (s *Service) sealSub2Challenge(tempToken string) (string, error) {
	if strings.TrimSpace(tempToken) == "" {
		return "", ErrInvalidCredentials
	}
	payload, err := json.Marshal(sub2Challenge{TempToken: tempToken, ExpiresAt: time.Now().Add(sub2ChallengeTTL).Unix()})
	if err != nil {
		return "", err
	}
	return secretbox.EncryptString(s.cfg.Snapshot().DataEncryptionKey, string(payload))
}

func (s *Service) openSub2Challenge(value string) (*sub2Challenge, error) {
	payload, err := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, strings.TrimSpace(value))
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	var challenge sub2Challenge
	if err = json.Unmarshal([]byte(payload), &challenge); err != nil || strings.TrimSpace(challenge.TempToken) == "" {
		return nil, ErrInvalidCredentials
	}
	if time.Now().Unix() >= challenge.ExpiresAt {
		return nil, ErrTwoFactorChallengeExpired
	}
	return &challenge, nil
}

func normalizeSub2LoginError(err error) error {
	if errors.Is(err, sub2api.ErrUnauthorized) {
		return ErrInvalidCredentials
	}
	var apiErr *sub2api.APIError
	if errors.As(err, &apiErr) {
		switch strings.ToUpper(strings.TrimSpace(apiErr.Reason)) {
		case "TURNSTILE_INVALID", "TURNSTILE_REQUIRED", "CAPTCHA_INVALID", "CAPTCHA_REQUIRED":
			return ErrHumanVerificationFailed
		}
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429 || apiErr.StatusCode >= 200 && apiErr.StatusCode < 300 {
			return ErrInvalidCredentials
		}
	}
	return ErrSub2Unavailable
}

func normalizeSub2RegistrationError(err error) error {
	var apiErr *sub2api.APIError
	if !errors.As(err, &apiErr) {
		return ErrSub2Unavailable
	}
	switch strings.ToUpper(strings.TrimSpace(apiErr.Reason)) {
	case "EMAIL_EXISTS":
		return ErrEmailAlreadyExists
	case "REGISTRATION_DISABLED":
		return ErrEmailRegistrationDisabled
	case "EMAIL_VERIFY_REQUIRED":
		return ErrEmailVerificationRequired
	case "INVALID_VERIFY_CODE", "VERIFY_CODE_MAX_ATTEMPTS":
		return ErrVerificationCodeInvalid
	case "EMAIL_SUFFIX_NOT_ALLOWED", "EMAIL_DOMAIN_REGISTRATION_LIMIT":
		return ErrEmailDomainNotAllowed
	case "TURNSTILE_INVALID", "TURNSTILE_REQUIRED", "CAPTCHA_INVALID", "CAPTCHA_REQUIRED":
		return ErrHumanVerificationFailed
	default:
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return ErrInvalidCredentials
		}
		return ErrSub2Unavailable
	}
}

func normalizeSub2PasswordChangeError(err error) error {
	var apiErr *sub2api.APIError
	if errors.As(err, &apiErr) && strings.EqualFold(strings.TrimSpace(apiErr.Reason), "PASSWORD_INCORRECT") {
		return ErrInvalidCurrentPassword
	}
	if errors.Is(err, sub2api.ErrUnauthorized) || errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return ErrInvalidCredentials
	}
	return ErrSub2Unavailable
}

func (s *Service) refreshSub2SessionProfile(
	ctx context.Context,
	principal *domainuser.User,
	session *domainuser.Session,
) (*domainuser.User, error) {
	if principal == nil || session == nil || principal.Sub2UserID <= 0 || principal.Sub2InstanceID != s.sub2.InstanceID() {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_principal_required")
	}
	refreshToken, err := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2RefreshTokenEncrypted)
	if err != nil || strings.TrimSpace(refreshToken) == "" {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	tokens, err := s.sub2.Refresh(ctx, refreshToken)
	if err != nil {
		return nil, normalizeSub2RefreshError(err)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" || strings.TrimSpace(tokens.RefreshToken) == "" || tokens.ExpiresIn <= 0 {
		return nil, ErrSub2Unavailable
	}
	credentials, err := s.encryptSub2SessionCredentials(*tokens, time.Now())
	if err != nil {
		return nil, err
	}
	if err = s.repo.StageSessionSub2Tokens(ctx, repository.UpdateSessionSub2TokensInput{
		UserID:                principal.ID,
		SessionID:             session.SessionID,
		AccessTokenEncrypted:  credentials.AccessTokenEncrypted,
		RefreshTokenEncrypted: credentials.RefreshTokenEncrypted,
		AccessExpiresAt:       credentials.AccessExpiresAt,
	}); err != nil {
		if errors.Is(err, repository.ErrInvalidInput) {
			return nil, ErrSessionRevoked
		}
		return nil, err
	}
	remoteUser, err := s.sub2.Me(ctx, tokens.AccessToken)
	if err != nil {
		return nil, normalizeSub2RefreshError(err)
	}
	return s.persistVerifiedSub2Session(ctx, principal, session, remoteUser, *tokens, time.Now())
}

func (s *Service) sub2AccessTokenForSession(ctx context.Context, userID uint, sessionID string) (string, error) {
	principal, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if principal.Sub2UserID <= 0 || principal.Sub2InstanceID != s.sub2.InstanceID() {
		return "", s.rejectSub2Session(ctx, principal, nil, "sub2_principal_required")
	}
	session, err := s.repo.GetSessionByUserAndSessionID(ctx, userID, strings.TrimSpace(sessionID))
	if errors.Is(err, repository.ErrNotFound) {
		return "", s.rejectSub2Session(ctx, principal, nil, "sub2_invalid_session")
	}
	if err != nil {
		return "", err
	}
	if session == nil || session.RevokedAt != nil || time.Now().After(session.ExpiresAt) {
		return "", s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	accessToken, err := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2AccessTokenEncrypted)
	if err != nil || strings.TrimSpace(accessToken) == "" {
		return "", s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	return accessToken, nil
}

func (s *Service) verifySub2SessionProfile(
	ctx context.Context,
	principal *domainuser.User,
	session *domainuser.Session,
) (*domainuser.User, error) {
	if principal == nil || session == nil || principal.Sub2UserID <= 0 || principal.Sub2InstanceID != s.sub2.InstanceID() {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_principal_required")
	}
	accessToken, err := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2AccessTokenEncrypted)
	if err != nil || strings.TrimSpace(accessToken) == "" {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	remoteUser, err := s.sub2.Me(ctx, accessToken)
	if sub2IdentityRejected(err) {
		return s.refreshSub2SessionProfile(ctx, principal, session)
	}
	if err != nil {
		return nil, normalizeSub2RefreshError(err)
	}
	refreshToken, err := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2RefreshTokenEncrypted)
	if err != nil || strings.TrimSpace(refreshToken) == "" {
		return nil, ErrSessionRevoked
	}
	expiresAt := time.Now().Add(time.Minute)
	if session.Sub2AccessExpiresAt != nil {
		expiresAt = *session.Sub2AccessExpiresAt
	}
	return s.persistVerifiedSub2Session(ctx, principal, session, remoteUser, sub2api.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    max(1, int(time.Until(expiresAt).Seconds())),
	}, time.Now())
}

func sub2IdentityRejected(err error) bool {
	if errors.Is(err, sub2api.ErrUnauthorized) {
		return true
	}
	var apiErr *sub2api.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode >= 200 && apiErr.StatusCode < 300 ||
		apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429
}

func (s *Service) persistVerifiedSub2Session(
	ctx context.Context,
	principal *domainuser.User,
	session *domainuser.Session,
	remoteUser *sub2api.User,
	tokens sub2api.TokenPair,
	now time.Time,
) (*domainuser.User, error) {
	if remoteUser == nil || remoteUser.ID != principal.Sub2UserID {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_principal_mismatch")
	}
	updatedPrincipal, err := s.upsertSub2Principal(ctx, remoteUser, now)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_identity")
		}
		return nil, err
	}
	if updatedPrincipal.Status != domainuser.StatusActive {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_inactive_principal")
	}
	credentials, err := s.encryptSub2SessionCredentials(tokens, now)
	if err != nil {
		return nil, err
	}
	if err = s.repo.UpdateSessionSub2Tokens(ctx, repository.UpdateSessionSub2TokensInput{
		UserID:                principal.ID,
		SessionID:             session.SessionID,
		AccessTokenEncrypted:  credentials.AccessTokenEncrypted,
		RefreshTokenEncrypted: credentials.RefreshTokenEncrypted,
		AccessExpiresAt:       credentials.AccessExpiresAt,
		VerifiedAt:            now,
	}); err != nil {
		if errors.Is(err, repository.ErrInvalidInput) {
			return nil, ErrSessionRevoked
		}
		return nil, err
	}
	return updatedPrincipal, nil
}

func (s *Service) ensureSub2Session(ctx context.Context, userID uint, session *domainuser.Session) (*domainuser.User, error) {
	principal, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if principal.Sub2UserID <= 0 || principal.Sub2InstanceID != s.sub2.InstanceID() || session == nil {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_principal_required")
	}
	accessToken, accessErr := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2AccessTokenEncrypted)
	refreshToken, refreshErr := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2RefreshTokenEncrypted)
	if accessErr != nil || refreshErr != nil || strings.TrimSpace(accessToken) == "" || strings.TrimSpace(refreshToken) == "" {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	if !sub2SessionNeedsRevalidation(session, time.Now()) {
		principal, err = s.repo.GetByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if principal.Status != domainuser.StatusActive {
			return nil, s.rejectSub2Session(ctx, principal, session, "sub2_inactive_principal")
		}
		return principal, nil
	}

	// Re-read after acquiring the session lock so concurrent requests reuse one proof update.
	release := s.sub2RefreshLocks.lock(userID, session.SessionID)
	defer release()
	session, err = s.repo.GetSessionByUserAndSessionID(ctx, userID, session.SessionID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	if err != nil {
		return nil, err
	}
	if session == nil || session.RevokedAt != nil || time.Now().After(session.ExpiresAt) {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_session")
	}
	if !sub2SessionNeedsRevalidation(session, time.Now()) {
		principal, err = s.repo.GetByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if principal.Status != domainuser.StatusActive {
			return nil, s.rejectSub2Session(ctx, principal, session, "sub2_inactive_principal")
		}
		return principal, nil
	}
	principal, err = s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	principal, err = s.verifySub2SessionProfile(ctx, principal, session)
	if errors.Is(err, ErrInvalidRefreshToken) || errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrSessionRevoked) {
		return nil, s.rejectSub2Session(ctx, principal, session, "sub2_invalid_identity")
	}
	if err != nil {
		return nil, err
	}
	return principal, nil
}

func sub2SessionNeedsRevalidation(session *domainuser.Session, now time.Time) bool {
	if session == nil || session.Sub2VerifiedAt == nil || session.Sub2AccessExpiresAt == nil {
		return true
	}
	return now.Sub(*session.Sub2VerifiedAt) >= time.Minute || !session.Sub2AccessExpiresAt.After(now)
}

func (s *Service) rejectSub2Session(ctx context.Context, principal *domainuser.User, session *domainuser.Session, reason string) error {
	if principal != nil && session != nil && principal.ID != 0 && strings.TrimSpace(session.SessionID) != "" {
		if err := s.repo.RevokeSession(ctx, principal.ID, session.SessionID, reason); err != nil && !errors.Is(err, repository.ErrInvalidInput) {
			return err
		}
	}
	return ErrSessionRevoked
}

func normalizeSub2RefreshError(err error) error {
	if errors.Is(err, sub2api.ErrUnauthorized) {
		return ErrInvalidRefreshToken
	}
	var apiErr *sub2api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429 || apiErr.StatusCode >= 200 && apiErr.StatusCode < 300 {
			return ErrInvalidRefreshToken
		}
	}
	return ErrSub2Unavailable
}
