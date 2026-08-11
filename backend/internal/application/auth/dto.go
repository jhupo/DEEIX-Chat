package auth

import "time"

// LoginResult 登录成功后的内部传输结构，不携带序列化标记。
type LoginResult struct {
	AccessToken             string
	RefreshToken            string
	SessionID               string
	ExpiresAt               *time.Time
	RefreshExpiresAt        *time.Time
	TwoFactorRequired       bool
	TwoFactorChallengeToken string
}

// MeResult 当前用户信息内部传输结构，不携带序列化标记。
// LogoutResult 登出结果内部传输结构，不携带序列化标记。
type LogoutResult struct {
	Revoked bool
}

// ActiveSessionResult 活跃会话内部传输结构，不携带序列化标记。
type ActiveSessionResult struct {
	SessionID        string
	Current          bool
	DeviceLabel      string
	DeviceName       string
	BrowserName      string
	OSName           string
	DeviceType       string
	ClientIP         string
	LocationLabel    string
	GeoSource        string
	GeoAccuracy      string
	CountryCode      string
	RegionName       string
	CityName         string
	TimezoneName     string
	IPLatitude       *float64
	IPLongitude      *float64
	PreciseLatitude  *float64
	PreciseLongitude *float64
	PreciseAccuracyM *float64
	PreciseLocatedAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastSeenAt       *time.Time
	ExpiresAt        time.Time
}

// ActiveSessionListResult 活跃会话列表内部传输结构，不携带序列化标记。
type ActiveSessionListResult struct {
	Total   int64
	Results []ActiveSessionResult
}
