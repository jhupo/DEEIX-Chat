package admin

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers administrator routes.
func (m *Module) RegisterRoutes(adminGroup *gin.RouterGroup) {
	updates := adminGroup.Group("/update")
	updates.Use(middleware.SuperAdminOnly())
	updates.GET("/status", m.Handler.UpdateStatus)
	updates.POST("/check", m.Handler.CheckUpdate)
	updates.POST("/install", m.Handler.InstallUpdate)
	updates.POST("/restart", m.Handler.RestartAfterUpdate)
	updates.GET("/jobs/:job_id", m.Handler.UpdateJob)

	adminGroup.GET("/users", m.Handler.ListUsers)
	adminGroup.PATCH("/users/:id", m.Handler.PatchUser)
	adminGroup.POST("/users/:id/revoke-sessions", m.Handler.RevokeUserSessions)
	adminGroup.GET("/user-auth-events", m.Handler.ListUserAuthEvents)
	adminGroup.GET("/audit-logs", m.Handler.ListAuditLogs)
	adminGroup.GET("/conversation-events", m.Handler.ListConversationEvents)
	adminGroup.GET("/conversation-events/:id", m.Handler.GetConversationEvent)
	adminGroup.POST("/conversation-events/cleanup", m.Handler.CleanupConversationRuns)
	adminGroup.GET("/system-events", m.Handler.ListSystemEvents)
	adminGroup.POST("/logs/cleanup", m.Handler.CleanupLogs)
	adminGroup.GET("/permission-groups", m.Handler.ListPermissionGroups)
	adminGroup.POST("/permission-groups", m.Handler.CreatePermissionGroup)
	adminGroup.PATCH("/permission-groups/:id", m.Handler.UpdatePermissionGroup)
	adminGroup.DELETE("/permission-groups/:id", m.Handler.DeletePermissionGroup)
	adminGroup.GET("/permission-groups/:id/models", m.Handler.ListGroupModels)
	adminGroup.PUT("/permission-groups/:id/models", m.Handler.SetGroupModels)
	adminGroup.GET("/models/:modelID/permission-groups", m.Handler.ListModelPermissionGroups)
	adminGroup.PUT("/models/:modelID/permission-groups", m.Handler.SetModelPermissionGroups)
	adminGroup.GET("/permission-groups/:id/users", m.Handler.ListGroupUsers)
	adminGroup.PUT("/permission-groups/:id/users", m.Handler.SetGroupUsers)
}
