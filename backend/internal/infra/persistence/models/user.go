package model

import "time"

// User is the local projection of a Sub2 principal plus DEEIX-owned preferences.
type User struct {
	BaseModel
	AuthProvider          string     `gorm:"size:16;not null;default:'relay';index:idx_identity_users_auth_provider;comment:身份来源 local 或 relay"`
	RelayConnectorID      string     `gorm:"size:64;not null;default:'';index:idx_identity_users_relay_connector;comment:所属中转连接器公开 ID"`
	Sub2InstanceID        string     `gorm:"size:64;not null;uniqueIndex:uk_identity_users_sub2_subject,priority:1;comment:Sub2 instance fingerprint"`
	Sub2UserID            int64      `gorm:"not null;uniqueIndex:uk_identity_users_sub2_subject,priority:2;comment:Sub2 user ID"`
	PublicID              string     `gorm:"size:32;not null;uniqueIndex:idx_identity_users_public_id;comment:public user ID"`
	Username              string     `gorm:"size:64;not null;uniqueIndex:idx_identity_users_username;comment:stable local username"`
	DisplayName           string     `gorm:"size:128;not null;default:'';comment:display name"`
	AvatarURL             string     `gorm:"size:2048;not null;default:'';index:idx_identity_users_file_avatar_url,where:avatar_url LIKE 'file:%';comment:avatar URL"`
	Email                 string     `gorm:"size:128;not null;default:'';index:idx_identity_users_email;comment:Sub2 email"`
	Role                  string     `gorm:"size:32;not null;default:'user';index:idx_identity_users_role;comment:projected Sub2 role"`
	Status                string     `gorm:"size:32;not null;default:'active';index:idx_identity_users_status;comment:projected Sub2 status"`
	Timezone              string     `gorm:"size:64;not null;default:'Etc/UTC';comment:timezone"`
	Locale                string     `gorm:"size:16;not null;default:'en-US';comment:locale"`
	ProfilePreferences    string     `gorm:"type:text;not null;default:'';comment:profile preferences"`
	AppearancePreferences string     `gorm:"type:text;not null;default:'';comment:appearance preferences"`
	LastLoginAt           *time.Time `gorm:"comment:last login time"`
	PasswordHash          string     `gorm:"type:text;not null;default:'';comment:本地身份密码 bcrypt 哈希"`
}

func (User) TableName() string { return "identity_users" }

// UserSession stores DEEIX session state and encrypted Sub2 credentials.
type UserSession struct {
	BaseModel
	SessionID                 string     `gorm:"size:64;not null;uniqueIndex:idx_identity_sessions_session_id;comment:session ID"`
	UserID                    uint       `gorm:"not null;index:idx_identity_sessions_user_id;comment:user ID"`
	RefreshTokenHash          string     `gorm:"size:255;not null;comment:refresh token hash"`
	AccessJTI                 string     `gorm:"size:64;not null;index:idx_identity_sessions_access_jti;comment:access token JTI"`
	Sub2AccessTokenEncrypted  string     `gorm:"type:text;not null;comment:encrypted Sub2 access token"`
	Sub2RefreshTokenEncrypted string     `gorm:"type:text;not null;comment:encrypted Sub2 refresh token"`
	Sub2AccessExpiresAt       *time.Time `gorm:"index:idx_identity_sessions_sub2_access_expires_at;comment:Sub2 access expiry"`
	Sub2VerifiedAt            *time.Time `gorm:"index:idx_identity_sessions_sub2_verified_at;comment:last Sub2 verification"`
	ClientIP                  string     `gorm:"size:64;not null;default:'';comment:client IP"`
	UserAgent                 string     `gorm:"size:512;not null;default:'';comment:user agent"`
	DeviceName                string     `gorm:"size:128;not null;default:'';comment:device name"`
	BrowserName               string     `gorm:"size:64;not null;default:'';comment:browser name"`
	OSName                    string     `gorm:"size:64;not null;default:'';comment:operating system"`
	DeviceType                string     `gorm:"size:32;not null;default:'';comment:device type"`
	GeoSource                 string     `gorm:"size:32;not null;default:'';comment:location source"`
	GeoAccuracy               string     `gorm:"size:32;not null;default:'';comment:location accuracy"`
	CountryCode               string     `gorm:"size:32;not null;default:'';comment:country code"`
	RegionName                string     `gorm:"size:64;not null;default:'';comment:region name"`
	CityName                  string     `gorm:"size:64;not null;default:'';comment:city name"`
	TimezoneName              string     `gorm:"size:64;not null;default:'';comment:timezone name"`
	IPLatitude                *float64   `gorm:"comment:IP latitude"`
	IPLongitude               *float64   `gorm:"comment:IP longitude"`
	PreciseLatitude           *float64   `gorm:"comment:precise latitude"`
	PreciseLongitude          *float64   `gorm:"comment:precise longitude"`
	PreciseAccuracyM          *float64   `gorm:"comment:precise accuracy in meters"`
	PreciseLocatedAt          *time.Time `gorm:"comment:precise location time"`
	IssuedAt                  time.Time  `gorm:"not null;comment:issued time"`
	LastSeenAt                *time.Time `gorm:"comment:last seen time"`
	ExpiresAt                 time.Time  `gorm:"not null;index:idx_identity_sessions_expires_at;comment:expiry time"`
	RevokedAt                 *time.Time `gorm:"index:idx_identity_sessions_revoked_at;comment:revocation time"`
	RevokeReason              string     `gorm:"size:128;not null;default:'';comment:revocation reason"`
}

func (UserSession) TableName() string { return "identity_sessions" }

// UserAuthEvent stores authentication audit events.
type UserAuthEvent struct {
	BaseModel
	RequestID  string    `gorm:"size:64;not null;default:'';index:idx_identity_auth_events_request_id;comment:request ID"`
	UserID     uint      `gorm:"not null;index:idx_identity_auth_events_user_id;comment:user ID"`
	EventType  string    `gorm:"size:64;not null;index:idx_identity_auth_events_event_type;comment:event type"`
	Result     string    `gorm:"size:32;not null;index:idx_identity_auth_events_result;comment:event result"`
	Reason     string    `gorm:"size:255;not null;default:'';comment:event reason"`
	ClientIP   string    `gorm:"size:64;not null;default:'';comment:client IP"`
	UserAgent  string    `gorm:"size:512;not null;default:'';comment:user agent"`
	DetailJSON string    `gorm:"type:text;not null;default:'';comment:event details"`
	OccurredAt time.Time `gorm:"not null;index:idx_identity_auth_events_occurred_at;comment:event time"`
}

func (UserAuthEvent) TableName() string { return "identity_auth_events" }
