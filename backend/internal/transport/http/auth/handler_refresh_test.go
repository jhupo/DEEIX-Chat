package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type changePasswordStorageRepo struct {
	repository.AuthRepository
	user *domainuser.User
	err  error
}

func (r changePasswordStorageRepo) GetByID(context.Context, uint) (*domainuser.User, error) {
	return r.user, nil
}

func (r changePasswordStorageRepo) GetSessionByUserAndSessionID(context.Context, uint, string) (*domainuser.Session, error) {
	return nil, r.err
}

func TestRefreshTokenTemporaryFailurePreservesCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	ctx.Request.AddCookie(&http.Cookie{Name: refreshTokenCookieName, Value: "refresh-token"})
	(&Handler{refresh: func(context.Context, string, string, requestmeta.SessionAuditContext) (*appauth.LoginResult, error) {
		return nil, appauth.ErrSub2Unavailable
	}}).RefreshToken(ctx)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("temporary refresh status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
	for _, value := range recorder.Header().Values("Set-Cookie") {
		if strings.Contains(value, refreshTokenCookieName) {
			t.Fatalf("temporary failure deleted refresh cookie: %q", value)
		}
	}
}

func TestRefreshTokenInvalidFailureClearsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	ctx.Request.AddCookie(&http.Cookie{Name: refreshTokenCookieName, Value: "refresh-token"})
	(&Handler{refresh: func(context.Context, string, string, requestmeta.SessionAuditContext) (*appauth.LoginResult, error) {
		return nil, appauth.ErrInvalidRefreshToken
	}}).RefreshToken(ctx)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid refresh status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if values := recorder.Header().Values("Set-Cookie"); len(values) != 1 || !strings.Contains(values[0], refreshTokenCookieName) || !strings.Contains(values[0], "Max-Age=0") {
		t.Fatalf("invalid refresh Set-Cookie = %#v", values)
	}
}

func TestChangePasswordStorageFailureReturnsInternalServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, err := sub2api.New(config.DefaultSub2BaseURL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("sub2api.New() error = %v", err)
	}
	service, err := appauth.NewServiceWithRuntime(
		config.NewRuntime(config.Config{DataEncryptionKey: "test-data-encryption-key-value-32"}),
		changePasswordStorageRepo{
			user: &domainuser.User{ID: 1, Sub2InstanceID: client.InstanceID(), Sub2UserID: 7},
			err:  errors.New("database connection lost"),
		},
		nil,
		client,
	)
	if err != nil {
		t.Fatalf("NewServiceWithRuntime() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/me/password", strings.NewReader(`{"currentPassword":"old-password","newPassword":"new-password"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(middleware.ContextKeyUserID, uint(1))
	ctx.Set(middleware.ContextKeySessionID, "session")

	NewHandler(service).ChangePassword(ctx)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("ChangePassword() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
