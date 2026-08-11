package admin

import (
	"context"
	"strconv"
	"strings"

	auditapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/audit"
	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	applogcleanup "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/logcleanup"
	systemeventapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/systemevent"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/userview"
	domainaudit "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/audit"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainsystemevent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/systemevent"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type userService interface {
	ListUsers(context.Context, int, int, repository.UserListFilter) ([]domainuser.User, int64, error)
	GetByID(context.Context, uint) (*domainuser.User, error)
	RevokeAllSessions(context.Context, uint, string) error
	UpdateFields(context.Context, uint, repository.UpdateUserFieldsInput) (*domainuser.User, error)
	ListAuthEvents(context.Context, uint, string, string, int, int) ([]domainuser.AuthEvent, int64, error)
}
type auditService interface {
	Write(context.Context, string, uint, string, string, string, string, string, interface{})
	List(context.Context, int, int, auditapp.ListFilter) ([]domainaudit.Log, int64, error)
}
type systemEventService interface {
	List(context.Context, int, int, systemeventapp.ListFilter) ([]domainsystemevent.Event, int64, error)
}
type conversationEventService interface {
	ListConversationEventLogs(context.Context, int, int, appconversation.EventLogListFilter) ([]domainconversation.EventLog, int64, error)
	GetConversationEventLog(context.Context, uint) (*domainconversation.EventLog, error)
}
type logCleanupService interface {
	Cleanup(context.Context, applogcleanup.Input) (*applogcleanup.Result, error)
	CleanupConversationRuns(context.Context, applogcleanup.ConversationRunInput) (*applogcleanup.ConversationRunResult, error)
}

type Service struct {
	userService                userService
	auditService               auditService
	systemEventService         systemEventService
	conversationEventSvc       conversationEventService
	logCleanupService          logCleanupService
	permissionGroupRepo        permissionGroupRepo
	permissionGroupModelLookup permissionGroupModelLookup
}
type UserLabel struct {
	ID          uint
	Username    string
	DisplayName string
	Label       string
}
type PatchUserInput struct {
	AvatarURL, DisplayName, Email, Phone, Role, Status, Timezone, Locale, ProfilePreferences *string
	Reason                                                                                   string
}

func NewService(users userService, audits auditService) *Service {
	return &Service{userService: users, auditService: audits}
}
func (s *Service) SetSystemEventService(service systemEventService) { s.systemEventService = service }
func (s *Service) SetConversationEventService(service conversationEventService) {
	s.conversationEventSvc = service
}
func (s *Service) SetLogCleanupService(service logCleanupService) { s.logCleanupService = service }
func (s *Service) ListUsers(ctx context.Context, page, size int, filter UserListFilter) ([]userview.UserView, int64, error) {
	items, total, err := s.userService.ListUsers(ctx, page, size, repository.UserListFilter{Query: filter.Query})
	if err != nil {
		return nil, 0, err
	}
	views := make([]userview.UserView, 0, len(items))
	for _, item := range items {
		views = append(views, userview.FromUser(item))
	}
	return views, total, nil
}
func (s *Service) BuildUserView(_ context.Context, item domainuser.User) (userview.UserView, error) {
	return userview.FromUser(item), nil
}
func (s *Service) ResolveUserLabels(ctx context.Context, ids []uint) map[uint]UserLabel {
	result := make(map[uint]UserLabel, len(ids))
	for _, id := range ids {
		item, err := s.userService.GetByID(ctx, id)
		if err != nil {
			continue
		}
		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = item.Username
		}
		result[id] = UserLabel{ID: id, Username: item.Username, DisplayName: item.DisplayName, Label: label}
	}
	return result
}
func (s *Service) getActor(ctx context.Context, id uint) (*domainuser.User, error) {
	return s.userService.GetByID(ctx, id)
}
func (s *Service) PatchUserByAdmin(ctx context.Context, requestID string, actorID, targetID uint, req PatchUserInput, ip, agent string) (*domainuser.User, error) {
	actor, err := s.getActor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	target, err := s.userService.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if actor.Role != domainuser.RoleSuperAdmin && target.Role == domainuser.RoleSuperAdmin {
		return nil, ErrSuperAdminManagementNotAllowed
	}
	update := repository.UpdateUserFieldsInput{AvatarURL: req.AvatarURL, DisplayName: req.DisplayName, Timezone: req.Timezone, Locale: req.Locale, ProfilePreferences: req.ProfilePreferences}
	if update.IsZero() {
		return nil, ErrEmptyAdminUserPatch
	}

	updated := target
	if !update.IsZero() {
		updated, err = s.userService.UpdateFields(ctx, targetID, update)
		if err != nil {
			return nil, err
		}
	}
	s.WriteAuditLog(ctx, requestID, actorID, "admin_patch_user", "user", strconv.FormatUint(uint64(targetID), 10), ip, agent, map[string]string{"reason": strings.TrimSpace(req.Reason)})
	return updated, nil
}
func (s *Service) RevokeUserSessionsByAdmin(ctx context.Context, requestID string, actorID, targetID uint, ip, agent string) (*RevokeUserSessionsResult, error) {
	if _, err := s.getActor(ctx, actorID); err != nil {
		return nil, err
	}
	if err := s.userService.RevokeAllSessions(ctx, targetID, "admin_revoke_sessions"); err != nil {
		return nil, err
	}
	s.WriteAuditLog(ctx, requestID, actorID, "admin_revoke_user_sessions", "user", strconv.FormatUint(uint64(targetID), 10), ip, agent, nil)
	return &RevokeUserSessionsResult{Revoked: true}, nil
}

func (s *Service) ListUserAuthEventsByAdmin(ctx context.Context, userID uint, eventType, result string, page, size int) ([]domainuser.AuthEvent, int64, error) {
	return s.userService.ListAuthEvents(ctx, userID, eventType, result, page, size)
}

func (s *Service) WriteAuditLog(ctx context.Context, requestID string, actorID uint, action, resource, resourceID, ip, agent string, detail interface{}) {
	s.auditService.Write(ctx, requestID, actorID, action, resource, resourceID, ip, agent, detail)
}
