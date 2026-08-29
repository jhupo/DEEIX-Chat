package schema

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// BootstrapCredentials is returned only when the first local control-plane
// administrator is created. The password is never persisted in plaintext.
type BootstrapCredentials struct {
	Email    string
	Username string
	Password string
}

// EnsureBootstrapSuperAdmin creates the one local control-plane administrator
// needed to configure relays before any relay account exists.
func EnsureBootstrapSuperAdmin(db *gorm.DB) (*BootstrapCredentials, error) {
	var existing model.User
	err := db.Where("auth_provider = ? AND role = ?", domainuser.AuthProviderLocal, domainuser.RoleSuperAdmin).First(&existing).Error
	if err == nil {
		return nil, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	user, credentials, err := newBootstrapSuperAdmin()
	if err != nil {
		return nil, err
	}
	if err = db.Create(&user).Error; err != nil {
		// The PostgreSQL partial unique index serializes concurrent first starts.
		// If another instance won the race, its credentials are the only valid pair.
		var winner model.User
		if lookupErr := db.Where("auth_provider = ? AND role = ?", domainuser.AuthProviderLocal, domainuser.RoleSuperAdmin).First(&winner).Error; lookupErr == nil {
			return nil, nil
		}
		return nil, err
	}
	return credentials, nil
}

func newBootstrapSuperAdmin() (model.User, *BootstrapCredentials, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return model.User{}, nil, fmt.Errorf("generate bootstrap credentials: %w", err)
	}
	seed := hex.EncodeToString(raw[:])
	email := "admin-" + seed[:12] + "@local.invalid"
	username := "admin_" + seed[:12]
	password := seed[12:] + "!A9"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, nil, fmt.Errorf("hash bootstrap password: %w", err)
	}
	return model.User{
		AuthProvider:     domainuser.AuthProviderLocal,
		RelayConnectorID: "",
		Sub2InstanceID:   "local",
		Sub2UserID:       0,
		PublicID:         "usr_" + seed[:16],
		Username:         username,
		DisplayName:      "DEEIX Control Administrator",
		Email:            email,
		Role:             domainuser.RoleSuperAdmin,
		Status:           domainuser.StatusActive,
		Timezone:         "Etc/UTC",
		Locale:           "en-US",
		PasswordHash:     string(hash),
	}, &BootstrapCredentials{Email: email, Username: username, Password: password}, nil
}

// IsLocalControlPlaneUser identifies identities that may mutate control-plane
// configuration. Relay administrators remain external identities.
func IsLocalControlPlaneUser(user model.User) bool {
	return strings.EqualFold(strings.TrimSpace(user.AuthProvider), domainuser.AuthProviderLocal) &&
		user.Role == domainuser.RoleSuperAdmin
}
