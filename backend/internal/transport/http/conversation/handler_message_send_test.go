package conversation

import (
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

func TestShouldPersistStreamEvents(t *testing.T) {
	if shouldPersistStreamEvents(model.ExecutionTypeGateway) {
		t.Fatal("gateway subscriber events must not be republished")
	}
	if !shouldPersistStreamEvents(model.ExecutionTypeCloud) {
		t.Fatal("cloud stream events must be published by the HTTP handler")
	}
}
