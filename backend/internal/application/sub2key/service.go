package sub2key

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	domainsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/sub2key"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/secretbox"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrInvalidBinding      = errors.New("invalid Sub2 key binding")
	ErrBindingUnavailable  = errors.New("Sub2 key binding unavailable")
	ErrIdempotencyConflict = errors.New("Sub2 key binding idempotency conflict")
)

const (
	remoteKeyPageSize = 100
	maxRemoteKeys     = 10_000
)

type TokenResolver interface {
	Sub2AccessTokenForSession(context.Context, uint, string) (string, error)
}

type BindingView struct {
	PublicID        string
	RemoteKeyID     int64
	Label           string
	MaskedKey       string
	GroupID         *int64
	GroupName       string
	GroupPlatform   string
	Status          string
	Quota           float64
	UsedQuota       float64
	ExpiresAt       *time.Time
	Version         uint
	LastValidatedAt *time.Time
}
type RemoteKeyView struct {
	RemoteKeyID     int64
	Label           string
	MaskedKey       string
	GroupID         *int64
	GroupName       string
	GroupPlatform   string
	Status          string
	Quota           float64
	UsedQuota       float64
	ExpiresAt       *time.Time
	Bound           bool
	BindingPublicID *string
}
type ModelView struct {
	ID          string
	DisplayName string
}
type Execution struct {
	BindingPublicID string
	BindingVersion  uint
	RemoteKeyID     int64
	APIKey          string
	Model           string
}

type Service struct {
	repo          repository.Sub2KeyBindingRepository
	tokens        TokenResolver
	client        *sub2api.Client
	encryptionKey string
}

func NewService(repo repository.Sub2KeyBindingRepository, tokens TokenResolver, client *sub2api.Client, encryptionKey string) *Service {
	return &Service{repo: repo, tokens: tokens, client: client, encryptionKey: encryptionKey}
}

func (s *Service) List(ctx context.Context, principalID uint, sessionID string) ([]BindingView, error) {
	items, err := s.synchronizedBindings(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]BindingView, 0, len(items))
	for _, x := range items {
		out = append(out, toView(x))
	}
	return out, nil
}
func (s *Service) ListRemote(ctx context.Context, principalID uint, sessionID string) ([]RemoteKeyView, error) {
	profile, keys, err := s.currentRemoteKeys(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	local, err := s.repo.ListSub2KeyBindings(ctx, principalID)
	if err != nil {
		return nil, err
	}
	local, err = s.synchronize(ctx, principalID, profile.ID, local, keys)
	if err != nil {
		return nil, err
	}
	bound := map[int64]string{}
	for _, x := range local {
		bound[x.RemoteKeyID] = x.PublicID
	}
	out := make([]RemoteKeyView, 0, len(keys))
	for _, key := range keys {
		if active(key, time.Now()) && strings.TrimSpace(key.Key) != "" {
			out = append(out, remoteView(key, bound[key.ID]))
		}
	}
	return out, nil
}
func (s *Service) Bind(ctx context.Context, principalID uint, sessionID string, remoteID int64, idempotencyKey string) (*BindingView, error) {
	if remoteID <= 0 || !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidBinding
	}
	requestHash := bindRequestHash(remoteID)
	if replay, err := s.replayBinding(ctx, principalID, idempotencyKey, requestHash); err != nil || replay != nil {
		return replay, err
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	profile, err := s.client.UserProfile(ctx, token)
	if err != nil {
		return nil, err
	}
	keys, err := s.remoteKeys(ctx, token, profile.ID)
	if err != nil {
		return nil, err
	}
	var found *sub2api.APIKey
	for i := range keys {
		if keys[i].ID == remoteID {
			found = &keys[i]
			break
		}
	}
	if found == nil || strings.TrimSpace(found.Key) == "" || !active(*found, time.Now()) {
		return nil, ErrBindingUnavailable
	}
	cipher, err := secretbox.EncryptString(s.encryptionKey, found.Key)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetSub2KeyBindingByRemoteKeyID(ctx, principalID, remoteID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	item := domainsub2key.Binding{PublicID: newPublicID(), PrincipalID: principalID, Sub2AccountID: profile.ID, RemoteKeyID: found.ID, Ciphertext: cipher, Fingerprint: fingerprint(s.encryptionKey, found.Key), Label: found.Name, MaskedKey: maskKey(found.Key), GroupID: found.GroupID, Status: found.Status, Quota: found.Quota, UsedQuota: found.QuotaUsed, ExpiresAt: found.ExpiresAt, Version: 1}
	if found.Group != nil {
		item.GroupName = found.Group.Name
		item.Platform = found.Group.Platform
	}
	now := time.Now()
	item.LastValidatedAt = &now
	if existing != nil {
		item.ID = existing.ID
		item.PublicID = existing.PublicID
		item.Version = existing.Version
		if existing.Fingerprint != item.Fingerprint || existing.Label != item.Label || existing.Status != item.Status {
			item.Version++
		}
	}
	created, err := s.repo.CreateSub2KeyBindingOperation(ctx, &domainsub2key.BindingOperation{PrincipalID: principalID, IdempotencyKey: idempotencyKey, RequestHash: requestHash, State: "pending"})
	if err != nil {
		return nil, err
	}
	if !created {
		return s.waitForBindingReplay(ctx, principalID, idempotencyKey, requestHash)
	}
	if err = s.repo.UpsertSub2KeyBinding(ctx, &item); err != nil {
		return nil, err
	}
	if err = s.repo.CompleteSub2KeyBindingOperation(ctx, principalID, idempotencyKey, item.PublicID); err != nil {
		return nil, err
	}
	view := toView(item)
	return &view, nil
}
func (s *Service) Delete(ctx context.Context, principalID uint, publicID string) error {
	if !validBindingPublicID(publicID) {
		return ErrInvalidBinding
	}
	return s.repo.RevokeSub2KeyBinding(ctx, principalID, publicID)
}
func (s *Service) ResolveBinding(ctx context.Context, principalID uint, publicID string) (*Execution, error) {
	if !validBindingPublicID(publicID) {
		return nil, ErrInvalidBinding
	}
	item, err := s.repo.GetSub2KeyBinding(ctx, principalID, strings.TrimSpace(publicID))
	if err != nil {
		return nil, err
	}
	if !activeKey(item, time.Now()) {
		return nil, ErrBindingUnavailable
	}
	key, err := secretbox.DecryptString(s.encryptionKey, item.Ciphertext)
	if err != nil || strings.TrimSpace(key) == "" {
		return nil, ErrBindingUnavailable
	}
	return &Execution{BindingPublicID: item.PublicID, BindingVersion: item.Version, RemoteKeyID: item.RemoteKeyID, APIKey: key}, nil
}
func (s *Service) ValidateModel(ctx context.Context, execution *Execution, requestedModel string) (*Execution, error) {
	if execution == nil || strings.TrimSpace(execution.APIKey) == "" || strings.TrimSpace(requestedModel) == "" {
		return nil, ErrInvalidBinding
	}
	models, err := s.client.GatewayModels(ctx, execution.APIKey)
	if err != nil {
		return nil, err
	}
	for _, candidate := range models.Data {
		if candidate.ID == requestedModel {
			validated := *execution
			validated.Model = candidate.ID
			return &validated, nil
		}
	}
	return nil, ErrBindingUnavailable
}
func (s *Service) Models(ctx context.Context, principalID uint, publicID string) ([]ModelView, error) {
	execution, err := s.ResolveBinding(ctx, principalID, publicID)
	if err != nil {
		return nil, err
	}
	models, err := s.client.GatewayModels(ctx, execution.APIKey)
	if err != nil {
		return nil, err
	}
	views := make([]ModelView, 0, len(models.Data))
	for _, model := range models.Data {
		name := strings.TrimSpace(model.DisplayName)
		if name == "" {
			name = model.ID
		}
		views = append(views, ModelView{ID: model.ID, DisplayName: name})
	}
	return views, nil
}
func (s *Service) remoteKeys(ctx context.Context, token string, expectedUserID int64) ([]sub2api.APIKey, error) {
	keys := make([]sub2api.APIKey, 0)
	total := -1
	for page := 1; ; page++ {
		result, err := s.client.ListAPIKeys(ctx, token, page, remoteKeyPageSize)
		if err != nil {
			return nil, err
		}
		if result.Total < 0 || result.Total > maxRemoteKeys || (total >= 0 && result.Total != total) {
			return nil, ErrInvalidBinding
		}
		total = result.Total
		if len(result.Items) > remoteKeyPageSize || len(keys)+len(result.Items) > total || len(keys)+len(result.Items) > maxRemoteKeys {
			return nil, ErrInvalidBinding
		}
		for _, key := range result.Items {
			if key.UserID != expectedUserID {
				return nil, ErrInvalidBinding
			}
			keys = append(keys, key)
		}
		if len(keys) == total {
			return keys, nil
		}
		if len(result.Items) == 0 {
			return nil, ErrInvalidBinding
		}
	}
}
func (s *Service) currentRemoteKeys(ctx context.Context, principalID uint, sessionID string) (*sub2api.UserProfile, []sub2api.APIKey, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, principalID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	profile, err := s.client.UserProfile(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	keys, err := s.remoteKeys(ctx, token, profile.ID)
	if err != nil {
		return nil, nil, err
	}
	return profile, keys, nil
}

func (s *Service) synchronizedBindings(ctx context.Context, principalID uint, sessionID string) ([]domainsub2key.Binding, error) {
	profile, keys, err := s.currentRemoteKeys(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	local, err := s.repo.ListSub2KeyBindings(ctx, principalID)
	if err != nil {
		return nil, err
	}
	return s.synchronize(ctx, principalID, profile.ID, local, keys)
}

func (s *Service) synchronize(ctx context.Context, principalID uint, accountID int64, local []domainsub2key.Binding, remote []sub2api.APIKey) ([]domainsub2key.Binding, error) {
	remoteByID := make(map[int64]sub2api.APIKey, len(remote))
	for _, key := range remote {
		remoteByID[key.ID] = key
	}
	now := time.Now()
	usable := make([]domainsub2key.Binding, 0, len(local))
	for _, binding := range local {
		key, found := remoteByID[binding.RemoteKeyID]
		if !found || !active(key, now) || strings.TrimSpace(key.Key) == "" {
			if err := s.repo.MarkSub2KeyBindingUnavailable(ctx, principalID, binding.RemoteKeyID, now); err != nil && !errors.Is(err, repository.ErrNotFound) {
				return nil, err
			}
			continue
		}
		refreshed, err := s.bindingFromRemote(binding, accountID, key, now)
		if err != nil {
			return nil, err
		}
		if err := s.repo.UpsertSub2KeyBinding(ctx, &refreshed); err != nil {
			return nil, err
		}
		usable = append(usable, refreshed)
	}
	return usable, nil
}

func (s *Service) bindingFromRemote(existing domainsub2key.Binding, accountID int64, key sub2api.APIKey, now time.Time) (domainsub2key.Binding, error) {
	refreshed := existing
	refreshed.Sub2AccountID = accountID
	refreshed.Label = key.Name
	refreshed.MaskedKey = maskKey(key.Key)
	refreshed.GroupID = key.GroupID
	refreshed.GroupName = ""
	refreshed.Platform = ""
	if key.Group != nil {
		refreshed.GroupName = key.Group.Name
		refreshed.Platform = key.Group.Platform
	}
	refreshed.Status = key.Status
	refreshed.Quota = key.Quota
	refreshed.UsedQuota = key.QuotaUsed
	refreshed.ExpiresAt = key.ExpiresAt
	nextFingerprint := fingerprint(s.encryptionKey, key.Key)
	if refreshed.Fingerprint != nextFingerprint {
		ciphertext, err := secretbox.EncryptString(s.encryptionKey, key.Key)
		if err != nil {
			return domainsub2key.Binding{}, err
		}
		refreshed.Ciphertext = ciphertext
		refreshed.Fingerprint = nextFingerprint
		refreshed.Version++
	}
	refreshed.LastValidatedAt = &now
	return refreshed, nil
}
func (s *Service) replayBinding(ctx context.Context, principalID uint, key, requestHash string) (*BindingView, error) {
	operation, err := s.repo.GetSub2KeyBindingOperation(ctx, principalID, key)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if operation.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	if operation.State != "completed" || strings.TrimSpace(operation.BindingPublicID) == "" {
		return nil, nil
	}
	binding, err := s.repo.GetSub2KeyBinding(ctx, principalID, operation.BindingPublicID)
	if err != nil {
		return nil, err
	}
	view := toView(*binding)
	return &view, nil
}
func (s *Service) waitForBindingReplay(ctx context.Context, principalID uint, key, requestHash string) (*BindingView, error) {
	for attempts := 0; attempts < 20; attempts++ {
		view, err := s.replayBinding(ctx, principalID, key, requestHash)
		if view != nil || err == ErrIdempotencyConflict {
			return view, err
		}
		if err != nil && !errors.Is(err, ErrBindingUnavailable) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return nil, ErrBindingUnavailable
}
func bindRequestHash(remoteID int64) string {
	return fingerprint("sub2-key-binding", fmt.Sprintf("remote:%d", remoteID))
}
func validIdempotencyKey(value string) bool {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil && parsed.String() == strings.ToLower(strings.TrimSpace(value))
}
func validBindingPublicID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != len("sub2_")+32 || !strings.HasPrefix(value, "sub2_") {
		return false
	}
	for _, ch := range value[len("sub2_"):] {
		if !(ch >= '0' && ch <= '9') && !(ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
func active(k sub2api.APIKey, now time.Time) bool {
	return strings.EqualFold(k.Status, "active") && (k.ExpiresAt == nil || k.ExpiresAt.After(now))
}
func activeKey(k *domainsub2key.Binding, now time.Time) bool {
	return k != nil && strings.EqualFold(k.Status, "active") && k.Ciphertext != "" && (k.ExpiresAt == nil || k.ExpiresAt.After(now))
}
func fingerprint(secret, key string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}
func newPublicID() string { return "sub2_" + strings.ReplaceAll(uuid.NewString(), "-", "") }
func maskKey(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
func toView(k domainsub2key.Binding) BindingView {
	return BindingView{PublicID: k.PublicID, RemoteKeyID: k.RemoteKeyID, Label: k.Label, MaskedKey: k.MaskedKey, GroupID: k.GroupID, GroupName: k.GroupName, GroupPlatform: k.Platform, Status: k.Status, Quota: k.Quota, UsedQuota: k.UsedQuota, ExpiresAt: k.ExpiresAt, Version: k.Version, LastValidatedAt: k.LastValidatedAt}
}
func remoteView(k sub2api.APIKey, bindingPublicID string) RemoteKeyView {
	groupName, platform := "", ""
	if k.Group != nil {
		groupName, platform = k.Group.Name, k.Group.Platform
	}
	var bindingID *string
	if bindingPublicID != "" {
		bindingID = &bindingPublicID
	}
	return RemoteKeyView{RemoteKeyID: k.ID, Label: k.Name, MaskedKey: maskKey(k.Key), GroupID: k.GroupID, GroupName: groupName, GroupPlatform: platform, Status: k.Status, Quota: k.Quota, UsedQuota: k.QuotaUsed, ExpiresAt: k.ExpiresAt, BindingPublicID: bindingID, Bound: bindingPublicID != ""}
}
