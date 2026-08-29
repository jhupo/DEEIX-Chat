package auth

import (
	"time"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/userview"
)

type LoginRequest struct {
	Email          string `json:"email" binding:"required,max=128,email"`
	Password       string `json:"password" binding:"required,min=6,max=128"`
	TurnstileToken string `json:"turnstileToken,omitempty" binding:"omitempty,max=2048"`
}

type TwoFactorVerifyRequest struct {
	ChallengeToken string `json:"challengeToken" binding:"required,min=20,max=4096"`
	Code           string `json:"code" binding:"required,min=6,max=32"`
}

type EmailRegistrationStartRequest struct {
	Email          string `json:"email" binding:"required,max=128,email"`
	TurnstileToken string `json:"turnstileToken,omitempty" binding:"omitempty,max=2048"`
}

type EmailRegistrationCompleteRequest struct {
	Email          string `json:"email" binding:"required,max=128,email"`
	Password       string `json:"password" binding:"required,min=8,max=128"`
	Code           string `json:"code,omitempty" binding:"omitempty,len=6"`
	TurnstileToken string `json:"turnstileToken,omitempty" binding:"omitempty,max=2048"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required,max=128"`
	NewPassword     string `json:"newPassword" binding:"required,min=8,max=128"`
}

type PatchMeRequest struct {
	AvatarURL             *string `json:"avatarURL,omitempty" binding:"omitempty,max=2048"`
	DisplayName           *string `json:"displayName,omitempty" binding:"omitempty,max=128"`
	Timezone              *string `json:"timezone,omitempty" binding:"omitempty,max=64"`
	Locale                *string `json:"locale,omitempty" binding:"omitempty,max=32"`
	ProfilePreferences    *string `json:"profilePreferences,omitempty" binding:"omitempty,max=65536"`
	AppearancePreferences *string `json:"appearancePreferences,omitempty" binding:"omitempty,max=65536"`
}

type UpdateCurrentSessionLocationRequest struct {
	Latitude       float64  `json:"latitude" binding:"required,min=-90,max=90"`
	Longitude      float64  `json:"longitude" binding:"required,min=-180,max=180"`
	AccuracyMeters *float64 `json:"accuracyMeters,omitempty" binding:"omitempty,min=0,max=100000"`
	Timezone       string   `json:"timezone,omitempty" binding:"omitempty,max=64"`
}

type UserResponse struct {
	ID                    uint       `json:"id"`
	AuthProvider          string     `json:"authProvider"`
	PublicID              string     `json:"publicID"`
	Username              string     `json:"username"`
	DisplayName           string     `json:"displayName"`
	AvatarURL             string     `json:"avatarURL"`
	Email                 string     `json:"email"`
	Role                  string     `json:"role"`
	Status                string     `json:"status"`
	Timezone              string     `json:"timezone"`
	Locale                string     `json:"locale"`
	ProfilePreferences    string     `json:"profilePreferences"`
	AppearancePreferences string     `json:"appearancePreferences"`
	LastLoginAt           *time.Time `json:"lastLoginAt" extensions:"x-nullable,!x-omitempty"`
	LastActiveAt          *time.Time `json:"lastActiveAt" extensions:"x-nullable,!x-omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

type LoginResponse struct {
	AccessToken             string     `json:"accessToken,omitempty"`
	SessionID               string     `json:"sessionID,omitempty"`
	ExpiresAt               *time.Time `json:"expiresAt,omitempty" extensions:"x-nullable"`
	RefreshExpiresAt        *time.Time `json:"refreshExpiresAt,omitempty" extensions:"x-nullable"`
	TwoFactorRequired       bool       `json:"twoFactorRequired"`
	TwoFactorChallengeToken string     `json:"twoFactorChallengeToken,omitempty"`
}

type LoginOptionsResponse struct {
	EmailRegistrationEnabled bool   `json:"emailRegistrationEnabled"`
	EmailVerificationEnabled bool   `json:"emailVerificationEnabled"`
	TurnstileEnabled         bool   `json:"turnstileEnabled"`
	TurnstileSiteKey         string `json:"turnstileSiteKey"`
}

type MeResponse struct {
	User UserResponse `json:"user"`
}

type LogoutResponse struct {
	Revoked bool `json:"revoked"`
}

type ChangePasswordResponse struct {
	Changed bool `json:"changed"`
}

type EmailRegistrationStartResponse struct {
	Sent      bool      `json:"sent"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ActiveSessionResponse struct {
	SessionID        string     `json:"sessionID"`
	Current          bool       `json:"current"`
	DeviceLabel      string     `json:"deviceLabel"`
	DeviceName       string     `json:"deviceName"`
	BrowserName      string     `json:"browserName"`
	OSName           string     `json:"osName"`
	DeviceType       string     `json:"deviceType"`
	ClientIP         string     `json:"clientIP"`
	LocationLabel    string     `json:"locationLabel"`
	GeoSource        string     `json:"geoSource"`
	GeoAccuracy      string     `json:"geoAccuracy"`
	CountryCode      string     `json:"countryCode"`
	RegionName       string     `json:"regionName"`
	CityName         string     `json:"cityName"`
	TimezoneName     string     `json:"timezoneName"`
	IPLatitude       *float64   `json:"ipLatitude" extensions:"x-nullable,!x-omitempty"`
	IPLongitude      *float64   `json:"ipLongitude" extensions:"x-nullable,!x-omitempty"`
	PreciseLatitude  *float64   `json:"preciseLatitude" extensions:"x-nullable,!x-omitempty"`
	PreciseLongitude *float64   `json:"preciseLongitude" extensions:"x-nullable,!x-omitempty"`
	PreciseAccuracyM *float64   `json:"preciseAccuracyMeters" extensions:"x-nullable,!x-omitempty"`
	PreciseLocatedAt *time.Time `json:"preciseLocatedAt" extensions:"x-nullable,!x-omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	LastSeenAt       *time.Time `json:"lastSeenAt" extensions:"x-nullable,!x-omitempty"`
	ExpiresAt        time.Time  `json:"expiresAt"`
}

type ActiveSessionListResponse struct {
	Total   int64                   `json:"total"`
	Results []ActiveSessionResponse `json:"results"`
}

// ErrorDoc documents the standard API error envelope.
type ErrorDoc struct {
	ErrorMsg  string      `json:"errorMsg" example:"invalid request"`
	ErrorCode string      `json:"errorCode,omitempty" example:"invalid_request"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"requestId,omitempty"`
	Data      interface{} `json:"data"`
}

// LoginResponseDoc documents login, login 2FA, registration completion, and refresh responses.
type LoginResponseDoc struct {
	ErrorMsg string        `json:"errorMsg"`
	Data     LoginResponse `json:"data"`
}

// LoginOptionsResponseDoc documents Sub2 login capability settings.
type LoginOptionsResponseDoc struct {
	ErrorMsg string               `json:"errorMsg"`
	Data     LoginOptionsResponse `json:"data"`
}

// EmailRegistrationStartResponseDoc documents an email registration code request.
type EmailRegistrationStartResponseDoc struct {
	ErrorMsg string                         `json:"errorMsg"`
	Data     EmailRegistrationStartResponse `json:"data"`
}

// MeResponseDoc documents current principal and preference responses.
type MeResponseDoc struct {
	ErrorMsg string     `json:"errorMsg"`
	Data     MeResponse `json:"data"`
}

// ChangePasswordResponseDoc documents a successful Sub2 password change.
type ChangePasswordResponseDoc struct {
	ErrorMsg string                 `json:"errorMsg"`
	Data     ChangePasswordResponse `json:"data"`
}

// LogoutResponseDoc documents session revocation responses.
type LogoutResponseDoc struct {
	ErrorMsg string         `json:"errorMsg"`
	Data     LogoutResponse `json:"data"`
}

// ActiveSessionListResponseDoc documents active DEEIX browser sessions.
type ActiveSessionListResponseDoc struct {
	ErrorMsg string                    `json:"errorMsg"`
	Data     ActiveSessionListResponse `json:"data"`
}

// ActiveSessionResponseDoc documents the current session location update response.
type ActiveSessionResponseDoc struct {
	ErrorMsg string                `json:"errorMsg"`
	Data     ActiveSessionResponse `json:"data"`
}

func toUserResponse(v userview.UserView) UserResponse {
	return UserResponse{
		ID: v.ID, AuthProvider: v.AuthProvider, PublicID: v.PublicID, Username: v.Username, DisplayName: v.DisplayName,
		AvatarURL: v.AvatarURL, Email: v.Email, Role: v.Role, Status: v.Status,
		Timezone: v.Timezone, Locale: v.Locale, ProfilePreferences: v.ProfilePreferences,
		AppearancePreferences: v.AppearancePreferences, LastLoginAt: v.LastLoginAt,
		LastActiveAt: v.LastActiveAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toLoginResponse(v *appauth.LoginResult) LoginResponse {
	return LoginResponse{
		AccessToken: v.AccessToken, SessionID: v.SessionID, ExpiresAt: v.ExpiresAt,
		RefreshExpiresAt:  v.RefreshExpiresAt,
		TwoFactorRequired: v.TwoFactorRequired, TwoFactorChallengeToken: v.TwoFactorChallengeToken,
	}
}

func toLoginOptionsResponse(v *appauth.LoginOptions) LoginOptionsResponse {
	return LoginOptionsResponse{
		EmailRegistrationEnabled: v.EmailRegistrationEnabled,
		EmailVerificationEnabled: v.EmailVerificationEnabled,
		TurnstileEnabled:         v.TurnstileEnabled,
		TurnstileSiteKey:         v.TurnstileSiteKey,
	}
}

func toActiveSessionResponse(v appauth.ActiveSessionResult) ActiveSessionResponse {
	return ActiveSessionResponse{
		SessionID: v.SessionID, Current: v.Current, DeviceLabel: v.DeviceLabel,
		DeviceName: v.DeviceName, BrowserName: v.BrowserName, OSName: v.OSName,
		DeviceType: v.DeviceType, ClientIP: v.ClientIP, LocationLabel: v.LocationLabel,
		GeoSource: v.GeoSource, GeoAccuracy: v.GeoAccuracy, CountryCode: v.CountryCode,
		RegionName: v.RegionName, CityName: v.CityName, TimezoneName: v.TimezoneName,
		IPLatitude: v.IPLatitude, IPLongitude: v.IPLongitude,
		PreciseLatitude: v.PreciseLatitude, PreciseLongitude: v.PreciseLongitude,
		PreciseAccuracyM: v.PreciseAccuracyM, PreciseLocatedAt: v.PreciseLocatedAt,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, LastSeenAt: v.LastSeenAt,
		ExpiresAt: v.ExpiresAt,
	}
}
