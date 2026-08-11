package auth

import "github.com/gin-gonic/gin"

// RegisterPublicRoutes 注册无需登录的鉴权路由。
func (m *Module) RegisterPublicRoutes(api *gin.RouterGroup) {
	api.GET("/auth/login-options", m.Handler.LoginOptions)
	api.POST("/auth/login", m.Handler.Login)
	api.POST("/auth/login/2fa", m.Handler.VerifyTwoFactorLogin)
	api.POST("/auth/register/email/start", m.Handler.StartEmailRegistration)
	api.POST("/auth/register/email/complete", m.Handler.CompleteEmailRegistration)
	api.POST("/auth/refresh", m.Handler.RefreshToken)
}

// RegisterProtectedRoutes 注册需登录的鉴权路由。
func (m *Module) RegisterProtectedRoutes(authRequired *gin.RouterGroup) {
	authRequired.GET("/me", m.Handler.Me)
	authRequired.PATCH("/me", m.Handler.PatchMe)
	authRequired.PUT("/me/password", m.Handler.ChangePassword)
	authRequired.GET("/auth/sessions", m.Handler.CurrentSessions)
	authRequired.PUT("/auth/sessions/current/location", m.Handler.UpdateCurrentSessionLocation)
	authRequired.POST("/auth/sessions/:session_id/logout", m.Handler.LogoutSession)
	authRequired.POST("/auth/logout", m.Handler.Logout)
	authRequired.POST("/auth/logout-all", m.Handler.LogoutAll)
}
