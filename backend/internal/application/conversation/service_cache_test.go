package conversation

import (
	"testing"
	"time"
)

func TestCleanupExpiredInMemoryCaches(t *testing.T) {
	svc := &Service{}
	now := time.Now()

	svc.snapshotCache.Store(uint(1), &cachedSnapshot{expiresAt: now.Add(-time.Second)})
	svc.snapshotCache.Store(uint(2), &cachedSnapshot{expiresAt: now.Add(time.Minute)})
	svc.userMemCache.Store(uint(1), &cachedUserMemories{expiresAt: now.Add(-time.Second)})
	svc.userMemCache.Store(uint(2), &cachedUserMemories{expiresAt: now.Add(time.Minute)})
	svc.userSettingCache.Store("1:stale", &cachedUserSetting{expiresAt: now.Add(-time.Second)})
	svc.userSettingCache.Store("1:fresh", &cachedUserSetting{expiresAt: now.Add(time.Minute)})

	svc.cleanupExpiredInMemoryCaches(now)

	if _, ok := svc.snapshotCache.Load(uint(1)); ok {
		t.Fatal("expected expired snapshot cache entry to be deleted")
	}
	if _, ok := svc.snapshotCache.Load(uint(2)); !ok {
		t.Fatal("expected fresh snapshot cache entry to remain")
	}
	if _, ok := svc.userMemCache.Load(uint(1)); ok {
		t.Fatal("expected expired user memory cache entry to be deleted")
	}
	if _, ok := svc.userMemCache.Load(uint(2)); !ok {
		t.Fatal("expected fresh user memory cache entry to remain")
	}
	if _, ok := svc.userSettingCache.Load("1:stale"); ok {
		t.Fatal("expected expired user setting cache entry to be deleted")
	}
	if _, ok := svc.userSettingCache.Load("1:fresh"); !ok {
		t.Fatal("expected fresh user setting cache entry to remain")
	}
}

func TestInvalidateUserSettingCache(t *testing.T) {
	svc := &Service{}
	expiresAt := time.Now().Add(time.Minute)
	svc.userSettingCache.Store("1:chat.content_width", &cachedUserSetting{value: "wide", valid: true, expiresAt: expiresAt})
	svc.userSettingCache.Store("1:chat.file_mode", &cachedUserSetting{value: "auto", valid: true, expiresAt: expiresAt})
	svc.userSettingCache.Store("2:chat.content_width", &cachedUserSetting{value: "compact", valid: true, expiresAt: expiresAt})

	svc.InvalidateUserSettingCache(1)

	for _, key := range []string{"1:chat.content_width", "1:chat.file_mode"} {
		if _, ok := svc.userSettingCache.Load(key); ok {
			t.Fatalf("expected %s to be invalidated", key)
		}
	}
	if _, ok := svc.userSettingCache.Load("2:chat.content_width"); !ok {
		t.Fatal("expected another user's cache entry to remain")
	}
}
