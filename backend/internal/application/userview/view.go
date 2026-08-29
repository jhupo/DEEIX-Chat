package userview

import (
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
)

type UserView struct {
	ID                    uint
	AuthProvider          string
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
	LastActiveAt          *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func FromUser(item domainuser.User) UserView {
	return UserView{
		ID: item.ID, AuthProvider: item.AuthProvider, PublicID: item.PublicID, Username: item.Username, DisplayName: item.DisplayName,
		AvatarURL: item.AvatarURL, Email: item.Email, Role: item.Role, Status: item.Status,
		Timezone: item.Timezone, Locale: item.Locale, ProfilePreferences: item.ProfilePreferences,
		AppearancePreferences: item.AppearancePreferences,
		LastLoginAt:           item.LastLoginAt, LastActiveAt: item.LastLoginAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func WithLastActiveAt(view UserView, value *time.Time) UserView {
	if value != nil {
		view.LastActiveAt = value
	}
	return view
}
