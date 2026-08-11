package conversation

import (
	"context"
	"time"

	appcompact "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/compact"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"go.uber.org/zap"
)

// postSendCompactionTask 保存消息发送后执行上下文压缩所需的运行信息。
type postSendCompactionTask struct {
	Async          bool
	Input          appcompact.MaybeCompactConversationInput
	ConversationID uint
	UserID         uint
	MessageID      uint
	RunID          string
	PreserveTurns  int
	OnEvent        func(eventType string, payload map[string]interface{}) error
	TraceRecorder  *messageTraceRecorder
}

// runPostSendCompaction 在独立超时内执行后置压缩任务。
func (s *Service) runPostSendCompaction(task *postSendCompactionTask, message *model.Message) {
	if task == nil {
		return
	}
	if s.compactSvc == nil {
		s.completePostSendCompactionTrace(task, message)
		return
	}
	run := func(ctx context.Context) {
		ctx = withBasicServiceContext(ctx, task.UserID, task.ConversationID)
		snapshot, err := s.compactSvc.MaybeCompactConversation(ctx, task.Input)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("post_send_compaction_failed",
					zap.Uint("user_id", task.UserID),
					zap.Uint("conversation_id", task.ConversationID),
					zap.Error(err),
				)
			}
			s.completePostSendCompactionTrace(task, message)
			return
		}
		if snapshot != nil {
			s.invalidateSnapshotCache(task.ConversationID)
			_ = s.repo.UpdateConversationLastResponseID(ctx, task.ConversationID, "")
			s.persistSnapshotContextArtifact(ctx, snapshotContextArtifactInput{
				ConversationID: task.ConversationID,
				UserID:         task.UserID,
				MessageID:      task.MessageID,
				RunID:          task.RunID,
				Snapshot:       snapshot,
			})
			if task.TraceRecorder != nil {
				summary, markdown, payload := buildCompactionProcessTrace(snapshot)
				task.TraceRecorder.appendProcessSection(summary, markdown, payload, messageTraceStatusStreaming)
			}
			if task.OnEvent != nil {
				preview := []rune(snapshot.SummaryText)
				if len(preview) > 80 {
					preview = preview[:80]
				}
				emitEvent(task.OnEvent, "compact_done", map[string]interface{}{
					"method":          snapshot.Strategy,
					"freed_tokens":    snapshot.SourceTokens - snapshot.SummaryTokens,
					"kept_turns":      task.PreserveTurns,
					"summary_preview": string(preview),
				})
			}
		}
		s.completePostSendCompactionTrace(task, message)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	run(ctx)
}

// discardPostSendCompaction 在发送中止时终止尚未开始的压缩 trace。
func (s *Service) discardPostSendCompaction(result *SendMessageResult) {
	if result == nil || result.postSendCompaction == nil {
		return
	}
	task := result.postSendCompaction
	result.postSendCompaction = nil
	s.completePostSendCompactionTrace(task, &result.AssistantMessage)
}

// completePostSendCompactionTrace 将同步压缩 trace 回填到响应消息。
func (s *Service) completePostSendCompactionTrace(task *postSendCompactionTask, message *model.Message) {
	if task == nil || task.TraceRecorder == nil {
		return
	}
	task.TraceRecorder.complete()
	task.TraceRecorder.attachToMessage(message)
}
