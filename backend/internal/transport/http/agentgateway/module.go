package agentgateway

import "github.com/gin-gonic/gin"

type Module struct{ Handler *Handler }

func NewModule(handler *Handler) *Module { return &Module{Handler: handler} }

func (m *Module) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/agent/devices/enrollments", m.Handler.CreateEnrollment)
	group.GET("/agent/devices", m.Handler.ListDevices)
	group.GET("/agent/devices/:device_id", m.Handler.GetDevice)
	group.PATCH("/agent/devices/:device_id", m.Handler.RenameDevice)
	group.DELETE("/agent/devices/:device_id", m.Handler.RevokeDevice)
	group.GET("/agent/devices/:device_id/profiles", m.Handler.ListRuntimeProfiles)
	group.GET("/agent/devices/:device_id/workspaces", m.Handler.ListWorkspaces)
	group.POST("/agent/workspaces/:workspace_id/artifacts", m.Handler.CreateArtifact)
	group.GET("/agent/devices/:device_id/profiles/:profile_id/resources/:resource", m.Handler.GetProfileResource)
	group.POST("/agent/devices/:device_id/profiles/:profile_id/resources/:resource/refresh", m.Handler.QueueProfileResourceRefresh)
	group.GET("/agent/devices/:device_id/workspaces/:workspace_id/resources/:resource", m.Handler.GetWorkspaceResource)
	group.POST("/agent/devices/:device_id/workspaces/:workspace_id/resources/:resource/refresh", m.Handler.QueueWorkspaceResourceRefresh)
	group.GET("/agent/threads", m.Handler.ListThreads)
	group.POST("/agent/threads", m.Handler.StartThread)
	group.GET("/agent/threads/:thread_id", m.Handler.GetThread)
	group.GET("/agent/threads/:thread_id/snapshot", m.Handler.GetThreadSnapshot)
	group.PATCH("/agent/threads/:thread_id", m.Handler.UpdateThreadMetadata)
	group.PATCH("/agent/threads/:thread_id/provider-metadata", m.Handler.UpdateProviderMetadata)
	group.PATCH("/agent/threads/:thread_id/name", m.Handler.RenameThread)
	group.POST("/agent/threads/:thread_id/fork", m.Handler.ForkThread)
	group.POST("/agent/threads/:thread_id/resume", m.Handler.ResumeThread)
	group.POST("/agent/threads/:thread_id/archive", m.Handler.ArchiveThread)
	group.POST("/agent/threads/:thread_id/unarchive", m.Handler.UnarchiveThread)
	group.DELETE("/agent/threads/:thread_id", m.Handler.DeleteThread)
	group.POST("/agent/threads/:thread_id/compact", m.Handler.CompactThread)
	group.POST("/agent/threads/:thread_id/reviews", m.Handler.StartReview)
	group.POST("/agent/threads/:thread_id/turns", m.Handler.StartTurn)
	group.GET("/agent/threads/:thread_id/turns", m.Handler.ListTurns)
	group.GET("/agent/threads/:thread_id/items", m.Handler.ListItems)
	group.POST("/agent/turns/:turn_id/steer", m.Handler.SteerTurn)
	group.POST("/agent/turns/:turn_id/interrupt", m.Handler.InterruptTurn)
	group.GET("/agent/threads/:thread_id/events", m.Handler.ListEvents)
	group.GET("/agent/threads/:thread_id/notifications", m.Handler.StreamThreadNotifications)
	group.GET("/agent/threads/:thread_id/interactions", m.Handler.ListInteractions)
	group.POST("/agent/interactions/:interaction_id/respond", m.Handler.RespondInteraction)
}

func (m *Module) RegisterBridgeRoutes(group *gin.RouterGroup) {
	group.POST("/agent/bridge/enroll", m.Handler.EnrollDevice)
	group.POST("/agent/bridge/token-challenges", m.Handler.CreateChallenge)
	group.POST("/agent/bridge/tokens", m.Handler.IssueConnection)
	group.GET("/agent/bridge/connect", m.Handler.ConnectBridge)
	group.GET("/agent/bridge/artifacts/:artifact_id/content", m.Handler.GetArtifactContent)
}
