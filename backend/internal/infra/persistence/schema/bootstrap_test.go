package schema

import (
	"testing"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"golang.org/x/crypto/bcrypt"
)

func TestNewBootstrapSuperAdminCreatesVerifiableCredentials(t *testing.T) {
	user, credentials, err := newBootstrapSuperAdmin()
	if err != nil {
		t.Fatalf("create bootstrap administrator: %v", err)
	}
	if credentials == nil || credentials.Email == "" || credentials.Username == "" || credentials.Password == "" {
		t.Fatalf("expected generated credentials, got %#v", credentials)
	}

	if user.PasswordHash == credentials.Password {
		t.Fatal("bootstrap password must not be stored in plaintext")
	}
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password)); err != nil {
		t.Fatalf("stored password hash does not verify: %v", err)
	}
	if !IsLocalControlPlaneUser(user) {
		t.Fatalf("expected local control-plane user, got %#v", user)
	}

	if user.AuthProvider != domainuser.AuthProviderLocal || user.Role != domainuser.RoleSuperAdmin {
		t.Fatalf("unexpected bootstrap identity: %#v", user)
	}
}
