package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/token"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	"github.com/gin-gonic/gin"
)

type authTestSessionValidator struct {
	principal *domainuser.User
	err       error
}

func (v authTestSessionValidator) ValidateAccessSession(
	context.Context,
	uint,
	string,
	time.Time,
	requestmeta.SessionAuditContext,
) (*domainuser.User, error) {
	return v.principal, v.err
}

func TestAuthMiddlewareClassifiesSessionValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "revoked", err: appauth.ErrSessionRevoked, wantStatus: http.StatusUnauthorized},
		{name: "sub2 unavailable", err: appauth.ErrSub2Unavailable, wantStatus: http.StatusBadGateway},
		{name: "storage failure", err: errors.New("database connection lost"), wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := executeAuthenticatedRequest(t, authTestSessionValidator{err: test.err}, nil)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestAuthMiddlewareProtectsMeValidationFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "sub2 temporarily unavailable", err: appauth.ErrSub2Unavailable, wantStatus: http.StatusBadGateway},
		{name: "session storage failure", err: errors.New("database connection lost"), wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlerCalled := false
			response := executeAuthenticatedMeRequest(t, authTestSessionValidator{err: test.err}, func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusNoContent)
			})
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if handlerCalled {
				t.Fatal("/me handler ran after session validation failure")
			}
		})
	}
}

func TestAuthMiddlewareUsesCurrentPrincipalRole(t *testing.T) {
	response := executeAuthenticatedRequest(t, authTestSessionValidator{principal: &domainuser.User{
		ID:       1,
		PublicID: "f6f910e920934def9a5cda479fc25251",
		Username: "remote-admin",
		Role:     domainuser.RoleSuperAdmin,
		Status:   domainuser.StatusActive,
	}}, func(c *gin.Context) {
		if got := MustUserRole(c); got != domainuser.RoleSuperAdmin {
			t.Fatalf("role = %q, want %q", got, domainuser.RoleSuperAdmin)
		}
		if got := MustUserPublicID(c); got != "f6f910e920934def9a5cda479fc25251" {
			t.Fatalf("public ID = %q", got)
		}
		c.Status(http.StatusNoContent)
	})
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func executeAuthenticatedRequest(t *testing.T, validator SessionValidator, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	const secret = "test-jwt-secret"
	accessToken, err := token.GenerateWithClaims(secret, 1, "stale-user", domainuser.RoleUser, "session-id", "jti", "access", time.Minute)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if handler == nil {
		handler = func(c *gin.Context) { c.Status(http.StatusNoContent) }
	}
	router := gin.New()
	router.GET("/", AuthMiddleware(secret, validator), handler)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func executeAuthenticatedMeRequest(t *testing.T, validator SessionValidator, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	const secret = "test-jwt-secret"
	accessToken, err := token.GenerateWithClaims(secret, 1, "stale-user", domainuser.RoleUser, "session-id", "jti", "access", time.Minute)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	router := gin.New()
	router.GET("/me", AuthMiddleware(secret, validator), handler)
	request := httptest.NewRequest(http.MethodGet, "/me", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
