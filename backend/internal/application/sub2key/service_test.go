package sub2key

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	domainsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/sub2key"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

type keyTestTokens struct{}

func (keyTestTokens) Sub2AccessTokenForSession(context.Context, uint, string) (string, error) {
	return "token", nil
}

type keyTestRepo struct{}

func (keyTestRepo) ListSub2KeyBindings(context.Context, uint) ([]domainsub2key.Binding, error) {
	return nil, nil
}
func (keyTestRepo) GetSub2KeyBinding(context.Context, uint, string) (*domainsub2key.Binding, error) {
	return nil, repository.ErrNotFound
}
func (keyTestRepo) GetSub2KeyBindingByRemoteKeyID(context.Context, uint, int64) (*domainsub2key.Binding, error) {
	return nil, repository.ErrNotFound
}
func (keyTestRepo) UpsertSub2KeyBinding(context.Context, *domainsub2key.Binding) error { return nil }
func (keyTestRepo) MarkSub2KeyBindingUnavailable(context.Context, uint, int64, time.Time) error {
	return nil
}
func (keyTestRepo) RevokeSub2KeyBinding(context.Context, uint, string) error { return nil }
func (keyTestRepo) GetSub2KeyBindingOperation(context.Context, uint, string) (*domainsub2key.BindingOperation, error) {
	return nil, repository.ErrNotFound
}
func (keyTestRepo) CreateSub2KeyBindingOperation(context.Context, *domainsub2key.BindingOperation) (bool, error) {
	return true, nil
}
func (keyTestRepo) CompleteSub2KeyBindingOperation(context.Context, uint, string, string) error {
	return nil
}

func TestListRemotePaginatesToTotal(t *testing.T) {
	keys := make([]map[string]any, 101)
	for i := range keys {
		keys[i] = map[string]any{"id": i + 1, "user_id": 7, "name": "key", "key": "sk-test-key", "status": "active"}
	}
	service := newKeyTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/profile":
			writeKeyEnvelope(w, map[string]any{"id": 7})
		case "/api/v1/keys":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			start := (page - 1) * remoteKeyPageSize
			end := start + remoteKeyPageSize
			if end > len(keys) {
				end = len(keys)
			}
			writeKeyEnvelope(w, map[string]any{"total": len(keys), "items": keys[start:end]})
		default:
			http.NotFound(w, r)
		}
	})
	items, err := service.ListRemote(context.Background(), 1, "session")
	if err != nil {
		t.Fatalf("ListRemote() error = %v", err)
	}
	if len(items) != len(keys) || items[100].RemoteKeyID != 101 {
		t.Fatalf("ListRemote() = %#v", items)
	}
}

func TestListRemoteRejectsInconsistentTotal(t *testing.T) {
	service := newKeyTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/profile":
			writeKeyEnvelope(w, map[string]any{"id": 7})
		case "/api/v1/keys":
			page := r.URL.Query().Get("page")
			total := 101
			items := make([]map[string]any, 100)
			if page == "2" {
				total, items = 102, []map[string]any{{"id": 101, "user_id": 7, "key": "sk-test-key", "status": "active"}}
			}
			for i := range items {
				if items[i] == nil {
					items[i] = map[string]any{"id": i + 1, "user_id": 7, "key": "sk-test-key", "status": "active"}
				}
			}
			writeKeyEnvelope(w, map[string]any{"total": total, "items": items})
		}
	})
	if _, err := service.ListRemote(context.Background(), 1, "session"); err != ErrInvalidBinding {
		t.Fatalf("ListRemote() error = %v, want %v", err, ErrInvalidBinding)
	}
}

func newKeyTestService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("sub2api.New() error = %v", err)
	}
	return NewService(keyTestRepo{}, keyTestTokens{}, client, "test-encryption-key")
}

func writeKeyEnvelope(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": value})
}

func TestRemoteKeyListUsesShortLivedCache(t *testing.T) {
	profileCalls := 0
	keyCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/user/profile", func(w http.ResponseWriter, _ *http.Request) {
		profileCalls++
		writeKeyEnvelope(w, map[string]any{"id": 7})
	})
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, _ *http.Request) {
		keyCalls++
		writeKeyEnvelope(w, map[string]any{"total": 0, "items": []any{}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(keyTestRepo{}, keyTestTokens{}, client, "key")
	for range 2 {
		if _, _, err := service.currentRemoteKeys(context.Background(), 1, "session"); err != nil {
			t.Fatal(err)
		}
	}
	if profileCalls != 1 || keyCalls != 1 {
		t.Fatalf("upstream calls = profile %d, keys %d", profileCalls, keyCalls)
	}
}

func TestRemoteKeyCachePrunesExpiredEntriesAndStaysBounded(t *testing.T) {
	now := time.Now()
	service := &Service{remoteCache: make(map[string]remoteKeyCacheEntry)}
	service.remoteCache["expired"] = remoteKeyCacheEntry{
		keys:      []sub2api.APIKey{{Key: "raw-secret"}},
		expiresAt: now.Add(-time.Second),
	}
	for i := 0; i < remoteKeyCacheMax+8; i++ {
		service.remoteCache[strconv.Itoa(i)] = remoteKeyCacheEntry{expiresAt: now.Add(time.Duration(i+1) * time.Second)}
	}

	service.pruneRemoteCacheLocked(now, 0)
	if _, found := service.remoteCache["expired"]; found {
		t.Fatal("expired raw API key remained cached")
	}
	if len(service.remoteCache) != remoteKeyCacheMax {
		t.Fatalf("cache size = %d, want %d", len(service.remoteCache), remoteKeyCacheMax)
	}
}

type countingKeyRepo struct {
	keyTestRepo
	getCalls, revokeCalls int
}

type syncKeyRepo struct {
	keyTestRepo
	items       []domainsub2key.Binding
	unavailable []int64
	upserts     []domainsub2key.Binding
}

type reclaimingKeyRepo struct {
	keyTestRepo
	operation   domainsub2key.BindingOperation
	createCalls int
	upserted    bool
	completed   bool
}

func (r *reclaimingKeyRepo) GetSub2KeyBindingOperation(context.Context, uint, string) (*domainsub2key.BindingOperation, error) {
	operation := r.operation
	return &operation, nil
}
func (r *reclaimingKeyRepo) CreateSub2KeyBindingOperation(context.Context, *domainsub2key.BindingOperation) (bool, error) {
	r.createCalls++
	return true, nil
}
func (r *reclaimingKeyRepo) UpsertSub2KeyBinding(context.Context, *domainsub2key.Binding) error {
	r.upserted = true
	return nil
}
func (r *reclaimingKeyRepo) CompleteSub2KeyBindingOperation(context.Context, uint, string, string) error {
	r.completed = true
	return nil
}

func (r *syncKeyRepo) ListSub2KeyBindings(context.Context, uint) ([]domainsub2key.Binding, error) {
	return r.items, nil
}
func (r *syncKeyRepo) MarkSub2KeyBindingUnavailable(_ context.Context, _ uint, remoteKeyID int64, _ time.Time) error {
	r.unavailable = append(r.unavailable, remoteKeyID)
	return nil
}
func (r *syncKeyRepo) UpsertSub2KeyBinding(_ context.Context, item *domainsub2key.Binding) error {
	r.upserts = append(r.upserts, *item)
	return nil
}

func (r *countingKeyRepo) GetSub2KeyBinding(context.Context, uint, string) (*domainsub2key.Binding, error) {
	r.getCalls++
	return nil, repository.ErrNotFound
}
func (r *countingKeyRepo) RevokeSub2KeyBinding(context.Context, uint, string) error {
	r.revokeCalls++
	return nil
}

func TestViewsExcludeSecrets(t *testing.T) {
	binding := domainsub2key.Binding{PublicID: "sub2_0123456789abcdef0123456789abcdef", Ciphertext: "cipher-secret", Fingerprint: "fingerprint-secret", MaskedKey: "sk-t...test", UsedQuota: 2}
	remote := sub2api.APIKey{ID: 1, Key: "raw-secret-key", QuotaUsed: 3}
	for _, value := range []any{toView(binding), remoteView(remote, "")} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		if strings.Contains(text, "secret") || strings.Contains(text, "Ciphertext") || strings.Contains(text, "Fingerprint") {
			t.Fatalf("view leaks a secret: %s", text)
		}
	}
}

func TestSynchronizeFiltersDeletedRemoteBindings(t *testing.T) {
	repo := &syncKeyRepo{items: []domainsub2key.Binding{
		{PublicID: "sub2_0123456789abcdef0123456789abcdef", PrincipalID: 1, RemoteKeyID: 1, Status: "active", Ciphertext: "ciphertext", Fingerprint: fingerprint("key", "sk-live-key"), Version: 1},
		{PublicID: "sub2_abcdef0123456789abcdef0123456789", PrincipalID: 1, RemoteKeyID: 2, Status: "active", Ciphertext: "ciphertext", Fingerprint: fingerprint("key", "sk-deleted-key"), Version: 1},
	}}
	service := NewService(repo, keyTestTokens{}, nil, "key")
	items, err := service.synchronize(context.Background(), 1, 7, repo.items, []sub2api.APIKey{{ID: 1, UserID: 7, Key: "sk-live-key", Status: "active"}})
	if err != nil {
		t.Fatalf("synchronize() error = %v", err)
	}
	if len(items) != 1 || items[0].RemoteKeyID != 1 {
		t.Fatalf("usable bindings = %#v", items)
	}
	if len(repo.unavailable) != 1 || repo.unavailable[0] != 2 {
		t.Fatalf("unavailable bindings = %#v", repo.unavailable)
	}
	if len(repo.upserts) != 1 || repo.upserts[0].RemoteKeyID != 1 {
		t.Fatalf("refreshed bindings = %#v", repo.upserts)
	}
}

func TestBindLetsRepositoryReclaimPendingOperation(t *testing.T) {
	const (
		idempotencyKey = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
		remoteKeyID    = int64(9)
	)
	repo := &reclaimingKeyRepo{operation: domainsub2key.BindingOperation{
		PrincipalID:    1,
		IdempotencyKey: idempotencyKey,
		RequestHash:    bindRequestHash(remoteKeyID),
		State:          "pending",
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/profile":
			writeKeyEnvelope(w, map[string]any{"id": 7})
		case "/api/v1/keys":
			writeKeyEnvelope(w, map[string]any{"total": 1, "items": []any{map[string]any{
				"id": remoteKeyID, "user_id": 7, "name": "default", "key": "sk-test-key", "status": "active",
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, keyTestTokens{}, client, "test-encryption-key")
	view, err := service.Bind(context.Background(), 1, "session", remoteKeyID, idempotencyKey)
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if view == nil || view.RemoteKeyID != remoteKeyID || repo.createCalls != 1 || !repo.upserted || !repo.completed {
		t.Fatalf("reclaim result = %#v, repo = %#v", view, repo)
	}
}
func TestMalformedBindingPublicIDShortCircuitsRepositoryAndNetwork(t *testing.T) {
	repo := &countingKeyRepo{}
	service := NewService(repo, keyTestTokens{}, nil, "key")
	for _, value := range []string{"", "sub2_0123456789abcdef0123456789abcde", "sub2_0123456789abcdef0123456789abcdeg", "sub2_0123456789ABCDEF0123456789abcdef"} {
		if err := service.Delete(context.Background(), 1, value); err != ErrInvalidBinding {
			t.Fatalf("Delete(%q) error = %v", value, err)
		}
	}
	if repo.getCalls != 0 || repo.revokeCalls != 0 {
		t.Fatalf("repository called: get=%d revoke=%d", repo.getCalls, repo.revokeCalls)
	}
}
