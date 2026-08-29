package channel

import (
	"context"
	"fmt"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type channelTestCache struct {
	upstreamOpen  map[uint]bool
	modelOpen     map[string]bool
	modelFailures map[string]int
}

func newChannelTestCache() *channelTestCache {
	return &channelTestCache{
		upstreamOpen:  make(map[uint]bool),
		modelOpen:     make(map[string]bool),
		modelFailures: make(map[string]int),
	}
}

func (c *channelTestCache) CheckUpstreamCircuitState(_ context.Context, upstreamID uint) (string, error) {
	if c.upstreamOpen[upstreamID] {
		return "open", nil
	}
	return "closed", nil
}

func (c *channelTestCache) CheckModelCircuitState(_ context.Context, upstreamID uint, modelKey string) (string, error) {
	if c.modelOpen[channelTestModelCircuitKey(upstreamID, modelKey)] {
		return "open", nil
	}
	return "closed", nil
}

func (c *channelTestCache) RecordCircuitFailure(_ context.Context, input repository.CircuitFailureInput) error {
	key := channelTestModelCircuitKey(input.UpstreamID, input.ModelKey)
	c.modelFailures[key]++
	if input.ModelFailureThreshold > 0 && c.modelFailures[key] >= input.ModelFailureThreshold {
		c.modelOpen[key] = true
	}
	return nil
}

func (c *channelTestCache) RecordFailureMetadata(context.Context, uint, string) {}

func (c *channelTestCache) RecordSuccessMetadata(context.Context, uint) {}

func (c *channelTestCache) ClearUpstreamCircuitKeys(_ context.Context, upstreamID uint) error {
	delete(c.upstreamOpen, upstreamID)
	return nil
}

func (c *channelTestCache) ClearModelCircuitKeys(_ context.Context, upstreamID uint, modelKey string) error {
	key := channelTestModelCircuitKey(upstreamID, modelKey)
	delete(c.modelOpen, key)
	delete(c.modelFailures, key)
	return nil
}

func (c *channelTestCache) ResetAllCircuitStates(context.Context) error {
	clear(c.upstreamOpen)
	clear(c.modelOpen)
	clear(c.modelFailures)
	return nil
}

func (c *channelTestCache) ReleaseRouteProbes(context.Context, uint, string) error {
	return nil
}

func (c *channelTestCache) OpenUpstreamCircuit(_ context.Context, upstreamID uint) error {
	c.upstreamOpen[upstreamID] = true
	return nil
}

func (c *channelTestCache) ResetUpstreamCircuit(_ context.Context, upstreamID uint) error {
	delete(c.upstreamOpen, upstreamID)
	return nil
}

func (c *channelTestCache) OpenModelCircuit(_ context.Context, upstreamID uint, modelKey string) error {
	c.modelOpen[channelTestModelCircuitKey(upstreamID, modelKey)] = true
	return nil
}

func (c *channelTestCache) ResetModelCircuit(_ context.Context, upstreamID uint, modelKey string) error {
	key := channelTestModelCircuitKey(upstreamID, modelKey)
	delete(c.modelOpen, key)
	delete(c.modelFailures, key)
	return nil
}

func (c *channelTestCache) QueryUpstreamCircuitStatus(_ context.Context, upstreamID uint) (bool, string) {
	return c.upstreamOpen[upstreamID], ""
}

func (c *channelTestCache) QueryModelCircuitStatus(_ context.Context, upstreamID uint, modelKey string) (bool, string) {
	return c.modelOpen[channelTestModelCircuitKey(upstreamID, modelKey)], ""
}

func (c *channelTestCache) GetRateLimitBackoff(context.Context, uint, uint) (time.Duration, error) {
	return 0, nil
}

func (c *channelTestCache) RecordRateLimitBackoff(context.Context, repository.RateLimitBackoffParams) error {
	return nil
}

func (c *channelTestCache) ClearRateLimitBackoff(context.Context, uint, uint) error {
	return nil
}

func (c *channelTestCache) IncrAPIKeyCounter(context.Context, uint) (int64, bool) {
	return 0, true
}

func channelTestModelCircuitKey(upstreamID uint, modelKey string) string {
	return fmt.Sprintf("%d:%s", upstreamID, modelKey)
}

var _ repository.ChannelCacheRepository = (*channelTestCache)(nil)
