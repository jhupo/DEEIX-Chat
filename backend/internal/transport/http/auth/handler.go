package auth

import (
	"context"
	"errors"
	"net/http"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const refreshTokenCookieName = "deeix_chat_refresh_token"

type Handler struct {
	service *appauth.Service
	refresh func(context.Context, string, string, requestmeta.SessionAuditContext) (*appauth.LoginResult, error)
}

func NewHandler(s *appauth.Service) *Handler { return &Handler{service: s, refresh: s.Refresh} }
func (h *Handler) cookie(c *gin.Context, r *appauth.LoginResult) {
	if r == nil || r.RefreshToken == "" || r.RefreshExpiresAt == nil {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    r.RefreshToken,
		Path:     "/api/v1/auth",
		Expires:  *r.RefreshExpiresAt,
		HttpOnly: true,
		Secure:   h.service.ShouldUseSecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}
func (h *Handler) clear(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: refreshTokenCookieName, Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true})
}

// LoginOptions godoc
// @Summary Get Sub2 login options
// @Description Returns current Sub2 email registration and Turnstile settings.
// @Tags auth
// @Produce json
// @Success 200 {object} LoginOptionsResponseDoc
// @Failure 502 {object} ErrorDoc
// @Router /auth/login-options [get]
func (h *Handler) LoginOptions(c *gin.Context) {
	v, e := h.service.GetLoginOptions(c)
	if e != nil {
		response.Error(c, http.StatusBadGateway, appauth.ErrSub2Unavailable.Error())
		return
	}
	response.Success(c, toLoginOptionsResponse(v))
}

// StartEmailRegistration godoc
// @Summary Start Sub2 email registration
// @Description Requests a Sub2 registration code for an email address.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body EmailRegistrationStartRequest true "Registration request"
// @Success 200 {object} EmailRegistrationStartResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /auth/register/email/start [post]
func (h *Handler) StartEmailRegistration(c *gin.Context) {
	var r EmailRegistrationStartRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	v, e := h.service.RequestEmailRegistration(c, r.Email, r.TurnstileToken)
	if e != nil {
		if errors.Is(e, appauth.ErrSub2Unavailable) {
			response.Error(c, http.StatusBadGateway, e.Error())
		} else {
			response.ErrorFrom(c, http.StatusBadRequest, e)
		}
		return
	}
	response.Success(c, EmailRegistrationStartResponse{Sent: v.Sent, ExpiresAt: v.ExpiresAt})
}

// CompleteEmailRegistration godoc
// @Summary Complete Sub2 email registration
// @Description Registers with Sub2 and creates a DEEIX browser session with a refresh cookie.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body EmailRegistrationCompleteRequest true "Registration completion"
// @Success 200 {object} LoginResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /auth/register/email/complete [post]
func (h *Handler) CompleteEmailRegistration(c *gin.Context) {
	var r EmailRegistrationCompleteRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	v, e := h.service.RegisterWithEmail(c, r.Email, r.Password, r.Code, r.TurnstileToken, middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c))
	if e != nil {
		if errors.Is(e, appauth.ErrSub2Unavailable) {
			response.Error(c, http.StatusBadGateway, e.Error())
		} else {
			response.ErrorFrom(c, http.StatusBadRequest, e)
		}
		return
	}
	h.cookie(c, v)
	response.Success(c, toLoginResponse(v))
}

// Login godoc
// @Summary Log in with Sub2
// @Description Authenticates an email/password with Sub2 and sets the DEEIX refresh cookie on success.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "Login request"
// @Success 200 {object} LoginResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 401 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var r LoginRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	v, e := h.service.Login(c, r.Email, r.Password, r.TurnstileToken, middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c))
	if e != nil {
		if errors.Is(e, appauth.ErrSub2Unavailable) {
			response.Error(c, http.StatusBadGateway, e.Error())
		} else if errors.Is(e, appauth.ErrHumanVerificationFailed) {
			response.ErrorFrom(c, http.StatusBadRequest, e)
		} else {
			response.Error(c, http.StatusUnauthorized, "invalid email or password")
		}
		return
	}
	h.cookie(c, v)
	response.Success(c, toLoginResponse(v))
}

// VerifyTwoFactorLogin godoc
// @Summary Complete Sub2 login 2FA
// @Description Verifies the encrypted Sub2 login challenge and sets the DEEIX refresh cookie on success.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body TwoFactorVerifyRequest true "Sub2 login 2FA request"
// @Success 200 {object} LoginResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 401 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /auth/login/2fa [post]
func (h *Handler) VerifyTwoFactorLogin(c *gin.Context) {
	var r TwoFactorVerifyRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	v, e := h.service.VerifyLoginTwoFactor(c, r.ChallengeToken, r.Code, middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c))
	if e != nil {
		if errors.Is(e, appauth.ErrSub2Unavailable) {
			response.Error(c, http.StatusBadGateway, e.Error())
		} else {
			response.Error(c, http.StatusUnauthorized, "invalid two factor code")
		}
		return
	}
	h.cookie(c, v)
	response.Success(c, toLoginResponse(v))
}

// RefreshToken godoc
// @Summary Refresh the browser session
// @Description Rotates the DEEIX session using only the HttpOnly refresh cookie and returns no refresh token in JSON.
// @Tags auth
// @Produce json
// @Success 200 {object} LoginResponseDoc
// @Failure 401 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /auth/refresh [post]
func (h *Handler) RefreshToken(c *gin.Context) {
	raw, _ := c.Cookie(refreshTokenCookieName)
	v, e := h.refresh(c, raw, middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c))
	if e != nil {
		if errors.Is(e, appauth.ErrInvalidRefreshToken) || errors.Is(e, appauth.ErrSessionRevoked) || errors.Is(e, appauth.ErrInvalidCredentials) {
			h.clear(c)
			response.Error(c, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		if errors.Is(e, appauth.ErrSub2Unavailable) {
			response.Error(c, http.StatusBadGateway, "identity service unavailable")
			return
		}
		response.Error(c, http.StatusInternalServerError, "refresh failed")
		return
	}
	h.cookie(c, v)
	response.Success(c, toLoginResponse(v))
}

// Me godoc
// @Summary Get the current Sub2 principal projection
// @Description Returns the revalidated Sub2 principal and DEEIX-owned preferences.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} MeResponseDoc
// @Failure 401 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /me [get]
func (h *Handler) Me(c *gin.Context) {
	v, e := h.service.GetVerifiedProfile(c, middleware.MustUserID(c), middleware.MustSessionID(c))
	if e != nil {
		if errors.Is(e, appauth.ErrInvalidRefreshToken) || errors.Is(e, appauth.ErrSessionRevoked) || errors.Is(e, appauth.ErrInvalidCredentials) {
			response.Error(c, http.StatusUnauthorized, "unauthorized")
			return
		}
		if errors.Is(e, appauth.ErrSub2Unavailable) {
			response.Error(c, http.StatusBadGateway, "identity service unavailable")
			return
		}
		response.Error(c, http.StatusInternalServerError, "profile failed")
		return
	}
	u, e := h.service.BuildUserView(c, *v)
	if e != nil {
		response.Error(c, 500, "profile failed")
		return
	}
	response.Success(c, MeResponse{User: toUserResponse(u)})
}

// PatchMe godoc
// @Summary Update local profile preferences
// @Description Updates DEEIX-owned display, avatar, locale, timezone, and preference fields.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body PatchMeRequest true "Profile update"
// @Success 200 {object} MeResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 401 {object} ErrorDoc
// @Router /me [patch]
func (h *Handler) PatchMe(c *gin.Context) {
	var r PatchMeRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	v, e := h.service.UpdateProfile(c, middleware.MustUserID(c), appauth.UpdateProfileInput{AvatarURL: r.AvatarURL, DisplayName: r.DisplayName, Timezone: r.Timezone, Locale: r.Locale, ProfilePreferences: r.ProfilePreferences, AppearancePreferences: r.AppearancePreferences})
	if e != nil {
		response.ErrorFrom(c, 400, e)
		return
	}
	u, e := h.service.BuildUserView(c, *v)
	if e != nil {
		response.Error(c, http.StatusInternalServerError, "profile failed")
		return
	}
	response.Success(c, MeResponse{User: toUserResponse(u)})
}

// ChangePassword godoc
// @Summary Change the Sub2 password
// @Description Proxies the password change to Sub2, then revokes DEEIX browser sessions.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ChangePasswordRequest true "Password change"
// @Success 200 {object} ChangePasswordResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 401 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /me/password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	var r ChangePasswordRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	e := h.service.ChangePassword(
		c,
		middleware.MustUserID(c),
		middleware.MustSessionID(c),
		r.CurrentPassword,
		r.NewPassword,
		middleware.MustRequestID(c),
		middleware.ResolveSessionAuditContext(c),
	)
	if e != nil {
		switch {
		case errors.Is(e, appauth.ErrInvalidCurrentPassword):
			response.ErrorFrom(c, http.StatusBadRequest, e)
		case errors.Is(e, appauth.ErrSessionRevoked), errors.Is(e, appauth.ErrInvalidRefreshToken), errors.Is(e, appauth.ErrInvalidCredentials):
			response.Error(c, http.StatusUnauthorized, "unauthorized")
		case errors.Is(e, appauth.ErrSub2Unavailable):
			response.Error(c, http.StatusBadGateway, e.Error())
		default:
			response.Error(c, http.StatusInternalServerError, "password change failed")
		}
		return
	}
	h.clear(c)
	response.Success(c, ChangePasswordResponse{Changed: true})
}

// CurrentSessions godoc
// @Summary List active browser sessions
// @Description Lists active DEEIX sessions for the current Sub2 principal.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ActiveSessionListResponseDoc
// @Failure 401 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /auth/sessions [get]
func (h *Handler) CurrentSessions(c *gin.Context) {
	v, e := h.service.ListCurrentActiveSessions(c, middleware.MustUserID(c), middleware.MustSessionID(c))
	if e != nil {
		response.Error(c, 500, "sessions failed")
		return
	}
	out := make([]ActiveSessionResponse, 0, len(v))
	for _, x := range v {
		out = append(out, toActiveSessionResponse(x))
	}
	response.Success(c, ActiveSessionListResponse{Total: int64(len(out)), Results: out})
}

// UpdateCurrentSessionLocation godoc
// @Summary Update current session location
// @Description Updates DEEIX session metadata for the authenticated browser session.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body UpdateCurrentSessionLocationRequest true "Current session location"
// @Success 200 {object} ActiveSessionResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 401 {object} ErrorDoc
// @Router /auth/sessions/current/location [put]
func (h *Handler) UpdateCurrentSessionLocation(c *gin.Context) {
	var r UpdateCurrentSessionLocationRequest
	if c.ShouldBindJSON(&r) != nil {
		response.InvalidRequestBody(c, errors.New("invalid request"))
		return
	}
	v, e := h.service.UpdateCurrentSessionLocation(c, middleware.MustUserID(c), middleware.MustSessionID(c), middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c), appauth.UpdateCurrentSessionLocationInput{Latitude: r.Latitude, Longitude: r.Longitude, AccuracyMeters: r.AccuracyMeters, Timezone: r.Timezone})
	if e != nil {
		response.ErrorFrom(c, 400, e)
		return
	}
	response.Success(c, toActiveSessionResponse(*v))
}

// LogoutSession godoc
// @Summary Revoke a browser session
// @Description Revokes the specified DEEIX browser session and clears its stored Sub2 credentials.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Param session_id path string true "Session ID"
// @Success 200 {object} LogoutResponseDoc
// @Failure 401 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /auth/sessions/{session_id}/logout [post]
func (h *Handler) LogoutSession(c *gin.Context) {
	e := h.service.Logout(c, middleware.MustUserID(c), c.Param("session_id"), middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c))
	if e != nil {
		response.Error(c, 500, "logout failed")
		return
	}
	response.Success(c, LogoutResponse{Revoked: true})
}

// Logout godoc
// @Summary Log out the current browser session
// @Description Revokes the current DEEIX session and clears its refresh cookie.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} LogoutResponseDoc
// @Failure 401 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	e := h.service.Logout(c, middleware.MustUserID(c), middleware.MustSessionID(c), middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c))
	if e != nil {
		response.Error(c, http.StatusInternalServerError, "logout failed")
		return
	}
	h.clear(c)
	response.Success(c, LogoutResponse{Revoked: true})
}

// LogoutAll godoc
// @Summary Log out all browser sessions
// @Description Revokes all DEEIX sessions for the current Sub2 principal and clears the refresh cookie.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} LogoutResponseDoc
// @Failure 401 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /auth/logout-all [post]
func (h *Handler) LogoutAll(c *gin.Context) {
	if h.service.LogoutAll(c, middleware.MustUserID(c), middleware.MustRequestID(c), middleware.ResolveSessionAuditContext(c)) != nil {
		response.Error(c, 500, "logout failed")
		return
	}
	h.clear(c)
	response.Success(c, LogoutResponse{Revoked: true})
}
