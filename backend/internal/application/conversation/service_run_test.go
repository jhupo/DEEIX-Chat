package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"go.uber.org/zap"
)

type runCreateFailureRepository struct {
	repository.ConversationRepository
	err error
}

func (r runCreateFailureRepository) CreateConversationRun(context.Context, *model.Run) error {
	return r.err
}

func TestMessageSendRunStateCreateRunReturnsRepositoryError(t *testing.T) {
	want := errors.New("create run failed")
	service := &Service{repo: runCreateFailureRepository{err: want}, logger: zap.NewNop()}
	state := newMessageSendRunState(service, SendMessageInput{UserID: 1, ConversationID: 2}, &model.Conversation{}, time.Now(), "run")
	state.traceContext = context.Background()
	if err := state.createRun(context.Background()); !errors.Is(err, want) {
		t.Fatalf("createRun() error = %v, want %v", err, want)
	}
	if state.runCreated {
		t.Fatal("createRun() marked failed run as created")
	}
}
