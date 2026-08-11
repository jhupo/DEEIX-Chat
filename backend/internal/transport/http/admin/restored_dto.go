package admin

import (
	"time"

	appadmin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/admin"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/userview"
	domainaudit "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/audit"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainsystemevent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/systemevent"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
)

type UserResponse struct {
	ID                    uint       `json:"id"`
	PublicID              string     `json:"publicID"`
	Username              string     `json:"username"`
	DisplayName           string     `json:"displayName"`
	AvatarURL             string     `json:"avatarURL"`
	Email                 string     `json:"email"`
	Role                  string     `json:"role"`
	Status                string     `json:"status"`
	Timezone              string     `json:"timezone"`
	Locale                string     `json:"locale"`
	ProfilePreferences    string     `json:"profilePreferences"`
	AppearancePreferences string     `json:"appearancePreferences"`
	LastLoginAt           *time.Time `json:"lastLoginAt"`
	LastActiveAt          *time.Time `json:"lastActiveAt"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}
type UserDataResponse struct {
	User UserResponse `json:"user"`
}
type RevokeUserSessionsResponse struct {
	Revoked bool `json:"revoked"`
}
type CleanupLogsRequest struct {
	Type   string `json:"type" binding:"required"`
	Before string `json:"before" binding:"required"`
}
type CleanupLogsResponse struct {
	Type         string    `json:"type"`
	Before       time.Time `json:"before"`
	DeletedCount int64     `json:"deletedCount"`
}
type CleanupConversationRunsRequest struct {
	RunIDs []string `json:"runIDs" binding:"required,min=1,max=100"`
}
type CleanupConversationRunsResponse struct {
	RunCount     int   `json:"runCount"`
	DeletedCount int64 `json:"deletedCount"`
}
type AuthEventResponse struct {
	ID              uint      `json:"id"`
	RequestID       string    `json:"requestID"`
	UserID          uint      `json:"userID"`
	Username        string    `json:"username"`
	UserDisplayName string    `json:"userDisplayName"`
	UserLabel       string    `json:"userLabel"`
	EventType       string    `json:"eventType"`
	Result          string    `json:"result"`
	Reason          string    `json:"reason"`
	ClientIP        string    `json:"clientIP"`
	UserAgent       string    `json:"userAgent"`
	DetailJSON      string    `json:"detailJSON"`
	OccurredAt      time.Time `json:"occurredAt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
type AuditLogResponse struct {
	ID               uint      `json:"id"`
	RequestID        string    `json:"requestID"`
	ActorUserID      uint      `json:"actorUserID"`
	ActorUsername    string    `json:"actorUsername"`
	ActorDisplayName string    `json:"actorDisplayName"`
	ActorLabel       string    `json:"actorLabel"`
	Action           string    `json:"action"`
	Resource         string    `json:"resource"`
	ResourceID       string    `json:"resourceID"`
	IP               string    `json:"ip"`
	UserAgent        string    `json:"userAgent"`
	DetailJSON       string    `json:"detailJSON"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
type SystemEventResponse struct {
	ID         uint      `json:"id"`
	RequestID  string    `json:"requestID"`
	TraceID    string    `json:"traceID"`
	Level      string    `json:"level"`
	Source     string    `json:"source"`
	Event      string    `json:"event"`
	Resource   string    `json:"resource"`
	ResourceID string    `json:"resourceID"`
	Message    string    `json:"message"`
	DetailJSON string    `json:"detailJSON"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
type ConversationEventResponse struct {
	ID              uint       `json:"id"`
	MessageID       uint       `json:"messageID"`
	ConversationID  uint       `json:"conversationID"`
	UserID          uint       `json:"userID"`
	Username        string     `json:"username"`
	UserDisplayName string     `json:"userDisplayName"`
	UserLabel       string     `json:"userLabel"`
	RunID           string     `json:"runID"`
	EventScope      string     `json:"eventScope"`
	EventID         string     `json:"eventID"`
	EventType       string     `json:"eventType"`
	Phase           string     `json:"phase"`
	Stage           string     `json:"stage"`
	Status          string     `json:"status"`
	Title           string     `json:"title"`
	Summary         string     `json:"summary"`
	ContentMarkdown string     `json:"contentMarkdown"`
	PayloadJSON     string     `json:"payloadJSON"`
	PayloadOmitted  bool       `json:"payloadOmitted"`
	Seq             int        `json:"seq"`
	ToolName        string     `json:"toolName"`
	LatencyMS       int64      `json:"latencyMS"`
	StartedAt       time.Time  `json:"startedAt"`
	EndedAt         *time.Time `json:"endedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}
type AdminPageData[T any] struct {
	Total   int64 `json:"total"`
	Results []T   `json:"results"`
}
type UserListResponseDoc struct {
	ErrorMsg string                      `json:"errorMsg"`
	Data     AdminPageData[UserResponse] `json:"data"`
}
type UserDataResponseDoc struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     UserDataResponse `json:"data"`
}
type RevokeUserSessionsResponseDoc struct {
	ErrorMsg string                     `json:"errorMsg"`
	Data     RevokeUserSessionsResponse `json:"data"`
}
type AuthEventListResponseDoc struct {
	ErrorMsg string                           `json:"errorMsg"`
	Data     AdminPageData[AuthEventResponse] `json:"data"`
}
type AuditLogListResponseDoc struct {
	ErrorMsg string                          `json:"errorMsg"`
	Data     AdminPageData[AuditLogResponse] `json:"data"`
}
type SystemEventListResponseDoc struct {
	ErrorMsg string                             `json:"errorMsg"`
	Data     AdminPageData[SystemEventResponse] `json:"data"`
}
type ConversationEventListResponseDoc struct {
	ErrorMsg string                                   `json:"errorMsg"`
	Data     AdminPageData[ConversationEventResponse] `json:"data"`
}
type ConversationEventDetailResponseDoc struct {
	ErrorMsg string                    `json:"errorMsg"`
	Data     ConversationEventResponse `json:"data"`
}
type CleanupLogsResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     CleanupLogsResponse `json:"data"`
}
type CleanupConversationRunsResponseDoc struct {
	ErrorMsg string                          `json:"errorMsg"`
	Data     CleanupConversationRunsResponse `json:"data"`
}

func toUserResponse(v userview.UserView) UserResponse {
	return UserResponse{ID: v.ID, PublicID: v.PublicID, Username: v.Username, DisplayName: v.DisplayName, AvatarURL: v.AvatarURL, Email: v.Email, Role: v.Role, Status: v.Status, Timezone: v.Timezone, Locale: v.Locale, ProfilePreferences: v.ProfilePreferences, AppearancePreferences: v.AppearancePreferences, LastLoginAt: v.LastLoginAt, LastActiveAt: v.LastActiveAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toAuthEventResponse(v domainuser.AuthEvent, l appadmin.UserLabel) AuthEventResponse {
	return AuthEventResponse{ID: v.ID, RequestID: v.RequestID, UserID: v.UserID, Username: l.Username, UserDisplayName: l.DisplayName, UserLabel: l.Label, EventType: v.EventType, Result: v.Result, Reason: v.Reason, ClientIP: v.ClientIP, UserAgent: v.UserAgent, DetailJSON: v.DetailJSON, OccurredAt: v.OccurredAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toAuditLogResponse(v domainaudit.Log, l appadmin.UserLabel) AuditLogResponse {
	return AuditLogResponse{ID: v.ID, RequestID: v.RequestID, ActorUserID: v.ActorUserID, ActorUsername: l.Username, ActorDisplayName: l.DisplayName, ActorLabel: l.Label, Action: v.Action, Resource: v.Resource, ResourceID: v.ResourceID, IP: v.IP, UserAgent: v.UserAgent, DetailJSON: v.DetailJSON, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toSystemEventResponse(v domainsystemevent.Event) SystemEventResponse {
	return SystemEventResponse{ID: v.ID, RequestID: v.RequestID, TraceID: v.TraceID, Level: v.Level, Source: v.Source, Event: v.Event, Resource: v.Resource, ResourceID: v.ResourceID, Message: v.Message, DetailJSON: v.DetailJSON, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toConversationEventResponse(v domainconversation.EventLog, l appadmin.UserLabel) ConversationEventResponse {
	return ConversationEventResponse{ID: v.ID, MessageID: v.MessageID, ConversationID: v.ConversationID, UserID: v.UserID, Username: l.Username, UserDisplayName: l.DisplayName, UserLabel: l.Label, RunID: v.RunID, EventScope: v.EventScope, EventID: v.EventID, EventType: v.EventType, Phase: v.Phase, Stage: v.Stage, Status: v.Status, Title: v.Title, Summary: v.Summary, ContentMarkdown: v.ContentMarkdown, PayloadJSON: v.PayloadJSON, PayloadOmitted: v.PayloadOmitted, Seq: v.Seq, ToolName: v.ToolName, LatencyMS: v.LatencyMS, StartedAt: v.StartedAt, EndedAt: v.EndedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func auditActorIDs(items []domainaudit.Log) []uint {
	ids := make([]uint, 0, len(items))
	for _, v := range items {
		ids = append(ids, v.ActorUserID)
	}
	return ids
}
func authUserIDs(items []domainuser.AuthEvent) []uint {
	ids := make([]uint, 0, len(items))
	for _, v := range items {
		ids = append(ids, v.UserID)
	}
	return ids
}
func conversationUserIDs(items []domainconversation.EventLog) []uint {
	ids := make([]uint, 0, len(items))
	for _, v := range items {
		ids = append(ids, v.UserID)
	}
	return ids
}
