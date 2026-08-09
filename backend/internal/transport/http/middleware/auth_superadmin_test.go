package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSuperAdminOnlyRequiresExactRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"admin", "superadmin"} {
		router := gin.New()
		router.GET("/", func(c *gin.Context) { c.Set(ContextKeyUserRole, role) }, SuperAdminOnly(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		want := http.StatusForbidden
		if role == "superadmin" {
			want = http.StatusNoContent
		}
		if response.Code != want {
			t.Fatalf("role %q: status = %d, want %d", role, response.Code, want)
		}
	}
}
