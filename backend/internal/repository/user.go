package repository

import (
	"context"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
)

type UpdateUserFieldsInput struct {
	AvatarURL             *string
	DisplayName           *string
	Timezone              *string
	Locale                *string
	ProfilePreferences    *string
	AppearancePreferences *string
}

func (input UpdateUserFieldsInput) IsZero() bool {
	return input.AvatarURL == nil && input.DisplayName == nil && input.Timezone == nil && input.Locale == nil && input.ProfilePreferences == nil && input.AppearancePreferences == nil
}

type UserListFilter struct {
	Query string
}

type UpdateSessionActivityInput struct {
	LastSeenAt       *time.Time
	ClientIP         *string
	UserAgent        *string
	DeviceName       *string
	BrowserName      *string
	OSName           *string
	DeviceType       *string
	GeoSource        *string
	GeoAccuracy      *string
	CountryCode      *string
	RegionName       *string
	CityName         *string
	TimezoneName     *string
	IPLatitude       **float64
	IPLongitude      **float64
	PreciseLatitude  *float64
	PreciseLongitude *float64
	PreciseAccuracyM *float64
	PreciseLocatedAt *time.Time
}

func (input UpdateSessionActivityInput) IsZero() bool {
	return input.LastSeenAt == nil && input.ClientIP == nil && input.UserAgent == nil && input.DeviceName == nil && input.BrowserName == nil && input.OSName == nil && input.DeviceType == nil && input.GeoSource == nil && input.GeoAccuracy == nil && input.CountryCode == nil && input.RegionName == nil && input.CityName == nil && input.TimezoneName == nil && input.IPLatitude == nil && input.IPLongitude == nil && input.PreciseLatitude == nil && input.PreciseLongitude == nil && input.PreciseAccuracyM == nil && input.PreciseLocatedAt == nil
}

type RotateSessionTokensInput struct {
	UserID               uint
	SessionID            string
	PresentedRefreshHash string
	NextRefreshHash      string
	NextAccessJTI        string
	IssuedAt             time.Time
	ExpiresAt            time.Time
}
type UpdateSessionSub2TokensInput struct {
	UserID                uint
	SessionID             string
	AccessTokenEncrypted  string
	RefreshTokenEncrypted string
	AccessExpiresAt       time.Time
	VerifiedAt            time.Time
}

type UserRepository interface {
	GetByID(context.Context, uint) (*domainuser.User, error)
	GetByPublicID(context.Context, string) (*domainuser.User, error)
	UpdateProfile(context.Context, uint, UpdateUserFieldsInput) (*domainuser.User, error)
	ListUsers(context.Context, int, int, UserListFilter) ([]domainuser.User, int64, error)
	ListLatestSessionActivityByUserIDs(context.Context, []uint) (map[uint]time.Time, error)
	RevokeAllSessions(context.Context, uint, string) error
	ListAuthEvents(context.Context, uint, string, string, int, int) ([]domainuser.AuthEvent, int64, error)
	RecordAuthEvent(context.Context, uint, string, string, string, string, string, string, string) error
}
