package relay

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func (m *Module) RegisterRoutes(adminGroup *gin.RouterGroup) {
	g := adminGroup.Group("/relays")
	g.Use(middleware.SuperAdminOnly())
	g.GET("/connectors", m.Handler.ListConnectors)
	g.POST("/connectors", m.Handler.CreateConnector)
	g.PATCH("/connectors/:id", m.Handler.UpdateConnector)
	g.DELETE("/connectors/:id", m.Handler.DeleteConnector)
	g.GET("/routes", m.Handler.ListRoutes)
	g.POST("/routes", m.Handler.CreateRoute)
	g.PATCH("/routes/:id", m.Handler.UpdateRoute)
	g.DELETE("/routes/:id", m.Handler.DeleteRoute)
}
