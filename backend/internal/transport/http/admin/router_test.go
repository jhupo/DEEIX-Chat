package admin

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUserIdentityResetRoutesAreNotRegistered(t *testing.T) {
	router := gin.New()
	NewModule(&Handler{}).RegisterRoutes(router.Group("/api/admin"))
	for _, route := range router.Routes() {
		if route.Path == "/api/admin/users/:id/reset-password" || route.Path == "/api/admin/users/:id/reset-2fa" {
			t.Fatalf("legacy identity route remains registered: %s %s", route.Method, route.Path)
		}
	}
}

func TestUpdateRestartRouteIsRegistered(t *testing.T) {
	router := gin.New()
	NewModule(&Handler{}).RegisterRoutes(router.Group("/api/admin"))
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/admin/update/restart" {
			return
		}
	}
	t.Fatal("update restart route is not registered")
}
