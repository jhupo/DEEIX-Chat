package auth

import (
	"context"
	"errors"
	"strings"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type contextualSub2Client interface {
	InstanceIDForContext(context.Context) string
}

func (s *Service) sub2InstanceID(ctx context.Context) string {
	if client, ok := s.sub2.(contextualSub2Client); ok {
		return client.InstanceIDForContext(ctx)
	}
	return s.sub2.InstanceID()
}

func (s *Service) changeLocalPassword(ctx context.Context, userID uint, currentPassword, newPassword string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil || user == nil || user.AuthProvider != domainuser.AuthProviderLocal || user.Role != domainuser.RoleSuperAdmin {
		return ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		return ErrInvalidCurrentPassword
	}
	if len(strings.TrimSpace(newPassword)) < 8 {
		return errors.New("invalid password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err = s.repo.UpdatePasswordHash(ctx, userID, string(hash)); err != nil {
		if errors.Is(err, repository.ErrInvalidInput) {
			return ErrInvalidCredentials
		}
		return err
	}
	return nil
}

func isLocalPrincipal(user *domainuser.User) bool {
	return user != nil && user.AuthProvider == domainuser.AuthProviderLocal && user.Role == domainuser.RoleSuperAdmin && strings.TrimSpace(user.PasswordHash) != ""
}

func isActiveLocalPrincipal(user *domainuser.User) bool {
	return isLocalPrincipal(user) && user.Status == domainuser.StatusActive
}
