package announcement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainannouncement "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/announcement"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

type announcementTokenResolver struct{}

func (announcementTokenResolver) Sub2AccessTokenForSession(context.Context, uint, string) (string, error) {
	return "token", nil
}

func TestCreateAnnouncementValidation(t *testing.T) {
	service := NewService(&fakeRepo{}, nil, nil)
	if _, err := service.Create(context.Background(), 1, WriteInput{Status: domainannouncement.StatusActive}); !errors.Is(err, ErrInvalidAnnouncement) {
		t.Fatalf("Create() error = %v, want ErrInvalidAnnouncement", err)
	}
	if _, err := service.Create(context.Background(), 0, WriteInput{Title: "A", ContentMarkdown: "Body"}); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateAnnouncementAcceptsValidWindow(t *testing.T) {
	start := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	repo := &fakeRepo{}
	service := NewService(repo, nil, nil)

	item, err := service.Create(context.Background(), 7, WriteInput{
		Title:           "Notice",
		ContentMarkdown: "Hello",
		Status:          domainannouncement.StatusActive,
		Priority:        10,
		StartsAt:        &start,
		ExpiresAt:       &end,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if item.CreatedByUserID != 7 || item.Priority != 10 || item.Status != domainannouncement.StatusActive {
		t.Fatalf("Create() item = %#v", item)
	}
	if item.Type != domainannouncement.TypeGeneral {
		t.Fatalf("Create() type = %q, want %q", item.Type, domainannouncement.TypeGeneral)
	}
}

func TestUpdateAnnouncementRejectsInvalidStatus(t *testing.T) {
	service := NewService(&fakeRepo{}, nil, nil)
	status := "archived"
	if _, err := service.Update(context.Background(), 1, PatchInput{Status: &status}); !errors.Is(err, ErrInvalidAnnouncement) {
		t.Fatalf("Update() error = %v, want ErrInvalidAnnouncement", err)
	}
}

func TestCloseRejectsInvalidInput(t *testing.T) {
	service := NewService(&fakeRepo{}, nil, nil)
	if err := service.Close(context.Background(), 0, "session", 1); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("Close() error = %v, want ErrInvalidInput", err)
	}
	if err := service.Close(context.Background(), 1, "", 1); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("Close() error = %v, want ErrInvalidInput", err)
	}
	if err := service.Close(context.Background(), 1, "session", 0); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("Close() error = %v, want ErrInvalidInput", err)
	}
}

func TestSub2AnnouncementProjectionAndRead(t *testing.T) {
	read := false
	listCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/announcements", func(w http.ResponseWriter, r *http.Request) {
		listCalls++
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{map[string]any{
			"id": 7, "title": "Maintenance", "content": "Tonight", "notify_mode": "popup",
			"created_at": "2026-08-11T00:00:00Z", "updated_at": "2026-08-11T01:00:00Z",
		}}})
	})
	mux.HandleFunc("/api/v1/announcements/7/read", func(w http.ResponseWriter, r *http.Request) {
		read = r.Method == http.MethodPost
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"message": "ok"}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(&fakeRepo{}, announcementTokenResolver{}, client)
	items, err := service.ListActive(context.Background(), 1, "session")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListActive() = %#v, %v", items, err)
	}
	if items[0].NotifyMode != "popup" || !items[0].Pinned || items[0].Type != domainannouncement.TypeInfo {
		t.Fatalf("projection = %#v", items[0])
	}
	if _, err := service.ListActive(context.Background(), 1, "session"); err != nil || listCalls != 1 {
		t.Fatalf("cached ListActive() calls = %d, error = %v", listCalls, err)
	}
	if err := service.Close(context.Background(), 1, "session", 7); err != nil || !read {
		t.Fatalf("Close() error = %v, read = %v", err, read)
	}
	if _, err := service.ListActive(context.Background(), 1, "session"); err != nil || listCalls != 2 {
		t.Fatalf("invalidated ListActive() calls = %d, error = %v", listCalls, err)
	}
}

type fakeRepo struct {
	item             domainannouncement.Announcement
	includeDismissed bool
}

func (r *fakeRepo) ListActiveAnnouncements(_ context.Context, _ uint, _ time.Time, includeDismissed bool) ([]domainannouncement.Announcement, error) {
	r.includeDismissed = includeDismissed
	return []domainannouncement.Announcement{}, nil
}

func (r *fakeRepo) ListAdminAnnouncements(context.Context, repository.AnnouncementListFilter, int, int) ([]domainannouncement.Announcement, int64, error) {
	return []domainannouncement.Announcement{}, 0, nil
}

func (r *fakeRepo) CreateAnnouncement(_ context.Context, item *domainannouncement.Announcement) (*domainannouncement.Announcement, error) {
	item.ID = 1
	r.item = *item
	return item, nil
}

func (r *fakeRepo) PatchAnnouncement(_ context.Context, id uint, patch repository.AnnouncementPatch) (*domainannouncement.Announcement, error) {
	if id == 0 {
		return nil, repository.ErrInvalidInput
	}
	if patch.Status != nil {
		r.item.Status = *patch.Status
	}
	if patch.Type != nil {
		r.item.Type = *patch.Type
	}
	if patch.Pinned != nil {
		r.item.Pinned = *patch.Pinned
	}
	return &r.item, nil
}

func (r *fakeRepo) DeleteAnnouncement(context.Context, uint) error {
	return nil
}

func (r *fakeRepo) DismissAnnouncementToday(context.Context, uint, uint, time.Time, time.Time, time.Time) error {
	return nil
}

func (r *fakeRepo) CloseAnnouncement(context.Context, uint, uint, time.Time, time.Time) error {
	return nil
}
