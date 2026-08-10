package app

import (
	"context"
	"time"

	rediscache "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/cache/redis"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	postgresdb "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	platformhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

func openDatabase(cfg config.Config) (*gorm.DB, error) {
	return postgresdb.New(cfg)
}

func openCache(cfg config.Config) (*redis.Client, error) {
	return rediscache.NewRedis(cfg)
}

func buildSettingsCache(redisClient *redis.Client) repository.SettingsCacheRepository {
	return rediscache.NewSettingsCache(redisClient)
}

func buildChannelCache(redisClient *redis.Client) repository.ChannelCacheRepository {
	return rediscache.NewChannelCache(redisClient)
}

func buildConversationCache(redisClient *redis.Client) repository.ConversationCacheRepository {
	return rediscache.NewConversationCache(redisClient)
}

func buildRateLimiter(redisClient *redis.Client) middleware.RateLimiter {
	return rediscache.NewRateLimiter(redisClient)
}

func buildProviderAuthBridge(redisClient *redis.Client) repository.ProviderAuthBridgeRepository {
	return rediscache.NewProviderAuthBridge(redisClient)
}

type healthChecker struct {
	db    *gorm.DB
	redis *redis.Client
}

func newHealthChecker(db *gorm.DB, redisClient *redis.Client) platformhttp.HealthChecker {
	return &healthChecker{db: db, redis: redisClient}
}

func (h *healthChecker) CheckHealth(ctx context.Context) ([]platformhttp.HealthCheck, bool) {
	checks := make([]platformhttp.HealthCheck, 0, 2)
	healthy := true

	if h.db == nil {
		checks = append(checks, platformhttp.HealthCheck{Name: "db", Status: "not_configured"})
		healthy = false
	} else if sqlDB, err := h.db.DB(); err != nil {
		checks = append(checks, platformhttp.HealthCheck{Name: "db", Status: "error: " + err.Error()})
		healthy = false
	} else {
		dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = sqlDB.PingContext(dbCtx)
		cancel()
		if err != nil {
			checks = append(checks, platformhttp.HealthCheck{Name: "db", Status: "error"})
			healthy = false
		} else {
			checks = append(checks, platformhttp.HealthCheck{Name: "db", Status: "ok"})
		}
	}

	redisCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if h.redis == nil {
		checks = append(checks, platformhttp.HealthCheck{Name: "redis", Status: "not_configured"})
		healthy = false
	} else if err := h.redis.Ping(redisCtx).Err(); err != nil {
		checks = append(checks, platformhttp.HealthCheck{Name: "redis", Status: "error"})
		healthy = false
	} else {
		checks = append(checks, platformhttp.HealthCheck{Name: "redis", Status: "ok"})
	}

	return checks, healthy
}
