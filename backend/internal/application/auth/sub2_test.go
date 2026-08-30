package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/secretbox"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/token"
	portsub2api "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

const testSub2BaseURL = "http://127.0.0.1:1"

type sub2PrincipalTestRepo struct {
	repository.AuthRepository
	user          *domainuser.User
	session       *domainuser.Session
	revokedReason string
}

type postLockPrincipalRepo struct {
	repository.AuthRepository
	initial, refreshed *domainuser.User
	session            *domainuser.Session
	getByIDCalls       int
	revokedReason      string
	revokeErr          error
}
type stagedSub2SessionRepo struct {
	repository.AuthRepository
	user    *domainuser.User
	session *domainuser.Session
	staged  repository.UpdateSessionSub2TokensInput
	calls   int
}

type sessionReadErrorRepo struct {
	repository.AuthRepository
	user *domainuser.User
	err  error
}

type contextualSub2TestClient struct {
	portsub2api.Client
	instanceID   string
	verifyCalled bool
}

func (c *contextualSub2TestClient) InstanceID() string { return "registry" }

func (c *contextualSub2TestClient) InstanceIDForContext(context.Context) string {
	return c.instanceID
}

func (c *contextualSub2TestClient) VerifyLogin2FA(context.Context, string, string) (*portsub2api.TokenPair, error) {
	c.verifyCalled = true
	return nil, errors.New("unexpected upstream verification")
}

type runtimeAccessTokenRepo struct {
	repository.AuthRepository
	user         *domainuser.User
	sessions     []domainuser.Session
	sessionReads int
}

func (r *runtimeAccessTokenRepo) ListActiveSessionsByUserID(context.Context, uint, time.Time) ([]domainuser.Session, error) {
	return r.sessions, nil
}

func (r *runtimeAccessTokenRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	return r.user, nil
}

func (r *runtimeAccessTokenRepo) GetSessionByUserAndSessionID(_ context.Context, _ uint, sessionID string) (*domainuser.Session, error) {
	r.sessionReads++
	for i := range r.sessions {
		if r.sessions[i].SessionID == sessionID {
			return &r.sessions[i], nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r sessionReadErrorRepo) GetSessionByUserAndSessionID(context.Context, uint, string) (*domainuser.Session, error) {
	return nil, r.err
}

func (r sessionReadErrorRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	return r.user, nil
}

type refreshRejectRepo struct {
	repository.AuthRepository
	user        *domainuser.User
	session     *domainuser.Session
	revokeErr   error
	revokeCalls int
}

func (r *refreshRejectRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	return r.user, nil
}

func (r *refreshRejectRepo) GetSessionByUserAndSessionID(context.Context, uint, string) (*domainuser.Session, error) {
	return r.session, nil
}

func (r *refreshRejectRepo) RevokeSession(context.Context, uint, string, string) error {
	r.revokeCalls++
	return r.revokeErr
}

func (r *stagedSub2SessionRepo) StageSessionSub2Tokens(_ context.Context, in repository.UpdateSessionSub2TokensInput) error {
	r.staged = in
	r.calls++
	return nil
}

func (r *stagedSub2SessionRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	return r.user, nil
}

func (r *postLockPrincipalRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	r.getByIDCalls++
	if r.getByIDCalls == 1 {
		return r.initial, nil
	}
	return r.refreshed, nil
}

func (r *postLockPrincipalRepo) GetSessionByUserAndSessionID(context.Context, uint, string) (*domainuser.Session, error) {
	return r.session, nil
}

func (r *postLockPrincipalRepo) RevokeSession(_ context.Context, _ uint, _ string, reason string) error {
	r.revokedReason = reason
	return r.revokeErr
}

func (r sub2PrincipalTestRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	return r.user, nil
}

func (r *sub2PrincipalTestRepo) GetSessionByUserAndSessionID(context.Context, uint, string) (*domainuser.Session, error) {
	return r.session, nil
}

func (r *sub2PrincipalTestRepo) RevokeSession(_ context.Context, _ uint, _ string, reason string) error {
	r.revokedReason = reason
	return nil
}

func TestMapSub2Role(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		want    string
		wantErr bool
	}{
		{name: "user", role: "user", want: domainuser.RoleUser},
		{name: "admin", role: " admin ", want: domainuser.RoleSuperAdmin},
		{name: "unknown", role: "operator", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := mapSub2Role(test.role)
			if test.wantErr {
				if err == nil {
					t.Fatal("mapSub2Role() error = nil, want error")
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("mapSub2Role() = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestMapSub2Status(t *testing.T) {
	if status, err := mapSub2Status("disabled"); err != nil || status != domainuser.StatusDisabled {
		t.Fatalf("mapSub2Status() = %q, %v", status, err)
	}
}

func TestNormalizeSub2LoginErrorHandlesBusinessEnvelope(t *testing.T) {
	err := normalizeSub2LoginError(&sub2api.APIError{StatusCode: http.StatusOK, Code: 40001})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("normalizeSub2LoginError() = %v, want ErrInvalidCredentials", err)
	}
}

func TestNormalizeSub2LoginErrorMapsHumanVerification(t *testing.T) {
	err := normalizeSub2LoginError(&sub2api.APIError{StatusCode: http.StatusBadRequest, Reason: "TURNSTILE_REQUIRED"})
	if !errors.Is(err, ErrHumanVerificationFailed) {
		t.Fatalf("normalizeSub2LoginError() = %v, want %v", err, ErrHumanVerificationFailed)
	}
}

func TestSub2SessionNeedsRevalidation(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(time.Hour)
	if sub2SessionNeedsRevalidation(&domainuser.Session{
		Sub2VerifiedAt:      &now,
		Sub2AccessExpiresAt: &expiresAt,
	}, now) {
		t.Fatal("fresh verification proof must not require a remote call")
	}
	stale := now.Add(-time.Minute)
	if !sub2SessionNeedsRevalidation(&domainuser.Session{
		Sub2VerifiedAt:      &stale,
		Sub2AccessExpiresAt: &expiresAt,
	}, now) {
		t.Fatal("stale verification proof must require a remote call")
	}
	if !sub2SessionNeedsRevalidation(&domainuser.Session{Sub2VerifiedAt: &now}, now) {
		t.Fatal("missing access-token expiry must require revalidation")
	}
}

func TestSub2AccessTokensForUserPrefersUnexpiredSession(t *testing.T) {
	client, err := sub2api.New(testSub2BaseURL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	const key = "test-data-encryption-key-value-32"
	validAccessToken, err := secretbox.EncryptString(key, "runtime-access-token")
	if err != nil {
		t.Fatal(err)
	}
	expiredAccessToken, err := secretbox.EncryptString(key, "expired-access-token")
	if err != nil {
		t.Fatal(err)
	}
	refreshToken, err := secretbox.EncryptString(key, "runtime-refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	expiredAt, validUntil := now.Add(-time.Hour), now.Add(time.Hour)
	repo := &runtimeAccessTokenRepo{
		user: &domainuser.User{ID: 1, Sub2UserID: 7, Sub2InstanceID: client.InstanceID(), Status: domainuser.StatusActive},
		sessions: []domainuser.Session{
			{ID: 1, UserID: 1, SessionID: "expired", Sub2AccessTokenEncrypted: expiredAccessToken, Sub2RefreshTokenEncrypted: refreshToken, Sub2AccessExpiresAt: &expiredAt, Sub2VerifiedAt: &expiredAt, ExpiresAt: now.Add(time.Hour)},
			{ID: 2, UserID: 1, SessionID: "valid", Sub2AccessTokenEncrypted: validAccessToken, Sub2RefreshTokenEncrypted: refreshToken, Sub2AccessExpiresAt: &validUntil, Sub2VerifiedAt: &now, ExpiresAt: now.Add(time.Hour)},
		},
	}
	service := &Service{cfg: config.NewRuntime(config.Config{DataEncryptionKey: key}), sub2: client, repo: repo}
	tokens, err := service.Sub2AccessTokensForUser(context.Background(), 1)
	if err != nil || len(tokens) != 1 || tokens[0] != "runtime-access-token" {
		t.Fatalf("Sub2AccessTokensForUser() = %#v, %v", tokens, err)
	}
	if repo.sessionReads != 1 {
		t.Fatalf("session reads = %d, want 1", repo.sessionReads)
	}
}

func TestEnsureSub2SessionReturnsPostLockPrincipal(t *testing.T) {
	client, err := sub2api.New(testSub2BaseURL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("sub2api.New() error = %v", err)
	}
	key := "test-data-encryption-key-value-32"
	accessToken, err := secretbox.EncryptString(key, "access-token")
	if err != nil {
		t.Fatalf("encrypt access token: %v", err)
	}
	refreshToken, err := secretbox.EncryptString(key, "refresh-token")
	if err != nil {
		t.Fatalf("encrypt refresh token: %v", err)
	}
	now := time.Now()
	fresh := now
	expiresAt := now.Add(time.Hour)
	repo := &postLockPrincipalRepo{
		initial:   &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7, Role: domainuser.RoleSuperAdmin, Status: domainuser.StatusActive},
		refreshed: &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7, Role: domainuser.RoleUser, Status: domainuser.StatusActive},
		session: &domainuser.Session{
			UserID:                    1,
			SessionID:                 "session",
			Sub2AccessTokenEncrypted:  accessToken,
			Sub2RefreshTokenEncrypted: refreshToken,
			Sub2VerifiedAt:            &fresh,
			Sub2AccessExpiresAt:       &expiresAt,
			ExpiresAt:                 now.Add(time.Hour),
		},
	}
	stale := *repo.session
	staleVerifiedAt := now.Add(-time.Minute)
	stale.Sub2VerifiedAt = &staleVerifiedAt
	service := &Service{cfg: config.NewRuntime(config.Config{DataEncryptionKey: key}), sub2: client, repo: repo}
	principal, err := service.ensureSub2Session(context.Background(), 1, &stale)
	if err != nil {
		t.Fatalf("ensureSub2Session() error = %v", err)
	}
	if principal.Role != domainuser.RoleUser {
		t.Fatalf("ensureSub2Session() role = %q, want %q", principal.Role, domainuser.RoleUser)
	}
}

func TestEnsureSub2SessionRejectsDisabledPrincipalOnFreshPaths(t *testing.T) {
	client, err := sub2api.New(testSub2BaseURL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("sub2api.New() error = %v", err)
	}
	const key = "test-data-encryption-key-value-32"
	accessToken, err := secretbox.EncryptString(key, "access-token")
	if err != nil {
		t.Fatalf("encrypt access token: %v", err)
	}
	refreshToken, err := secretbox.EncryptString(key, "refresh-token")
	if err != nil {
		t.Fatalf("encrypt refresh token: %v", err)
	}
	storageErr := errors.New("revoke database timeout")
	tests := []struct {
		name      string
		stale     bool
		revokeErr error
		want      error
	}{
		{name: "fresh session", want: ErrSessionRevoked},
		{name: "fresh session after lock", stale: true, want: ErrSessionRevoked},
		{name: "fresh session revoke failure", revokeErr: storageErr, want: storageErr},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now()
			verifiedAt := now
			expiresAt := now.Add(time.Hour)
			repo := &postLockPrincipalRepo{
				initial:   &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7, Status: domainuser.StatusActive},
				refreshed: &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7, Status: domainuser.StatusDisabled},
				session: &domainuser.Session{
					UserID:                    1,
					SessionID:                 "session",
					Sub2AccessTokenEncrypted:  accessToken,
					Sub2RefreshTokenEncrypted: refreshToken,
					Sub2VerifiedAt:            &verifiedAt,
					Sub2AccessExpiresAt:       &expiresAt,
					ExpiresAt:                 expiresAt,
				},
				revokeErr: test.revokeErr,
			}
			input := repo.session
			if test.stale {
				stale := *repo.session
				staleVerifiedAt := now.Add(-time.Minute)
				stale.Sub2VerifiedAt = &staleVerifiedAt
				input = &stale
			}
			service := &Service{cfg: config.NewRuntime(config.Config{DataEncryptionKey: key}), sub2: client, repo: repo}
			_, err = service.ensureSub2Session(context.Background(), 1, input)
			if !errors.Is(err, test.want) {
				t.Fatalf("ensureSub2Session() error = %v, want %v", err, test.want)
			}
			if repo.revokedReason != "sub2_inactive_principal" {
				t.Fatalf("revocation reason = %q, want %q", repo.revokedReason, "sub2_inactive_principal")
			}
		})
	}
}

func TestNormalizeSub2RegistrationError(t *testing.T) {
	tests := []struct {
		reason string
		want   error
	}{
		{reason: "EMAIL_EXISTS", want: ErrEmailAlreadyExists},
		{reason: "REGISTRATION_DISABLED", want: ErrEmailRegistrationDisabled},
		{reason: "INVALID_VERIFY_CODE", want: ErrVerificationCodeInvalid},
		{reason: "EMAIL_SUFFIX_NOT_ALLOWED", want: ErrEmailDomainNotAllowed},
		{reason: "TURNSTILE_INVALID", want: ErrHumanVerificationFailed},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			got := normalizeSub2RegistrationError(&sub2api.APIError{StatusCode: http.StatusBadRequest, Reason: test.reason})
			if !errors.Is(got, test.want) {
				t.Fatalf("normalizeSub2RegistrationError() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNormalizeSub2PasswordChangeError(t *testing.T) {
	err := normalizeSub2PasswordChangeError(&sub2api.APIError{StatusCode: http.StatusBadRequest, Reason: "PASSWORD_INCORRECT"})
	if !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("normalizeSub2PasswordChangeError() = %v, want %v", err, ErrInvalidCurrentPassword)
	}
}

func TestSub2IdentityRejected(t *testing.T) {
	if !sub2IdentityRejected(&sub2api.APIError{StatusCode: http.StatusForbidden}) {
		t.Fatal("expected forbidden identity response to be rejected")
	}
	if !sub2IdentityRejected(&sub2api.APIError{StatusCode: http.StatusOK, Code: 40001}) {
		t.Fatal("expected failed business envelope to be rejected")
	}
	if sub2IdentityRejected(&sub2api.APIError{StatusCode: http.StatusTooManyRequests}) {
		t.Fatal("rate limiting must remain a temporary upstream error")
	}
	if sub2IdentityRejected(&sub2api.APIError{StatusCode: http.StatusBadGateway}) {
		t.Fatal("upstream failure must remain a temporary error")
	}
}

func TestNormalizeSub2RefreshErrorTreatsRateLimitAsTemporary(t *testing.T) {
	if err := normalizeSub2RefreshError(&sub2api.APIError{StatusCode: http.StatusTooManyRequests}); !errors.Is(err, ErrSub2Unavailable) {
		t.Fatalf("rate-limit error = %v, want %v", err, ErrSub2Unavailable)
	}
}

func TestRefreshSub2SessionStagesRotatedTokensBeforeTemporaryProfileFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/auth/refresh":
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 3600,
			}})
		case "/api/v1/auth/me":
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 503, "reason": "TEMPORARY"})
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("new Sub2 client: %v", err)
	}
	key := "test-data-encryption-key-value-32"
	oldAccess, err := secretbox.EncryptString(key, "old-access")
	if err != nil {
		t.Fatalf("encrypt old access: %v", err)
	}
	oldRefresh, err := secretbox.EncryptString(key, "old-refresh")
	if err != nil {
		t.Fatalf("encrypt old refresh: %v", err)
	}
	verifiedAt := time.Now().Add(-time.Minute)
	repo := &stagedSub2SessionRepo{
		user:    &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7, Status: domainuser.StatusActive},
		session: &domainuser.Session{UserID: 1, SessionID: "session", Sub2AccessTokenEncrypted: oldAccess, Sub2RefreshTokenEncrypted: oldRefresh, Sub2VerifiedAt: &verifiedAt, ExpiresAt: time.Now().Add(time.Hour)},
	}
	service := &Service{cfg: config.NewRuntime(config.Config{DataEncryptionKey: key}), sub2: client, repo: repo}
	_, err = service.refreshSub2SessionProfile(context.Background(), repo.user, repo.session)
	if !errors.Is(err, ErrSub2Unavailable) {
		t.Fatalf("refresh error = %v, want temporary upstream failure", err)
	}
	if repo.calls != 1 || !repo.staged.AccessExpiresAt.After(time.Now()) || !repo.staged.VerifiedAt.IsZero() {
		t.Fatalf("staged state = %#v, calls = %d", repo.staged, repo.calls)
	}
	access, err := secretbox.DecryptString(key, repo.staged.AccessTokenEncrypted)
	if err != nil || access != "new-access" {
		t.Fatalf("staged access = %q, %v", access, err)
	}
	refresh, err := secretbox.DecryptString(key, repo.staged.RefreshTokenEncrypted)
	if err != nil || refresh != "new-refresh" {
		t.Fatalf("staged refresh = %q, %v", refresh, err)
	}
}

func TestValidateAccessSessionRejectsAndRevokesMissingSub2Principal(t *testing.T) {
	client, err := sub2api.New(testSub2BaseURL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("sub2api.New() error = %v", err)
	}
	now := time.Now()
	repo := &sub2PrincipalTestRepo{
		user:    &domainuser.User{ID: 1, Status: domainuser.StatusActive},
		session: &domainuser.Session{UserID: 1, SessionID: "missing-sub2-principal", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	}
	service := &Service{cfg: config.NewRuntime(config.Config{DataEncryptionKey: "test-data-encryption-key-value-32"}), sub2: client, repo: repo}
	_, err = service.ValidateAccessSession(context.Background(), 1, "missing-sub2-principal", now, requestmeta.SessionAuditContext{})
	if !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("ValidateAccessSession() error = %v, want ErrSessionRevoked", err)
	}
	if repo.revokedReason != "sub2_principal_required" {
		t.Fatalf("missing Sub2 principal revocation reason = %q", repo.revokedReason)
	}
}

func TestSessionReadsPreserveStorageFailures(t *testing.T) {
	storageErr := errors.New("database connection lost")
	cfg := config.NewRuntime(config.Config{JWTSecret: "test-jwt-secret-value"})
	refreshToken, err := token.GenerateWithClaims("test-jwt-secret-value", 1, "user", domainuser.RoleUser, "session", "refresh-jti", "refresh", time.Hour)
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}
	service := &Service{cfg: cfg, repo: sessionReadErrorRepo{err: storageErr}}
	if _, err = service.Refresh(context.Background(), refreshToken, "", requestmeta.SessionAuditContext{}); !errors.Is(err, storageErr) {
		t.Fatalf("Refresh() error = %v, want storage error", err)
	}
	if _, err = service.ValidateAccessSession(context.Background(), 1, "session", time.Now(), requestmeta.SessionAuditContext{}); !errors.Is(err, storageErr) {
		t.Fatalf("ValidateAccessSession() error = %v, want storage error", err)
	}
}

func TestSub2AccessTokenForSessionClassifiesSessionReadErrors(t *testing.T) {
	client, err := sub2api.New(testSub2BaseURL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("sub2api.New() error = %v", err)
	}
	principal := &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7}
	storageErr := errors.New("database connection lost")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "storage failure", err: storageErr, want: storageErr},
		{name: "not found", err: repository.ErrNotFound, want: ErrSessionRevoked},
		{name: "nil session", want: ErrSessionRevoked},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &Service{
				cfg:  config.NewRuntime(config.Config{DataEncryptionKey: "test-data-encryption-key-value-32"}),
				sub2: client,
				repo: sessionReadErrorRepo{user: principal, err: test.err},
			}
			_, err := service.sub2AccessTokenForSession(context.Background(), 1, "session")
			if !errors.Is(err, test.want) {
				t.Fatalf("sub2AccessTokenForSession() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRefreshRejectsSub2IdentityByRevokingLocalSession(t *testing.T) {
	tests := []struct {
		name      string
		revokeErr error
		want      error
	}{
		{name: "revoked", want: ErrInvalidRefreshToken},
		{name: "already revoked", revokeErr: repository.ErrInvalidInput, want: ErrInvalidRefreshToken},
		{name: "revoke storage failure", revokeErr: errors.New("revoke database timeout")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &refreshRejectRepo{revokeErr: test.revokeErr}
			service, raw := newSub2RefreshRejectService(t, repo)
			_, err := service.Refresh(context.Background(), raw, "", requestmeta.SessionAuditContext{})
			if repo.revokeCalls != 1 {
				t.Fatalf("RevokeSession() calls = %d, want 1", repo.revokeCalls)
			}
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("Refresh() error = %v, want %v", err, test.want)
				}
				return
			}
			if !errors.Is(err, test.revokeErr) {
				t.Fatalf("Refresh() error = %v, want revoke error %v", err, test.revokeErr)
			}
		})
	}
}

func newSub2RefreshRejectService(t *testing.T, repo *refreshRejectRepo) (*Service, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/auth/refresh" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		writer.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(writer).Encode(map[string]any{"code": 401, "reason": "UNAUTHORIZED"})
	}))
	t.Cleanup(server.Close)
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("new Sub2 client: %v", err)
	}
	const jwtSecret = "test-jwt-secret-value"
	const encryptionKey = "test-data-encryption-key-value-32"
	raw, err := token.GenerateWithClaims(jwtSecret, 1, "user", domainuser.RoleUser, "session", "refresh-jti", "refresh", time.Hour)
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}
	accessToken, err := secretbox.EncryptString(encryptionKey, "access-token")
	if err != nil {
		t.Fatalf("encrypt access token: %v", err)
	}
	refreshToken, err := secretbox.EncryptString(encryptionKey, "sub2-refresh-token")
	if err != nil {
		t.Fatalf("encrypt refresh token: %v", err)
	}
	repo.user = &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7, Status: domainuser.StatusActive}
	repo.session = &domainuser.Session{
		UserID:                    1,
		SessionID:                 "session",
		RefreshTokenHash:          hashToken(raw),
		Sub2AccessTokenEncrypted:  accessToken,
		Sub2RefreshTokenEncrypted: refreshToken,
		ExpiresAt:                 time.Now().Add(time.Hour),
	}
	return &Service{cfg: config.NewRuntime(config.Config{JWTSecret: jwtSecret, DataEncryptionKey: encryptionKey}), repo: repo, sub2: client}, raw
}

func TestSessionKeyedLock(t *testing.T) {
	var locks sessionKeyedLock
	releaseFirst := locks.lock(1, "session")
	attempted := make(chan struct{})
	acquired := make(chan func())
	go func() {
		close(attempted)
		acquired <- locks.lock(1, "session")
	}()
	<-attempted
	select {
	case release := <-acquired:
		release()
		t.Fatal("same session lock was not serialized")
	default:
	}
	releaseFirst()
	releaseSecond := <-acquired
	releaseSecond()

	releaseFirst = locks.lock(1, "shared-session")
	differentKeyAcquired := make(chan func())
	go func() { differentKeyAcquired <- locks.lock(2, "shared-session") }()
	releaseDifferent := <-differentKeyAcquired
	releaseDifferent()
	releaseFirst()

	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.entries) != 0 {
		t.Fatalf("idle lock entries = %d, want 0", len(locks.entries))
	}
}

func TestSub2ChallengeIsEncryptedAndExpires(t *testing.T) {
	service := &Service{cfg: config.NewRuntime(config.Config{DataEncryptionKey: "test-data-encryption-key-value-32"})}
	sealed, err := service.sealSub2Challenge("upstream-temp-token", "relay-1")
	if err != nil {
		t.Fatalf("sealSub2Challenge() error = %v", err)
	}
	if sealed == "" || sealed == "upstream-temp-token" {
		t.Fatal("sealed challenge exposed the upstream token")
	}
	opened, err := service.openSub2Challenge(sealed)
	if err != nil {
		t.Fatalf("openSub2Challenge() error = %v", err)
	}
	if opened.TempToken != "upstream-temp-token" {
		t.Fatalf("openSub2Challenge() temp token = %q", opened.TempToken)
	}
	if opened.ConnectorID != "relay-1" {
		t.Fatalf("openSub2Challenge() connector = %q", opened.ConnectorID)
	}

	_, err = service.openSub2Challenge("invalid")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("openSub2Challenge(invalid) error = %v, want ErrInvalidCredentials", err)
	}
}

func TestSub2ChallengeCannotCrossConnectors(t *testing.T) {
	client := &contextualSub2TestClient{instanceID: "relay-1"}
	service := &Service{
		cfg:  config.NewRuntime(config.Config{DataEncryptionKey: "test-data-encryption-key-value-32"}),
		sub2: client,
	}
	sealed, err := service.sealSub2Challenge("upstream-temp-token", "relay-1")
	if err != nil {
		t.Fatalf("sealSub2Challenge() error = %v", err)
	}
	client.instanceID = "relay-2"

	_, err = service.VerifyLoginTwoFactor(context.Background(), sealed, "123456", "", requestmeta.SessionAuditContext{})
	if !errors.Is(err, ErrTwoFactorChallengeExpired) {
		t.Fatalf("VerifyLoginTwoFactor() error = %v, want %v", err, ErrTwoFactorChallengeExpired)
	}
	if client.verifyCalled {
		t.Fatal("VerifyLoginTwoFactor() called the wrong connector")
	}
}
