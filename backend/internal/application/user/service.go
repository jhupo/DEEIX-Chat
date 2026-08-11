package user

import (
	"context"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type Service struct {
	repo                repository.UserRepository
	avatarContentOpener avatarContentOpener
}
type avatarContentOpener interface {
	OpenAvatarFileContent(context.Context, uint, string) (*AvatarFileContent, error)
}
type AvatarFileContent struct {
	Reader      io.ReadCloser
	ContentType string
	SizeBytes   int64
	ModTime     time.Time
	FileName    string
}
type AvatarContentResult struct {
	Reader      io.ReadCloser
	ContentType string
	SizeBytes   int64
	ModTime     time.Time
}

func NewService(repo repository.UserRepository) *Service             { return &Service{repo: repo} }
func (s *Service) SetAvatarContentOpener(opener avatarContentOpener) { s.avatarContentOpener = opener }
func (s *Service) GetByID(ctx context.Context, id uint) (*domainuser.User, error) {
	return s.repo.GetByID(ctx, id)
}
func (s *Service) GetByPublicID(ctx context.Context, id string) (*domainuser.User, error) {
	return s.repo.GetByPublicID(ctx, strings.TrimSpace(id))
}
func (s *Service) OpenAvatarContent(ctx context.Context, publicID string) (*AvatarContentResult, error) {
	item, err := s.GetByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	fileID, ok := domainuser.ParseFileAvatarURL(item.AvatarURL)
	if !ok || s.avatarContentOpener == nil {
		return nil, ErrAvatarNotFound
	}
	content, err := s.avatarContentOpener.OpenAvatarFileContent(ctx, item.ID, fileID)
	if err != nil {
		return nil, err
	}
	contentType := content.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(content.FileName))
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		_ = content.Reader.Close()
		return nil, ErrAvatarNotFound
	}
	return &AvatarContentResult{Reader: content.Reader, ContentType: contentType, SizeBytes: content.SizeBytes, ModTime: content.ModTime}, nil
}
func (s *Service) UpdateFields(ctx context.Context, id uint, input repository.UpdateUserFieldsInput) (*domainuser.User, error) {
	return s.repo.UpdateProfile(ctx, id, input)
}
func (s *Service) ListUsers(ctx context.Context, page, pageSize int, filter repository.UserListFilter) ([]domainuser.User, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return s.repo.ListUsers(ctx, (page-1)*pageSize, pageSize, filter)
}
func (s *Service) ListLatestSessionActivityByUserIDs(ctx context.Context, ids []uint) (map[uint]time.Time, error) {
	return s.repo.ListLatestSessionActivityByUserIDs(ctx, ids)
}
func (s *Service) RevokeAllSessions(ctx context.Context, id uint, reason string) error {
	return s.repo.RevokeAllSessions(ctx, id, reason)
}
func (s *Service) ListAuthEvents(ctx context.Context, userID uint, eventType, result string, page, pageSize int) ([]domainuser.AuthEvent, int64, error) {
	return s.repo.ListAuthEvents(ctx, userID, eventType, result, (page-1)*pageSize, pageSize)
}
func (s *Service) RecordAuthEvent(ctx context.Context, userID uint, requestID, eventType, result, reason, clientIP, userAgent, detail string) error {
	return s.repo.RecordAuthEvent(ctx, userID, requestID, eventType, result, reason, clientIP, userAgent, detail)
}
