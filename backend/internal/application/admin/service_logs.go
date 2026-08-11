package admin

import (
	"context"
	"errors"

	auditapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/audit"
	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	applogcleanup "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/logcleanup"
	systemeventapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/systemevent"
	domainaudit "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/audit"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainsystemevent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/systemevent"
)

// ListAuditLogs 查询审计日志分页列表。
func (s *Service) ListAuditLogs(ctx context.Context, page int, pageSize int, filter auditapp.ListFilter) ([]domainaudit.Log, int64, error) {
	return s.auditService.List(ctx, page, pageSize, filter)
}

// ListConversationEventLogs 查询管理员对话事件。
func (s *Service) ListConversationEventLogs(ctx context.Context, page int, pageSize int, filter appconversation.EventLogListFilter) ([]domainconversation.EventLog, int64, error) {
	if s.conversationEventSvc == nil {
		return []domainconversation.EventLog{}, 0, nil
	}
	return s.conversationEventSvc.ListConversationEventLogs(ctx, page, pageSize, filter)
}

// GetConversationEventLog 查询管理员对话事件详情。
func (s *Service) GetConversationEventLog(ctx context.Context, eventID uint) (*domainconversation.EventLog, error) {
	if s.conversationEventSvc == nil {
		return nil, appconversation.ErrConversationEventNotFound
	}
	return s.conversationEventSvc.GetConversationEventLog(ctx, eventID)
}

// ListSystemEvents 查询系统事件分页列表。
func (s *Service) ListSystemEvents(ctx context.Context, page int, pageSize int, filter systemeventapp.ListFilter) ([]domainsystemevent.Event, int64, error) {
	if s.systemEventService == nil {
		return []domainsystemevent.Event{}, 0, nil
	}
	return s.systemEventService.List(ctx, page, pageSize, filter)
}

// CleanupLogs 物理清理指定截止时间之前的一类日志。
func (s *Service) CleanupLogs(ctx context.Context, input applogcleanup.Input) (*applogcleanup.Result, error) {
	if s.logCleanupService == nil {
		return nil, errors.New("log cleanup service unavailable")
	}
	return s.logCleanupService.Cleanup(ctx, input)
}

// CleanupConversationRuns 清理指定运行的全部对话事件。
func (s *Service) CleanupConversationRuns(ctx context.Context, input applogcleanup.ConversationRunInput) (*applogcleanup.ConversationRunResult, error) {
	if s.logCleanupService == nil {
		return nil, errors.New("log cleanup service unavailable")
	}
	return s.logCleanupService.CleanupConversationRuns(ctx, input)
}
