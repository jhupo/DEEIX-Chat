package conversation

import (
	"context"
	"strings"

	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repo) GetConversationExecutionByRunID(ctx context.Context, userID uint, runID string) (*domainconversation.Conversation, error) {
	if userID == 0 || strings.TrimSpace(runID) == "" {
		return nil, repository.ErrInvalidInput
	}
	var row model.Conversation
	err := r.db.WithContext(ctx).Table("chat_conversations AS conversations").
		Select("conversations.*").
		Joins("JOIN chat_runs AS runs ON runs.conversation_id = conversations.id").
		Where("runs.user_id = ? AND runs.run_id = ?", userID, strings.TrimSpace(runID)).First(&row).Error
	if err != nil {
		return nil, translateError(err)
	}
	result := toConversationDomain(row)
	return &result, nil
}

func (r *Repo) ProjectExecutionEvent(ctx context.Context, item *domainconversation.ExecutionEvent) (bool, error) {
	if item == nil || item.ConversationID == 0 || item.UserID == 0 || strings.TrimSpace(item.RunID) == "" ||
		strings.TrimSpace(item.SourceKey) == "" || strings.TrimSpace(item.Kind) == "" || item.OccurredAt.IsZero() {
		return false, repository.ErrInvalidInput
	}
	applied := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation model.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", item.ConversationID, item.UserID).
			First(&conversation).Error; err != nil {
			return err
		}
		var messageCount int64
		if err := tx.Model(&model.Message{}).
			Where("conversation_id = ? AND user_id = ? AND run_id = ?", item.ConversationID, item.UserID, item.RunID).
			Count(&messageCount).Error; err != nil {
			return err
		}
		if messageCount == 0 && item.TerminalStatus == "" {
			return repository.ErrConflict
		}
		var run model.ConversationRun
		if err := tx.Select("status", "error_code").
			Where("conversation_id = ? AND user_id = ? AND run_id = ?", item.ConversationID, item.UserID, item.RunID).
			First(&run).Error; err != nil {
			return err
		}
		if run.Status == "success" ||
			((run.Status == "interrupted" || run.Status == "error") &&
				(item.TerminalStatus == "" || run.ErrorCode != "stream_interrupted")) {
			return nil
		}

		var existing int64
		if err := tx.Model(&model.ConversationExecutionEvent{}).Where("source_key = ?", item.SourceKey).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			return nil
		}

		item.Seq = conversation.ExecutionEventSeq + 1
		row := model.ConversationExecutionEvent{
			ConversationID: item.ConversationID, UserID: item.UserID, RunID: item.RunID,
			SourceKey: item.SourceKey, Seq: item.Seq, Kind: item.Kind,
			PayloadJSON: item.PayloadJSON, OccurredAt: item.OccurredAt,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if conversation.ExecutionType == domainconversation.ExecutionTypeGateway && item.TextDelta != "" {
			if err := appendGatewayMessageDelta(tx, item, "content", item.TextDelta); err != nil {
				return err
			}
		}
		if conversation.ExecutionType == domainconversation.ExecutionTypeGateway && item.ReasoningDelta != "" {
			if err := appendGatewayMessageDelta(tx, item, "reasoning_content", item.ReasoningDelta); err != nil {
				return err
			}
		}
		if conversation.ExecutionType == domainconversation.ExecutionTypeGateway && item.Kind == "turn/started" {
			if err := startGatewayTurn(tx, item); err != nil {
				return err
			}
		}
		if conversation.ExecutionType == domainconversation.ExecutionTypeGateway && item.TerminalStatus != "" {
			if err := completeGatewayTurn(tx, item); err != nil {
				return err
			}
		}
		if err := tx.Model(&conversation).Update("execution_event_seq", item.Seq).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, translateError(err)
}

func startGatewayTurn(tx *gorm.DB, item *domainconversation.ExecutionEvent) error {
	run := tx.Model(&model.ConversationRun{}).
		Where("conversation_id = ? AND user_id = ? AND run_id = ? AND status = ?", item.ConversationID, item.UserID, item.RunID, "queued").
		Update("status", "running")
	if run.Error != nil {
		return run.Error
	}
	if run.RowsAffected == 0 {
		var status string
		if err := tx.Model(&model.ConversationRun{}).
			Select("status").Where("conversation_id = ? AND user_id = ? AND run_id = ?", item.ConversationID, item.UserID, item.RunID).
			Scan(&status).Error; err != nil {
			return err
		}
		if status != "running" {
			return repository.ErrConflict
		}
	}
	if err := tx.Model(&model.Message{}).
		Where("conversation_id = ? AND user_id = ? AND run_id = ? AND role = ? AND status = ?", item.ConversationID, item.UserID, item.RunID, "user", "pending").
		Updates(map[string]any{"status": "success", "error_code": "", "error_message": ""}).Error; err != nil {
		return err
	}
	return nil
}

func appendGatewayMessageDelta(tx *gorm.DB, item *domainconversation.ExecutionEvent, column, delta string) error {
	result := tx.Model(&model.Message{}).
		Where(
			"conversation_id = ? AND user_id = ? AND run_id = ? AND role = ? AND (status = ? OR (status = ? AND error_code = ?))",
			item.ConversationID, item.UserID, item.RunID, "assistant", "pending", "error", "stream_interrupted",
		).
		UpdateColumn(column, gorm.Expr("COALESCE("+column+", '') || ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return repository.ErrConflict
	}
	return nil
}

func completeGatewayTurn(tx *gorm.DB, item *domainconversation.ExecutionEvent) error {
	endedAt := item.OccurredAt.UTC()
	normalizedErrorMessage := truncateText(item.ErrorMessage, 255)
	messageStatus, runStatus := "success", "success"
	if item.TerminalStatus == "interrupted" {
		messageStatus, runStatus = "interrupted", "interrupted"
	} else if item.TerminalStatus == "failed" {
		messageStatus, runStatus = "error", "error"
	}
	if err := tx.Model(&model.Message{}).
		Where("conversation_id = ? AND user_id = ? AND run_id = ? AND role = ? AND status = ?", item.ConversationID, item.UserID, item.RunID, "user", "pending").
		Updates(map[string]any{"status": "success", "error_code": "", "error_message": ""}).Error; err != nil {
		return err
	}
	assistant := tx.Model(&model.Message{}).
		Where(
			"conversation_id = ? AND user_id = ? AND run_id = ? AND role = ? AND (status = ? OR (status IN ? AND error_code = ?))",
			item.ConversationID, item.UserID, item.RunID, "assistant", "pending", []string{"error", "interrupted"}, "stream_interrupted",
		).
		Updates(map[string]any{"status": messageStatus, "error_code": item.ErrorCode, "error_message": normalizedErrorMessage, "latency_ms": item.LatencyMS})
	if assistant.Error != nil {
		return assistant.Error
	}
	if assistant.RowsAffected > 1 {
		return repository.ErrConflict
	}
	run := tx.Model(&model.ConversationRun{}).
		Where("conversation_id = ? AND user_id = ? AND run_id = ? AND (status IN ? OR (status = ? AND error_code = ?))", item.ConversationID, item.UserID, item.RunID, []string{"queued", "running"}, "interrupted", "stream_interrupted").
		Updates(map[string]any{"status": runStatus, "error_code": item.ErrorCode, "error_message": normalizedErrorMessage, "total_latency_ms": item.LatencyMS, "ended_at": endedAt})
	if run.Error != nil {
		return run.Error
	}
	if run.RowsAffected != 1 {
		return repository.ErrConflict
	}
	return nil
}

func (r *Repo) ListExecutionEvents(ctx context.Context, userID, conversationID uint, after uint64, runIDs []string, limit int) ([]domainconversation.ExecutionEvent, error) {
	if userID == 0 || conversationID == 0 || limit < 1 || limit > 500 {
		return nil, repository.ErrInvalidInput
	}
	rows := make([]model.ConversationExecutionEvent, 0)
	query := r.db.WithContext(ctx).Where("user_id = ? AND conversation_id = ? AND seq > ?", userID, conversationID, after)
	if len(runIDs) > 0 {
		query = query.Where("run_id IN ?", runIDs)
	}
	if err := query.Order("seq ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, translateError(err)
	}
	result := make([]domainconversation.ExecutionEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, domainconversation.ExecutionEvent{
			ConversationID: row.ConversationID, UserID: row.UserID, RunID: row.RunID,
			SourceKey: row.SourceKey, Seq: row.Seq, Kind: row.Kind,
			PayloadJSON: row.PayloadJSON, OccurredAt: row.OccurredAt,
		})
	}
	return result, nil
}
