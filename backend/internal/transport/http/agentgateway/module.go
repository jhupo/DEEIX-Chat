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
}

func (m *Module) RegisterBridgeRoutes(group *gin.RouterGroup) {
	group.POST("/agent/bridge/enroll", m.Handler.EnrollDevice)
	group.POST("/agent/bridge/token-challenges", m.Handler.CreateChallenge)
	group.POST("/agent/bridge/tokens", m.Handler.IssueConnection)
	group.GET("/agent/bridge/connect", m.Handler.ConnectBridge)
	group.GET("/agent/bridge/artifacts/:artifact_id/content", m.Handler.GetArtifactContent)
}
