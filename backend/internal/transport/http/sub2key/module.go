package sub2key

import "github.com/gin-gonic/gin"

type Module struct{ Handler *Handler }

func NewModule(handler *Handler) *Module { return &Module{Handler: handler} }
func (m *Module) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/me/sub2-keys", m.Handler.ListRemote)
	group.POST("/me/sub2-keys", m.Handler.CreateRemote)
	group.GET("/me/sub2-key-groups", m.Handler.ListGroups)
	group.GET("/me/sub2-key-bindings", m.Handler.ListBindings)
	group.POST("/me/sub2-key-bindings", m.Handler.Bind)
	group.DELETE("/me/sub2-key-bindings/:public_id", m.Handler.Delete)
}
