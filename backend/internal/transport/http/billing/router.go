package billing

import "github.com/gin-gonic/gin"

// RegisterPublicRoutes 注册计费公开回调路由。
func (m *Module) RegisterPublicRoutes(publicGroup *gin.RouterGroup) {
}

// RegisterRoutes 注册计费域路由。
func (m *Module) RegisterRoutes(authRequired *gin.RouterGroup) {
	authRequired.GET("/billing/config", m.Handler.Config)
	authRequired.GET("/billing/account", m.Handler.Account)
	authRequired.GET("/billing/overview", m.Handler.Overview)
	authRequired.GET("/billing/plans", m.Handler.Plans)
	authRequired.GET("/billing/usage", m.Handler.Usage)
	authRequired.GET("/billing/usage/daily", m.Handler.Daily)
	authRequired.GET("/billing/usage/hourly", m.Handler.Hourly)
	authRequired.GET("/billing/usage/monthly", m.Handler.Monthly)
	authRequired.GET("/billing/orders", m.Handler.Orders)
	authRequired.GET("/billing/orders/:id", m.Handler.Order)
	authRequired.POST("/billing/orders/verify", m.Handler.VerifyOrder)
	authRequired.POST("/billing/orders/:id/cancel", m.Handler.CancelOrder)
	authRequired.POST("/billing/orders/:id/refund-request", m.Handler.RequestRefund)
	authRequired.POST("/billing/payments/checkout", m.Handler.Checkout)
	authRequired.GET("/billing/redemptions", m.Handler.RedemptionHistory)
	authRequired.POST("/billing/redemptions", m.Handler.Redeem)
}

// RegisterAdminRoutes 注册管理员侧计费路由。
func (m *Module) RegisterAdminRoutes(adminGroup *gin.RouterGroup) {
}
