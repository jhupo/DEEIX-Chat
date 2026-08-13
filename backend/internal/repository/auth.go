package repository

import (
	"context"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
)

type AuthRepository interface {
	GetByID(context.Context, uint) (*domainuser.User, error)
	GetByPublicID(context.Context, string) (*domainuser.User, error)
	UpsertSub2Principal(context.Context, *domainuser.User) (*domainuser.User, error)
	UpdateProfile(context.Context, uint, UpdateUserFieldsInput) (*domainuser.User, error)
	UpdateLastLogin(context.Context, uint) error
	RecordAuthEvent(context.Context, uint, string, string, string, string, string, string, string) error
	CreateSession(context.Context, *domainuser.Session) error
	GetSessionByUserAndSessionID(context.Context, uint, string) (*domainuser.Session, error)
	RotateSessionTokens(context.Context, RotateSessionTokensInput) error
	StageSessionSub2Tokens(context.Context, UpdateSessionSub2TokensInput) error
	UpdateSessionSub2Tokens(context.Context, UpdateSessionSub2TokensInput) error
	TouchSessionActivity(context.Context, uint, string, UpdateSessionActivityInput) error
	RevokeSession(context.Context, uint, string, string) error
	RevokeAllSessions(context.Context, uint, string) error
	ListActiveSessionsByUserID(context.Context, uint, time.Time) ([]domainuser.Session, error)
}
