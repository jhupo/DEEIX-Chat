package model

import "time"

// Sub2KeyBinding stores only a server-side encrypted credential selected by a
// principal. Browser input is limited to the remote key identifier.
type Sub2KeyBinding struct {
	BaseModel
	PublicID        string     `gorm:"size:64;not null;uniqueIndex:uk_sub2_key_bindings_public_id"`
	PrincipalID     uint       `gorm:"not null;index:idx_sub2_key_bindings_principal;uniqueIndex:uk_sub2_key_bindings_principal_remote,priority:1"`
	Sub2AccountID   int64      `gorm:"not null"`
	RemoteKeyID     int64      `gorm:"not null;uniqueIndex:uk_sub2_key_bindings_principal_remote,priority:2"`
	Ciphertext      string     `gorm:"type:text;not null;default:''"`
	Fingerprint     string     `gorm:"size:64;not null;default:'';uniqueIndex:uk_sub2_key_bindings_active_fingerprint,where:deleted_at IS NULL"`
	Label           string     `gorm:"size:255;not null;default:''"`
	MaskedKey       string     `gorm:"size:255;not null;default:''"`
	GroupID         *int64     `gorm:"index"`
	GroupName       string     `gorm:"size:255;not null;default:''"`
	Platform        string     `gorm:"size:64;not null;default:''"`
	Status          string     `gorm:"size:32;not null;default:'';index"`
	Quota           float64    `gorm:"not null;default:0"`
	UsedQuota       float64    `gorm:"not null;default:0"`
	ExpiresAt       *time.Time `gorm:"index"`
	Version         uint       `gorm:"not null;default:1"`
	LastValidatedAt *time.Time `gorm:"index"`
	DeletedAt       *time.Time `gorm:"index"`
}

func (Sub2KeyBinding) TableName() string { return "sub2_key_bindings" }

// Sub2KeyBindingOperation makes a bind request replayable without retaining a
// remote key or its encrypted representation outside the binding itself.
type Sub2KeyBindingOperation struct {
	BaseModel
	PrincipalID     uint   `gorm:"not null;uniqueIndex:uk_sub2_key_binding_operations_principal_key,priority:1"`
	IdempotencyKey  string `gorm:"size:36;not null;uniqueIndex:uk_sub2_key_binding_operations_principal_key,priority:2"`
	RequestHash     string `gorm:"size:64;not null"`
	State           string `gorm:"size:32;not null;index"`
	BindingPublicID string `gorm:"size:64;not null;default:''"`
}

func (Sub2KeyBindingOperation) TableName() string { return "sub2_key_binding_operations" }

// Sub2PaymentOperation is an idempotency journal. It deliberately contains no
// financial state, balance, or provider payload.
type Sub2PaymentOperation struct {
	BaseModel
	PrincipalID    uint   `gorm:"not null;uniqueIndex:uk_sub2_payment_operations_principal_key,priority:1"`
	IdempotencyKey string `gorm:"size:128;not null;uniqueIndex:uk_sub2_payment_operations_principal_key,priority:2"`
	RequestHash    string `gorm:"size:64;not null"`
	State          string `gorm:"size:32;not null;index"`
	RemoteOrderID  string `gorm:"size:128;not null;default:''"`
}

func (Sub2PaymentOperation) TableName() string { return "sub2_payment_operations" }
