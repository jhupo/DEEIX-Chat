package announcement

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	domainannouncement "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/announcement"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"golang.org/x/sync/singleflight"
)

const (
	maxAnnouncementTitleLength   = 120
	maxAnnouncementContentLength = 20000
	activeAnnouncementCacheTTL   = 30 * time.Second
	activeAnnouncementCacheMax   = 512
)

// Service 封装公告业务逻辑。
type Service struct {
	repo    repository.AnnouncementRepository
	tokens  TokenResolver
	client  *sub2api.Client
	cacheMu sync.Mutex
	cache   map[string]activeAnnouncementCacheEntry
	group   singleflight.Group
}

type activeAnnouncementCacheEntry struct {
	items     []domainannouncement.Announcement
	expiresAt time.Time
}

type TokenResolver interface {
	Sub2AccessTokenForSession(context.Context, uint, string) (string, error)
}

// NewService 创建公告服务。
func NewService(repo repository.AnnouncementRepository, tokens TokenResolver, client *sub2api.Client) *Service {
	return &Service{repo: repo, tokens: tokens, client: client, cache: make(map[string]activeAnnouncementCacheEntry)}
}

// ListActive 查询当前用户可展示公告。
func (s *Service) ListActive(ctx context.Context, userID uint, sessionID string) ([]domainannouncement.Announcement, error) {
	if userID == 0 || strings.TrimSpace(sessionID) == "" || s.tokens == nil || s.client == nil {
		return nil, repository.ErrInvalidInput
	}
	cacheKey := fmt.Sprintf("%d:%s", userID, strings.TrimSpace(sessionID))
	now := time.Now()
	s.cacheMu.Lock()
	if entry, ok := s.cache[cacheKey]; ok && now.Before(entry.expiresAt) {
		items := append([]domainannouncement.Announcement(nil), entry.items...)
		s.cacheMu.Unlock()
		return items, nil
	}
	delete(s.cache, cacheKey)
	s.cacheMu.Unlock()

	value, err, _ := s.group.Do(cacheKey, func() (any, error) {
		results, loadErr := s.loadActive(ctx, userID, sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		s.cacheMu.Lock()
		for len(s.cache) >= activeAnnouncementCacheMax {
			for key := range s.cache {
				delete(s.cache, key)
				break
			}
		}
		s.cache[cacheKey] = activeAnnouncementCacheEntry{items: append([]domainannouncement.Announcement(nil), results...), expiresAt: time.Now().Add(activeAnnouncementCacheTTL)}
		s.cacheMu.Unlock()
		return results, nil
	})
	if err != nil {
		return nil, err
	}
	results := value.([]domainannouncement.Announcement)
	return append([]domainannouncement.Announcement(nil), results...), nil
}

func (s *Service) loadActive(ctx context.Context, userID uint, sessionID string) ([]domainannouncement.Announcement, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	items, err := s.client.Announcements(ctx, token)
	if err != nil {
		return nil, err
	}
	results := make([]domainannouncement.Announcement, 0, len(items))
	for _, item := range items {
		if item.ID <= 0 || uint64(item.ID) > uint64(^uint(0)) {
			return nil, ErrInvalidAnnouncement
		}
		notifyMode := strings.TrimSpace(item.NotifyMode)
		if notifyMode != "popup" {
			notifyMode = "silent"
		}
		announcementType := domainannouncement.TypeGeneral
		priority := 0
		if notifyMode == "popup" {
			announcementType = domainannouncement.TypeInfo
			priority = 1
		}
		results = append(results, domainannouncement.Announcement{
			ID: uint(item.ID), Title: item.Title, ContentMarkdown: item.Content,
			Status: domainannouncement.StatusActive, Type: announcementType, NotifyMode: notifyMode,
			Pinned: notifyMode == "popup", Priority: priority, StartsAt: item.StartsAt, ExpiresAt: item.EndsAt,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ClosedAt: item.ReadAt,
		})
	}
	return results, nil
}

// ListAdmin 查询管理员公告列表。
func (s *Service) ListAdmin(ctx context.Context, input ListInput) ([]domainannouncement.Announcement, int64, error) {
	page, pageSize := normalizePage(input.Page, input.PageSize)
	return s.repo.ListAdminAnnouncements(ctx, repository.AnnouncementListFilter{
		Query:  strings.TrimSpace(input.Query),
		Status: strings.TrimSpace(input.Status),
		Type:   strings.TrimSpace(input.Type),
		Pinned: input.Pinned,
	}, (page-1)*pageSize, pageSize)
}

// Create 创建公告。
func (s *Service) Create(ctx context.Context, actorUserID uint, input WriteInput) (*domainannouncement.Announcement, error) {
	if actorUserID == 0 {
		return nil, repository.ErrInvalidInput
	}
	item, err := normalizeWriteInput(input, true)
	if err != nil {
		return nil, err
	}
	item.CreatedByUserID = actorUserID
	return s.repo.CreateAnnouncement(ctx, item)
}

// Update 更新公告。
func (s *Service) Update(ctx context.Context, id uint, input PatchInput) (*domainannouncement.Announcement, error) {
	if id == 0 {
		return nil, repository.ErrInvalidInput
	}
	patch, err := normalizePatchInput(input)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.PatchAnnouncement(ctx, id, patch)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return item, nil
}

// Delete 删除公告。
func (s *Service) Delete(ctx context.Context, id uint) error {
	if id == 0 {
		return repository.ErrInvalidInput
	}
	return mapRepositoryError(s.repo.DeleteAnnouncement(ctx, id))
}

// Close 记录当前用户关闭指定公告版本。
func (s *Service) Close(ctx context.Context, userID uint, sessionID string, announcementID uint) error {
	if userID == 0 || strings.TrimSpace(sessionID) == "" || announcementID == 0 || s.tokens == nil || s.client == nil {
		return repository.ErrInvalidInput
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if err := s.client.MarkAnnouncementRead(ctx, token, int64(announcementID)); err != nil {
		return err
	}
	cacheKey := fmt.Sprintf("%d:%s", userID, strings.TrimSpace(sessionID))
	s.cacheMu.Lock()
	delete(s.cache, cacheKey)
	s.cacheMu.Unlock()
	return nil
}

// ListInput 定义公告列表入参。
type ListInput struct {
	Query    string
	Status   string
	Type     string
	Pinned   *bool
	Page     int
	PageSize int
}

// WriteInput 定义公告创建入参。
type WriteInput struct {
	Title           string
	ContentMarkdown string
	Status          string
	Type            string
	Pinned          bool
	Priority        int
	StartsAt        *time.Time
	ExpiresAt       *time.Time
}

// PatchInput 定义公告更新入参。
type PatchInput struct {
	Title              *string
	ContentMarkdown    *string
	Status             *string
	Type               *string
	Pinned             *bool
	Priority           *int
	StartsAtSet        bool
	StartsAt           *time.Time
	ExpiresAtSet       bool
	ExpiresAt          *time.Time
	CreatedByUserIDSet bool
	CreatedByUserID    uint
}

func normalizeWriteInput(input WriteInput, requireContent bool) (*domainannouncement.Announcement, error) {
	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.ContentMarkdown)
	status := normalizeStatus(input.Status)
	announcementType := normalizeType(input.Type)
	if title == "" || len(title) > maxAnnouncementTitleLength {
		return nil, ErrInvalidAnnouncement
	}
	if requireContent && content == "" {
		return nil, ErrInvalidAnnouncement
	}
	if len(content) > maxAnnouncementContentLength {
		return nil, ErrInvalidAnnouncement
	}
	if status == "" {
		return nil, ErrInvalidAnnouncement
	}
	if announcementType == "" {
		return nil, ErrInvalidAnnouncement
	}
	if !validWindow(input.StartsAt, input.ExpiresAt) {
		return nil, ErrInvalidAnnouncement
	}
	return &domainannouncement.Announcement{
		Title:           title,
		ContentMarkdown: content,
		Status:          status,
		Type:            announcementType,
		Pinned:          input.Pinned,
		Priority:        input.Priority,
		StartsAt:        input.StartsAt,
		ExpiresAt:       input.ExpiresAt,
	}, nil
}

func normalizePatchInput(input PatchInput) (repository.AnnouncementPatch, error) {
	patch := repository.AnnouncementPatch{
		StartsAtSet:        input.StartsAtSet,
		StartsAt:           input.StartsAt,
		ExpiresAtSet:       input.ExpiresAtSet,
		ExpiresAt:          input.ExpiresAt,
		CreatedByUserIDSet: input.CreatedByUserIDSet,
		CreatedByUserID:    input.CreatedByUserID,
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" || len(title) > maxAnnouncementTitleLength {
			return repository.AnnouncementPatch{}, ErrInvalidAnnouncement
		}
		patch.Title = &title
	}
	if input.ContentMarkdown != nil {
		content := strings.TrimSpace(*input.ContentMarkdown)
		if content == "" || len(content) > maxAnnouncementContentLength {
			return repository.AnnouncementPatch{}, ErrInvalidAnnouncement
		}
		patch.ContentMarkdown = &content
	}
	if input.Status != nil {
		status := normalizeStatus(*input.Status)
		if status == "" {
			return repository.AnnouncementPatch{}, ErrInvalidAnnouncement
		}
		patch.Status = &status
	}
	if input.Type != nil {
		announcementType := normalizeType(*input.Type)
		if announcementType == "" {
			return repository.AnnouncementPatch{}, ErrInvalidAnnouncement
		}
		patch.Type = &announcementType
	}
	if input.Pinned != nil {
		patch.Pinned = input.Pinned
	}
	if input.Priority != nil {
		patch.Priority = input.Priority
	}
	if input.StartsAtSet && input.ExpiresAtSet && !validWindow(input.StartsAt, input.ExpiresAt) {
		return repository.AnnouncementPatch{}, ErrInvalidAnnouncement
	}
	return patch, nil
}

func normalizeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "", domainannouncement.StatusActive:
		return domainannouncement.StatusActive
	case domainannouncement.StatusInactive:
		return domainannouncement.StatusInactive
	default:
		return ""
	}
}

func normalizeType(announcementType string) string {
	switch strings.TrimSpace(announcementType) {
	case "", domainannouncement.TypeGeneral:
		return domainannouncement.TypeGeneral
	case domainannouncement.TypeCritical:
		return domainannouncement.TypeCritical
	case domainannouncement.TypeWarning:
		return domainannouncement.TypeWarning
	case domainannouncement.TypeInfo:
		return domainannouncement.TypeInfo
	case domainannouncement.TypeNormal:
		return domainannouncement.TypeNormal
	default:
		return ""
	}
}

func validWindow(startsAt *time.Time, expiresAt *time.Time) bool {
	if startsAt == nil || expiresAt == nil {
		return true
	}
	return expiresAt.After(*startsAt)
}

func normalizePage(page int, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	const maxPageSize = 1000
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func mapRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return ErrAnnouncementNotFound
	}
	return err
}
