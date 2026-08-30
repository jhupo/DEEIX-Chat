package sub2api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
	port "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

var ErrNoConnector = port.ErrConnectorUnavailable

const maxCachedClients = 64

type Registry struct {
	connectors connectorResolver
	policy     sharedsecurity.OutboundPolicy
	mu         sync.Mutex
	cache      map[string]cachedClient
	cacheClock uint64
}

type connectorResolver interface {
	GetConnectorByHostname(context.Context, string) (*domainrelay.Connector, error)
}

type cachedClient struct {
	accountBaseURL string
	client         *Client
	lastUsed       uint64
}

func NewRegistry(connectors connectorResolver, policy sharedsecurity.OutboundPolicy) *Registry {
	return &Registry{connectors: connectors, policy: policy, cache: make(map[string]cachedClient)}
}

func (r *Registry) InstanceID() string { return "registry" }

func (r *Registry) InstanceIDForContext(ctx context.Context) string {
	connector, err := r.connector(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(connector.PublicID)
}

func (r *Registry) ModelBaseURL(ctx context.Context) string {
	connector, err := r.connector(ctx)
	if err != nil {
		return ""
	}
	return connector.ModelBaseURL
}

func (r *Registry) client(ctx context.Context) (*Client, error) {
	connector, err := r.connector(ctx)
	if err != nil {
		return nil, err
	}
	if connector.Protocol != "sub2api" {
		return nil, fmt.Errorf("relay protocol %q is not implemented", connector.Protocol)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheClock++
	if cached, ok := r.cache[connector.PublicID]; ok && cached.accountBaseURL == connector.AccountBaseURL {
		cached.lastUsed = r.cacheClock
		r.cache[connector.PublicID] = cached
		return cached.client, nil
	}
	if cached, ok := r.cache[connector.PublicID]; ok {
		cached.client.CloseIdleConnections()
		delete(r.cache, connector.PublicID)
	}
	connectorPolicy, err := r.policy.WithTrustedHTTPURLs(connector.AccountBaseURL)
	if err != nil {
		return nil, err
	}
	client, err := New(connector.AccountBaseURL, connectorPolicy)
	if err != nil {
		return nil, err
	}
	if len(r.cache) >= maxCachedClients {
		oldestID := ""
		oldestUse := ^uint64(0)
		for publicID, cached := range r.cache {
			if cached.lastUsed < oldestUse {
				oldestID, oldestUse = publicID, cached.lastUsed
			}
		}
		if cached, ok := r.cache[oldestID]; ok {
			cached.client.CloseIdleConnections()
			delete(r.cache, oldestID)
		}
	}
	r.cache[connector.PublicID] = cachedClient{accountBaseURL: connector.AccountBaseURL, client: client, lastUsed: r.cacheClock}
	return client, nil
}

func (r *Registry) connector(ctx context.Context) (port.ConnectorEndpoint, error) {
	if r == nil || r.connectors == nil {
		return port.ConnectorEndpoint{}, ErrNoConnector
	}
	return port.ResolveRequestConnector(ctx, func(resolveCtx context.Context, host string) (port.ConnectorEndpoint, error) {
		connector, err := r.connectors.GetConnectorByHostname(resolveCtx, host)
		if err != nil || connector == nil || !connector.Enabled || strings.TrimSpace(connector.AccountBaseURL) == "" || strings.TrimSpace(connector.ModelBaseURL) == "" {
			return port.ConnectorEndpoint{}, ErrNoConnector
		}
		return port.ConnectorEndpoint{
			PublicID: connector.PublicID, Protocol: connector.Protocol,
			AccountBaseURL: connector.AccountBaseURL, ModelBaseURL: connector.ModelBaseURL,
		}, nil
	})
}

func (r *Registry) Login(ctx context.Context, email, password, turnstile string) (*LoginResult, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Login(ctx, email, password, turnstile)
}
func (r *Registry) VerifyLogin2FA(ctx context.Context, temp, code string) (*TokenPair, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.VerifyLogin2FA(ctx, temp, code)
}
func (r *Registry) SendRegistrationCode(ctx context.Context, email, turnstile string) (*VerificationCodeResult, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.SendRegistrationCode(ctx, email, turnstile)
}
func (r *Registry) Register(ctx context.Context, email, password, code, turnstile string) (*TokenPair, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Register(ctx, email, password, code, turnstile)
}
func (r *Registry) Refresh(ctx context.Context, refresh string) (*TokenPair, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Refresh(ctx, refresh)
}
func (r *Registry) Logout(ctx context.Context, refresh string) error {
	c, e := r.client(ctx)
	if e != nil {
		return e
	}
	return c.Logout(ctx, refresh)
}
func (r *Registry) ChangePassword(ctx context.Context, access, current, next string) error {
	c, e := r.client(ctx)
	if e != nil {
		return e
	}
	return c.ChangePassword(ctx, access, current, next)
}
func (r *Registry) Me(ctx context.Context, access string) (*User, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Me(ctx, access)
}
func (r *Registry) UserProfile(ctx context.Context, access string) (*UserProfile, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.UserProfile(ctx, access)
}
func (r *Registry) ListAPIKeys(ctx context.Context, access string, page, size int) (*APIKeyPage, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.ListAPIKeys(ctx, access, page, size)
}
func (r *Registry) AvailableGroups(ctx context.Context, access string) ([]AvailableGroup, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.AvailableGroups(ctx, access)
}
func (r *Registry) CreateAPIKey(ctx context.Context, access string, input CreateAPIKeyInput, key string) (*APIKey, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.CreateAPIKey(ctx, access, input, key)
}
func (r *Registry) Announcements(ctx context.Context, access string) ([]Announcement, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Announcements(ctx, access)
}
func (r *Registry) MarkAnnouncementRead(ctx context.Context, access string, id int64) error {
	c, e := r.client(ctx)
	if e != nil {
		return e
	}
	return c.MarkAnnouncementRead(ctx, access, id)
}
func (r *Registry) CheckoutInfo(ctx context.Context, access string) (CheckoutInfo, error) {
	c, e := r.client(ctx)
	if e != nil {
		return CheckoutInfo{}, e
	}
	return c.CheckoutInfo(ctx, access)
}
func (r *Registry) Redeem(ctx context.Context, access, code string) (*RedeemResult, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Redeem(ctx, access, code)
}
func (r *Registry) CreatePaymentOrder(ctx context.Context, access string, input CreatePaymentOrderInput) (*CreatePaymentOrderResult, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.CreatePaymentOrder(ctx, access, input)
}
func (r *Registry) PaymentOrders(ctx context.Context, access string, input PaymentOrderQuery) (*PaymentOrderPage, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.PaymentOrders(ctx, access, input)
}
func (r *Registry) PaymentOrder(ctx context.Context, access string, id int64) (*PaymentOrder, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.PaymentOrder(ctx, access, id)
}
func (r *Registry) VerifyPaymentOrder(ctx context.Context, access, order string) (*PaymentOrder, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.VerifyPaymentOrder(ctx, access, order)
}
func (r *Registry) CancelPaymentOrder(ctx context.Context, access string, id int64) error {
	c, e := r.client(ctx)
	if e != nil {
		return e
	}
	return c.CancelPaymentOrder(ctx, access, id)
}
func (r *Registry) RequestPaymentRefund(ctx context.Context, access string, id int64, reason string) error {
	c, e := r.client(ctx)
	if e != nil {
		return e
	}
	return c.RequestPaymentRefund(ctx, access, id, reason)
}
func (r *Registry) RedemptionHistory(ctx context.Context, access string) ([]RedemptionHistoryItem, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.RedemptionHistory(ctx, access)
}
func (r *Registry) PaymentPlans(ctx context.Context, access string) ([]PaymentPlan, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.PaymentPlans(ctx, access)
}
func (r *Registry) ActiveSubscription(ctx context.Context, access string) (json.RawMessage, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.ActiveSubscription(ctx, access)
}
func (r *Registry) ActiveSubscriptions(ctx context.Context, access string) ([]Subscription, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.ActiveSubscriptions(ctx, access)
}
func (r *Registry) Usage(ctx context.Context, access string, input UsageQuery) (*UsagePage, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Usage(ctx, access, input)
}
func (r *Registry) Trend(ctx context.Context, access, start, end, granularity string) (*UsageTrend, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Trend(ctx, access, start, end, granularity)
}
func (r *Registry) Settings(ctx context.Context) (*PublicSettings, error) {
	c, e := r.client(ctx)
	if e != nil {
		return nil, e
	}
	return c.Settings(ctx)
}
