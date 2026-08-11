package repository

import (
	"context"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/sub2key"
)

type Sub2KeyBindingRepository interface {
	ListSub2KeyBindings(context.Context, uint) ([]sub2key.Binding, error)
	GetSub2KeyBinding(context.Context, uint, string) (*sub2key.Binding, error)
	GetSub2KeyBindingByRemoteKeyID(context.Context, uint, int64) (*sub2key.Binding, error)
	UpsertSub2KeyBinding(context.Context, *sub2key.Binding) error
	MarkSub2KeyBindingUnavailable(context.Context, uint, int64, time.Time) error
	RevokeSub2KeyBinding(context.Context, uint, string) error
	GetSub2KeyBindingOperation(context.Context, uint, string) (*sub2key.BindingOperation, error)
	CreateSub2KeyBindingOperation(context.Context, *sub2key.BindingOperation) (bool, error)
	CompleteSub2KeyBindingOperation(context.Context, uint, string, string) error
}
