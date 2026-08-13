package conversation

import (
	"context"
	"strings"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

func (s *Service) ListInteractions(ctx context.Context, userID uint, conversationPublicID, status string) ([]GatewayInteraction, error) {
	conversation, err := s.repo.GetConversationByPublicID(ctx, strings.TrimSpace(conversationPublicID), userID)
	if err != nil {
		return nil, ErrConversationNotFound
	}
	if conversation.ExecutionType != model.ExecutionTypeGateway || s.gatewayExecutor == nil {
		return []GatewayInteraction{}, nil
	}
	return s.gatewayExecutor.ListInteractions(ctx, userID, conversation.ID, strings.TrimSpace(status))
}

func (s *Service) RespondInteraction(ctx context.Context, userID uint, input GatewayInteractionResponse) (*GatewayInteraction, error) {
	if s.gatewayExecutor == nil || userID == 0 {
		return nil, ErrExecutionUnavailable
	}
	return s.gatewayExecutor.RespondInteraction(ctx, userID, input)
}
