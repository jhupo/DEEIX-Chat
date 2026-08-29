package user

import "time"

const (
	AuthProviderLocal = "local"
	AuthProviderRelay = "relay"
	RoleSuperAdmin    = "superadmin"
	RoleUser          = "user"
	StatusActive      = "active"
	StatusDisabled    = "disabled"
)

type User struct {
	ID                    uint
	AuthProvider          string
	RelayConnectorID      string
	Sub2InstanceID        string
	Sub2UserID            int64
	PublicID              string
	Username              string
	DisplayName           string
	AvatarURL             string
	Email                 string
	Role                  string
	Status                string
	Timezone              string
	Locale                string
	ProfilePreferences    string
	AppearancePreferences string
	LastLoginAt           *time.Time
	PasswordHash          string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Session struct {
	ID                        uint
	SessionID                 string
	UserID                    uint
	RefreshTokenHash          string
	AccessJTI                 string
	Sub2AccessTokenEncrypted  string
	Sub2RefreshTokenEncrypted string
	Sub2AccessExpiresAt       *time.Time
	Sub2VerifiedAt            *time.Time
	ClientIP                  string
	UserAgent                 string
	DeviceName                string
	BrowserName               string
	OSName                    string
	DeviceType                string
	GeoSource                 string
	GeoAccuracy               string
	CountryCode               string
	RegionName                string
	CityName                  string
	TimezoneName              string
	IPLatitude                *float64
	IPLongitude               *float64
	PreciseLatitude           *float64
	PreciseLongitude          *float64
	PreciseAccuracyM          *float64
	PreciseLocatedAt          *time.Time
	IssuedAt                  time.Time
	LastSeenAt                *time.Time
	ExpiresAt                 time.Time
	RevokedAt                 *time.Time
	RevokeReason              string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type AuthEvent struct {
	ID         uint
	RequestID  string
	UserID     uint
	EventType  string
	Result     string
	Reason     string
	ClientIP   string
	UserAgent  string
	DetailJSON string
	OccurredAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
