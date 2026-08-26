package conversation

import (
	"context"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

func (s *Service) hydrateMessageFeedback(ctx context.Context, userID uint, items []model.Message) error {
	if len(items) == 0 {
		return nil
	}

	messageIDs := make([]uint, 0, len(items))
	for _, item := range items {
		if item.ID == 0 {
			continue
		}
		messageIDs = append(messageIDs, item.ID)
	}
	if len(messageIDs) == 0 {
		return nil
	}

	userFeedbackMap, err := s.repo.GetUserMessageFeedbackMap(ctx, userID, messageIDs)
	if err != nil {
		return err
	}
	countsMap, err := s.repo.GetMessageFeedbackCounts(ctx, messageIDs)
	if err != nil {
		return err
	}

	for i := range items {
		items[i].MyFeedback = userFeedbackMap[items[i].ID]
		if counts := countsMap[items[i].ID]; counts != nil {
			items[i].ThumbsUpCount = counts["up"]
			items[i].ThumbsDownCount = counts["down"]
		} else {
			items[i].ThumbsUpCount = 0
			items[i].ThumbsDownCount = 0
		}
	}
	return nil
}
