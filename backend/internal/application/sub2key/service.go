package sub2key

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	domainsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/sub2key"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/secretbox"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

var (
	ErrInvalidBinding      = errors.New("invalid Sub2 key binding")
	ErrBindingUnavailable  = errors.New("Sub2 key binding unavailable")
	ErrIdempotencyConflict = errors.New("Sub2 key binding idempotency conflict")
)

const (
	remoteKeyPageSize = 100
	maxRemoteKeys     = 10_000
	remoteKeyCacheTTL = time.Minute
	remoteKeyCacheMax = 256
)

type TokenResolver interface {
	Sub2AccessTokenForSession(context.Context, uint, string) (string, error)
}

type UserTokenResolver interface {
	Sub2AccessTokensForUser(context.Context, uint) ([]string, error)
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
type GroupView struct {
	ID          int64
	Name        string
	Description string
	Platform    string
}
type Execution struct {
	BindingPublicID string
	BindingVersion  uint
	RemoteKeyID     int64
	APIKey          string
	GroupPlatform   string
}

type Service struct {
	repo          repository.Sub2KeyBindingRepository
	tokens        TokenResolver
	client        sub2api.Client
	encryptionKey string
	remoteCacheMu sync.Mutex
	remoteCache   map[string]remoteKeyCacheEntry
	remoteGroup   singleflight.Group
	groupCacheMu  sync.Mutex
	groupCache    map[string]groupCacheEntry
	groupGroup    singleflight.Group
}

type remoteKeyCacheEntry struct {
	profile   sub2api.UserProfile
	keys      []sub2api.APIKey
	expiresAt time.Time
}

type groupCacheEntry struct {
	groups    []sub2api.AvailableGroup
	expiresAt time.Time
}

func NewService(repo repository.Sub2KeyBindingRepository, tokens TokenResolver, client sub2api.Client, encryptionKey string) *Service {
	return &Service{
		repo: repo, tokens: tokens, client: client, encryptionKey: encryptionKey,
		remoteCache: make(map[string]remoteKeyCacheEntry), groupCache: make(map[string]groupCacheEntry),
	}
}

// MatchRuntimeProof validates a proof against live keys from the current relay
// instance. Raw keys and the proof are kept in this call only.
func (s *Service) MatchRuntimeProof(
	ctx context.Context,
	userID uint,
	expectedRemoteUserID int64,
	challenge []byte,
	proof []byte,
) (int64, string, error) {
	resolver, ok := s.tokens.(UserTokenResolver)
	if !ok || userID == 0 || expectedRemoteUserID <= 0 || len(challenge) == 0 || len(proof) != sha256.Size {
		return 0, "", ErrInvalidBinding
	}
	tokens, err := resolver.Sub2AccessTokensForUser(ctx, userID)
	if err != nil {
		return 0, "", err
	}
	matchedID := int64(0)
	for _, token := range tokens {
		keys, keysErr := s.remoteKeys(ctx, token, expectedRemoteUserID)
		if keysErr != nil {
			continue
		}
		for i := range keys {
			if !runtimeAuthenticatable(keys[i], time.Now()) || strings.TrimSpace(keys[i].Key) == "" {
				continue
			}
			mac := hmac.New(sha256.New, []byte(keys[i].Key))
			_, _ = mac.Write(challenge)
			if hmac.Equal(mac.Sum(nil), proof) {
				if matchedID != 0 && matchedID != keys[i].ID {
					return 0, "", ErrInvalidBinding
				}
				matchedID = keys[i].ID
			}
		}
	}
	if matchedID == 0 {
		return 0, "", ErrBindingUnavailable
	}
	return matchedID, runtimeCredentialFingerprint(s.encryptionKey, matchedID), nil
}

func runtimeCredentialFingerprint(secret string, remoteKeyID int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "deeix-runtime-key-v1\n%d", remoteKeyID)
	return hex.EncodeToString(mac.Sum(nil))
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
func (s *Service) ListGroups(ctx context.Context, principalID uint, sessionID string) ([]GroupView, error) {
	groups, err := s.currentGroups(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupView, 0, len(groups))
	for _, group := range groups {
		if group.ID <= 0 || strings.TrimSpace(group.Name) == "" || strings.TrimSpace(group.Platform) == "" {
			return nil, ErrInvalidBinding
		}
		out = append(out, GroupView{ID: group.ID, Name: group.Name, Description: group.Description, Platform: group.Platform})
	}
	return out, nil
}

func (s *Service) currentGroups(ctx context.Context, principalID uint, sessionID string) ([]sub2api.AvailableGroup, error) {
	cacheKey := fmt.Sprintf("%d:%s", principalID, strings.TrimSpace(sessionID))
	now := time.Now()
	s.groupCacheMu.Lock()
	if entry, ok := s.groupCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		groups := append([]sub2api.AvailableGroup(nil), entry.groups...)
		s.groupCacheMu.Unlock()
		return groups, nil
	}
	delete(s.groupCache, cacheKey)
	s.groupCacheMu.Unlock()

	value, err, _ := s.groupGroup.Do(cacheKey, func() (any, error) {
		token, tokenErr := s.tokens.Sub2AccessTokenForSession(ctx, principalID, sessionID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		groups, loadErr := s.client.AvailableGroups(ctx, token)
		if loadErr != nil {
			return nil, loadErr
		}
		s.groupCacheMu.Lock()
		for len(s.groupCache) >= remoteKeyCacheMax {
			for key := range s.groupCache {
				delete(s.groupCache, key)
				break
			}
		}
		s.groupCache[cacheKey] = groupCacheEntry{groups: append([]sub2api.AvailableGroup(nil), groups...), expiresAt: time.Now().Add(remoteKeyCacheTTL)}
		s.groupCacheMu.Unlock()
		return groups, nil
	})
	if err != nil {
		return nil, err
	}
	return append([]sub2api.AvailableGroup(nil), value.([]sub2api.AvailableGroup)...), nil
}
func (s *Service) CreateRemote(ctx context.Context, principalID uint, sessionID, name string, groupID int64, idempotencyKey string) (*RemoteKeyView, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || groupID <= 0 || !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidBinding
	}
	profile, _, err := s.currentRemoteKeys(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	groups, err := s.currentGroups(ctx, principalID, sessionID)
	if err != nil {
		return nil, err
	}
	groupAllowed := false
	for _, group := range groups {
		if group.ID == groupID {
			groupAllowed = true
			break
		}
	}
	if !groupAllowed {
		return nil, ErrInvalidBinding
	}
	created, err := s.client.CreateAPIKey(ctx, token, sub2api.CreateAPIKeyInput{Name: name, GroupID: groupID}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if created == nil || created.ID <= 0 || created.UserID != profile.ID || created.GroupID == nil || *created.GroupID != groupID || strings.TrimSpace(created.Key) == "" {
		return nil, ErrInvalidBinding
	}
	s.invalidateRemoteCache(principalID, sessionID)
	view := remoteView(*created, "")
	return &view, nil
}
func (s *Service) Bind(ctx context.Context, principalID uint, sessionID string, remoteID int64, idempotencyKey string) (*BindingView, error) {
	if remoteID <= 0 || !validIdempotencyKey(idempotencyKey) {
		return nil, ErrInvalidBinding
	}
	requestHash := bindRequestHash(remoteID)
	if replay, err := s.replayBinding(ctx, principalID, idempotencyKey, requestHash); err != nil || replay != nil {
		return replay, err
	}
	profile, keys, err := s.currentRemoteKeys(ctx, principalID, sessionID)
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
	return &Execution{BindingPublicID: item.PublicID, BindingVersion: item.Version, RemoteKeyID: item.RemoteKeyID, APIKey: key, GroupPlatform: item.Platform}, nil
}

func (s *Service) invalidateRemoteCache(principalID uint, sessionID string) {
	cacheKey := fmt.Sprintf("%d:%s", principalID, strings.TrimSpace(sessionID))
	s.remoteCacheMu.Lock()
	delete(s.remoteCache, cacheKey)
	s.remoteCacheMu.Unlock()
	s.groupCacheMu.Lock()
	delete(s.groupCache, cacheKey)
	s.groupCacheMu.Unlock()
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
	cacheKey := fmt.Sprintf("%d:%s", principalID, strings.TrimSpace(sessionID))
	now := time.Now()
	s.remoteCacheMu.Lock()
	s.pruneRemoteCacheLocked(now, 0)
	entry, found := s.remoteCache[cacheKey]
	if found {
		profile := entry.profile
		keys := append([]sub2api.APIKey(nil), entry.keys...)
		s.remoteCacheMu.Unlock()
		return &profile, keys, nil
	}
	s.remoteCacheMu.Unlock()

	value, err, _ := s.remoteGroup.Do(cacheKey, func() (any, error) {
		now := time.Now()
		s.remoteCacheMu.Lock()
		if cached, ok := s.remoteCache[cacheKey]; ok && now.Before(cached.expiresAt) {
			copy := cached
			copy.keys = append([]sub2api.APIKey(nil), cached.keys...)
			s.remoteCacheMu.Unlock()
			return copy, nil
		}
		s.remoteCacheMu.Unlock()
		token, tokenErr := s.tokens.Sub2AccessTokenForSession(ctx, principalID, sessionID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		profile, profileErr := s.client.UserProfile(ctx, token)
		if profileErr != nil {
			return nil, profileErr
		}
		keys, keysErr := s.remoteKeys(ctx, token, profile.ID)
		if keysErr != nil {
			return nil, keysErr
		}
		loaded := remoteKeyCacheEntry{profile: *profile, keys: append([]sub2api.APIKey(nil), keys...), expiresAt: time.Now().Add(remoteKeyCacheTTL)}
		s.remoteCacheMu.Lock()
		reserve := 1
		if _, exists := s.remoteCache[cacheKey]; exists {
			reserve = 0
		}
		s.pruneRemoteCacheLocked(time.Now(), reserve)
		s.remoteCache[cacheKey] = loaded
		s.remoteCacheMu.Unlock()
		return loaded, nil
	})
	if err != nil {
		return nil, nil, err
	}
	loaded := value.(remoteKeyCacheEntry)
	profile := &loaded.profile
	return profile, append([]sub2api.APIKey(nil), loaded.keys...), nil
}

func (s *Service) pruneRemoteCacheLocked(now time.Time, reserve int) {
	for key, entry := range s.remoteCache {
		if !now.Before(entry.expiresAt) {
			delete(s.remoteCache, key)
		}
	}
	for len(s.remoteCache)+reserve > remoteKeyCacheMax {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range s.remoteCache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.remoteCache, oldestKey)
	}
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
func runtimeAuthenticatable(k sub2api.APIKey, now time.Time) bool {
	switch strings.ToLower(strings.TrimSpace(k.Status)) {
	case "expired", "quota_exhausted":
		return true
	default:
		return active(k, now)
	}
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
