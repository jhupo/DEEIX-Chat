package auth

import (
	"testing"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
)

func TestLocalPrincipalRequiresControlPlaneIdentity(t *testing.T) {
	base := &domainuser.User{
		AuthProvider: domainuser.AuthProviderLocal,
		Role:         domainuser.RoleSuperAdmin,
		Status:       domainuser.StatusActive,
		PasswordHash: "bcrypt-hash",
	}
	if !isLocalPrincipal(base) || !isActiveLocalPrincipal(base) {
		t.Fatal("expected active local superadmin to be recognized")
	}

	for name, user := range map[string]*domainuser.User{
		"relay identity":         {AuthProvider: domainuser.AuthProviderRelay, Role: domainuser.RoleSuperAdmin, PasswordHash: "bcrypt-hash"},
		"local non-admin":        {AuthProvider: domainuser.AuthProviderLocal, Role: domainuser.RoleUser, PasswordHash: "bcrypt-hash"},
		"local without password": {AuthProvider: domainuser.AuthProviderLocal, Role: domainuser.RoleSuperAdmin},
		"disabled local admin":   {AuthProvider: domainuser.AuthProviderLocal, Role: domainuser.RoleSuperAdmin, Status: domainuser.StatusDisabled, PasswordHash: "bcrypt-hash"},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "disabled local admin" {
				if !isLocalPrincipal(user) || isActiveLocalPrincipal(user) {
					t.Fatal("disabled local admin should not be active")
				}
				return
			}
			if isLocalPrincipal(user) {
				t.Fatalf("identity incorrectly recognized as local principal: %#v", user)
			}
		})
	}
}
