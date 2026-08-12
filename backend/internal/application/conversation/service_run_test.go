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
	err         error
	updateCalls int
}

func (r runCreateFailureRepository) CreateConversationRun(context.Context, *model.Run) error {
	return r.err
}

type runUpdateRepository struct {
	repository.ConversationRepository
	updateCalls int
}

func (r *runUpdateRepository) UpdateConversationRun(context.Context, *model.Run) error {
	r.updateCalls++
	return nil
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

func TestMessageSendRunStateFinalizeUpdatesCreatedRun(t *testing.T) {
	repo := &runUpdateRepository{}
	service := &Service{repo: repo, logger: zap.NewNop()}
	state := newMessageSendRunState(service, SendMessageInput{UserID: 1, ConversationID: 2}, &model.Conversation{}, time.Now(), "run")
	state.traceContext = context.Background()
	state.run.ID = 9
	state.runCreated = true

	state.finalize(context.Background(), nil)

	if repo.updateCalls != 1 {
		t.Fatalf("expected one run update, got %d", repo.updateCalls)
	}
	if state.run.Status != "success" || state.run.EndedAt == nil {
		t.Fatalf("expected terminal success state, got %#v", state.run)
	}
}
