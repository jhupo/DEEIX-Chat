package sub2key

import "time"

type Binding struct {
	ID              uint
	CreatedAt       time.Time
	PublicID        string
	PrincipalID     uint
	Sub2AccountID   int64
	RemoteKeyID     int64
	Ciphertext      string
	Fingerprint     string
	Label           string
	MaskedKey       string
	GroupID         *int64
	GroupName       string
	Platform        string
	Status          string
	Quota           float64
	UsedQuota       float64
	ExpiresAt       *time.Time
	Version         uint
	LastValidatedAt *time.Time
}

type BindingOperation struct {
	PrincipalID     uint
	IdempotencyKey  string
	RequestHash     string
	State           string
	BindingPublicID string
}
