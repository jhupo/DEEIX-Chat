package admin

import (
	domainaudit "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/audit"
	domainsystemevent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/systemevent"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
)

type UserListFilter struct {
	Query string
}

type RevokeUserSessionsResult struct {
	Revoked bool
}

type UserAuthEventsResult struct {
	Total   int64
	Results []domainuser.AuthEvent
}

type AuditLogsResult struct {
	Total   int64
	Results []domainaudit.Log
}

type SystemEventsResult struct {
	Total   int64
	Results []domainsystemevent.Event
}
