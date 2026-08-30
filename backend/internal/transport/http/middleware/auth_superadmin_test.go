package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/gin-gonic/gin"
)

func TestSuperAdminOnlyRequiresExactRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		role     string
		provider string
		want     int
	}{
		{role: "admin", provider: domainuser.AuthProviderLocal, want: http.StatusForbidden},
		{role: domainuser.RoleSuperAdmin, provider: domainuser.AuthProviderRelay, want: http.StatusForbidden},
		{role: domainuser.RoleSuperAdmin, provider: domainuser.AuthProviderLocal, want: http.StatusNoContent},
	} {
		router := gin.New()
		router.GET("/", func(c *gin.Context) {
			c.Set(ContextKeyUserRole, test.role)
			c.Set(ContextKeyAuthProvider, test.provider)
		}, SuperAdminOnly(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != test.want {
			t.Fatalf("role %q provider %q: status = %d, want %d", test.role, test.provider, response.Code, test.want)
		}
	}
}

func TestRelayIdentityOnlyRejectsLocalAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		provider string
		want     int
	}{
		{provider: domainuser.AuthProviderLocal, want: http.StatusForbidden},
		{provider: domainuser.AuthProviderRelay, want: http.StatusNoContent},
	} {
		router := gin.New()
		router.GET("/", func(c *gin.Context) {
			c.Set(ContextKeyAuthProvider, test.provider)
		}, RelayIdentityOnly(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != test.want {
			t.Fatalf("provider %q: status = %d, want %d", test.provider, response.Code, test.want)
		}
	}
}
