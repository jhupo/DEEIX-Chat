package sub2api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	platformtracing "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/observability/tracing"
	portsub2api "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

const (
	requestTimeout  = 15 * time.Second
	maxResponseSize = 1 << 20
)

var ErrUnauthorized = portsub2api.ErrUnauthorized

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

type (
	User                     = portsub2api.User
	UserProfile              = portsub2api.UserProfile
	APIKey                   = portsub2api.APIKey
	APIKeyGroup              = portsub2api.APIKeyGroup
	AvailableGroup           = portsub2api.AvailableGroup
	CreateAPIKeyInput        = portsub2api.CreateAPIKeyInput
	APIKeyPage               = portsub2api.APIKeyPage
	Announcement             = portsub2api.Announcement
	CheckoutInfo             = portsub2api.CheckoutInfo
	RedeemResult             = portsub2api.RedeemResult
	CreatePaymentOrderInput  = portsub2api.CreatePaymentOrderInput
	CreatePaymentOrderResult = portsub2api.CreatePaymentOrderResult
	PaymentOrder             = portsub2api.PaymentOrder
	PaymentOrderPage         = portsub2api.PaymentOrderPage
	PaymentOrderQuery        = portsub2api.PaymentOrderQuery
	RedemptionHistoryItem    = portsub2api.RedemptionHistoryItem
	PaymentMethod            = portsub2api.PaymentMethod
	PaymentPlan              = portsub2api.PaymentPlan
	UsageRecord              = portsub2api.UsageRecord
	UsagePage                = portsub2api.UsagePage
	UsageQuery               = portsub2api.UsageQuery
	TrendPoint               = portsub2api.TrendPoint
	UsageTrend               = portsub2api.UsageTrend
	Subscription             = portsub2api.Subscription
	SubscriptionGroup        = portsub2api.SubscriptionGroup
	TokenPair                = portsub2api.TokenPair
	LoginResult              = portsub2api.LoginResult
	PublicSettings           = portsub2api.PublicSettings
	VerificationCodeResult   = portsub2api.VerificationCodeResult
	APIError                 = portsub2api.APIError
)

func New(baseURL string, policy sharedsecurity.OutboundPolicy) (*Client, error) {
	origin, err := sharedsecurity.HTTPOrigin(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Sub2API base URL: %w", err)
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return nil, fmt.Errorf("parse Sub2API base URL: %w", err)
	}
	policy, err = policy.WithTrustedHTTPURLs(origin)
	if err != nil {
		return nil, fmt.Errorf("trust Sub2API base URL: %w", err)
	}
	httpClient := sharedsecurity.NewOutboundHTTPClient(policy, requestTimeout)
	httpClient.Transport = platformtracing.NewHTTPTransport(httpClient.Transport)
	httpClient.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		redirectOrigin, originErr := sharedsecurity.HTTPOrigin(request.URL.String())
		if originErr != nil || redirectOrigin != origin {
			return errors.New("Sub2API redirect changed origin")
		}
		return nil
	}
	return &Client{baseURL: parsed, http: httpClient}, nil
}

func (c *Client) InstanceID() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.baseURL.String())))
}

func (c *Client) Login(ctx context.Context, email string, password string, turnstileToken string) (*LoginResult, error) {
	var result LoginResult
	err := c.post(ctx, "/api/v1/auth/login", map[string]string{
		"email":           strings.TrimSpace(email),
		"password":        password,
		"turnstile_token": strings.TrimSpace(turnstileToken),
	}, &result, "")
	return &result, err
}

func (c *Client) VerifyLogin2FA(ctx context.Context, tempToken string, code string) (*TokenPair, error) {
	var result TokenPair
	err := c.post(ctx, "/api/v1/auth/login/2fa", map[string]string{
		"temp_token": strings.TrimSpace(tempToken),
		"totp_code":  strings.TrimSpace(code),
	}, &result, "")
	return &result, err
}

func (c *Client) SendRegistrationCode(ctx context.Context, email string, turnstileToken string) (*VerificationCodeResult, error) {
	var result VerificationCodeResult
	err := c.post(ctx, "/api/v1/auth/send-verify-code", map[string]string{
		"email":           strings.TrimSpace(email),
		"turnstile_token": strings.TrimSpace(turnstileToken),
	}, &result, "")
	return &result, err
}

func (c *Client) Register(ctx context.Context, email string, password string, code string, turnstileToken string) (*TokenPair, error) {
	var result TokenPair
	err := c.post(ctx, "/api/v1/auth/register", map[string]string{
		"email":           strings.TrimSpace(email),
		"password":        password,
		"verify_code":     strings.TrimSpace(code),
		"turnstile_token": strings.TrimSpace(turnstileToken),
	}, &result, "")
	return &result, err
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	var result TokenPair
	err := c.post(ctx, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": strings.TrimSpace(refreshToken),
	}, &result, "")
	return &result, err
}

func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	return c.post(ctx, "/api/v1/auth/logout", map[string]string{
		"refresh_token": strings.TrimSpace(refreshToken),
	}, nil, "")
}

func (c *Client) ChangePassword(ctx context.Context, accessToken string, currentPassword string, newPassword string) error {
	return c.put(ctx, "/api/v1/user/password", map[string]string{
		"old_password": currentPassword,
		"new_password": newPassword,
	}, nil, accessToken)
}

func (c *Client) Me(ctx context.Context, accessToken string) (*User, error) {
	var result User
	if err := c.get(ctx, "/api/v1/auth/me", &result, accessToken); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UserProfile(ctx context.Context, accessToken string) (*UserProfile, error) {
	var result UserProfile
	if err := c.get(ctx, "/api/v1/user/profile", &result, accessToken); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListAPIKeys(ctx context.Context, accessToken string, page, pageSize int) (*APIKeyPage, error) {
	values := url.Values{}
	values.Set("page", fmt.Sprintf("%d", page))
	values.Set("page_size", fmt.Sprintf("%d", pageSize))
	var result APIKeyPage
	if err := c.getQuery(ctx, "/api/v1/keys", values, &result, accessToken); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) AvailableGroups(ctx context.Context, accessToken string) ([]AvailableGroup, error) {
	var result []AvailableGroup
	return result, c.get(ctx, "/api/v1/groups/available", &result, accessToken)
}

func (c *Client) CreateAPIKey(ctx context.Context, accessToken string, input CreateAPIKeyInput, idempotencyKey string) (*APIKey, error) {
	var result APIKey
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode Sub2API request: %w", err)
	}
	if err := c.doWithHeaders(ctx, http.MethodPost, "/api/v1/keys", body, &result, accessToken, map[string]string{
		"Idempotency-Key": idempotencyKey,
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Announcements(ctx context.Context, accessToken string) ([]Announcement, error) {
	var result []Announcement
	return result, c.get(ctx, "/api/v1/announcements", &result, accessToken)
}

func (c *Client) MarkAnnouncementRead(ctx context.Context, accessToken string, id int64) error {
	if id <= 0 {
		return errors.New("invalid announcement id")
	}
	return c.post(ctx, "/api/v1/announcements/"+strconv.FormatInt(id, 10)+"/read", nil, nil, accessToken)
}

func (c *Client) CheckoutInfo(ctx context.Context, accessToken string) (CheckoutInfo, error) {
	var result CheckoutInfo
	return result, c.get(ctx, "/api/v1/payment/checkout-info", &result, accessToken)
}
func (c *Client) Redeem(ctx context.Context, accessToken string, code string) (*RedeemResult, error) {
	var result RedeemResult
	if err := c.post(ctx, "/api/v1/redeem", map[string]string{"code": strings.TrimSpace(code)}, &result, accessToken); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) CreatePaymentOrder(ctx context.Context, accessToken string, input CreatePaymentOrderInput) (*CreatePaymentOrderResult, error) {
	var result CreatePaymentOrderResult
	if err := c.post(ctx, "/api/v1/payment/orders", input, &result, accessToken); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PaymentOrders(ctx context.Context, accessToken string, input PaymentOrderQuery) (*PaymentOrderPage, error) {
	values := url.Values{}
	values.Set("page", strconv.Itoa(input.Page))
	values.Set("page_size", strconv.Itoa(input.PageSize))
	if input.Status != "" {
		values.Set("status", input.Status)
	}
	if input.OrderType != "" {
		values.Set("order_type", input.OrderType)
	}
	if input.PaymentType != "" {
		values.Set("payment_type", input.PaymentType)
	}
	var result PaymentOrderPage
	return &result, c.getQuery(ctx, "/api/v1/payment/orders/my", values, &result, accessToken)
}

func (c *Client) PaymentOrder(ctx context.Context, accessToken string, id int64) (*PaymentOrder, error) {
	var result PaymentOrder
	return &result, c.get(ctx, "/api/v1/payment/orders/"+strconv.FormatInt(id, 10), &result, accessToken)
}

func (c *Client) VerifyPaymentOrder(ctx context.Context, accessToken, orderNo string) (*PaymentOrder, error) {
	var result PaymentOrder
	return &result, c.post(ctx, "/api/v1/payment/orders/verify", map[string]string{"out_trade_no": orderNo}, &result, accessToken)
}

func (c *Client) CancelPaymentOrder(ctx context.Context, accessToken string, id int64) error {
	var result struct {
		Message string `json:"message"`
	}
	return c.post(ctx, "/api/v1/payment/orders/"+strconv.FormatInt(id, 10)+"/cancel", nil, &result, accessToken)
}

func (c *Client) RequestPaymentRefund(ctx context.Context, accessToken string, id int64, reason string) error {
	var result struct {
		Message string `json:"message"`
	}
	return c.post(ctx, "/api/v1/payment/orders/"+strconv.FormatInt(id, 10)+"/refund-request", map[string]string{"reason": reason}, &result, accessToken)
}

func (c *Client) RedemptionHistory(ctx context.Context, accessToken string) ([]RedemptionHistoryItem, error) {
	var result []RedemptionHistoryItem
	return result, c.get(ctx, "/api/v1/redeem/history", &result, accessToken)
}

func (c *Client) PaymentPlans(ctx context.Context, accessToken string) ([]PaymentPlan, error) {
	var result []PaymentPlan
	return result, c.get(ctx, "/api/v1/payment/plans", &result, accessToken)
}

func (c *Client) ActiveSubscription(ctx context.Context, accessToken string) (json.RawMessage, error) {
	var result json.RawMessage
	return result, c.get(ctx, "/api/v1/subscriptions/active", &result, accessToken)
}

func (c *Client) ActiveSubscriptions(ctx context.Context, accessToken string) ([]Subscription, error) {
	var result []Subscription
	return result, c.get(ctx, "/api/v1/subscriptions/active", &result, accessToken)
}

func (c *Client) Usage(ctx context.Context, accessToken string, input UsageQuery) (*UsagePage, error) {
	values := url.Values{}
	values.Set("page", fmt.Sprintf("%d", input.Page))
	values.Set("page_size", fmt.Sprintf("%d", input.PageSize))
	if input.Model != "" {
		values.Set("model", input.Model)
	}
	if input.BillingType != "" {
		values.Set("billing_type", input.BillingType)
	}
	if input.SortBy != "" {
		values.Set("sort_by", input.SortBy)
	}
	if input.SortOrder != "" {
		values.Set("sort_order", input.SortOrder)
	}
	var result UsagePage
	err := c.getQuery(ctx, "/api/v1/usage", values, &result, accessToken)
	return &result, err
}
func (c *Client) Trend(ctx context.Context, accessToken, start, end, granularity string) (*UsageTrend, error) {
	values := url.Values{}
	values.Set("start_date", start)
	values.Set("end_date", end)
	values.Set("granularity", granularity)
	var result UsageTrend
	err := c.getQuery(ctx, "/api/v1/usage/dashboard/trend", values, &result, accessToken)
	return &result, err
}

func (c *Client) Settings(ctx context.Context) (*PublicSettings, error) {
	var result PublicSettings
	if err := c.get(ctx, "/api/v1/settings/public", &result, ""); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) get(ctx context.Context, path string, output any, accessToken string) error {
	return c.do(ctx, http.MethodGet, path, nil, output, accessToken)
}

func (c *Client) getQuery(ctx context.Context, path string, values url.Values, output any, accessToken string) error {
	if c == nil || c.baseURL == nil {
		return errors.New("Sub2API client is not configured")
	}
	target := *c.baseURL
	target.Path = path
	target.RawQuery = values.Encode()
	return c.doURL(ctx, http.MethodGet, &target, nil, output, accessToken, true)
}

func (c *Client) post(ctx context.Context, path string, input any, output any, accessToken string) error {
	return c.write(ctx, http.MethodPost, path, input, output, accessToken)
}

func (c *Client) put(ctx context.Context, path string, input any, output any, accessToken string) error {
	return c.write(ctx, http.MethodPut, path, input, output, accessToken)
}

func (c *Client) write(ctx context.Context, method string, path string, input any, output any, accessToken string) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode Sub2API request: %w", err)
	}
	return c.do(ctx, method, path, body, output, accessToken)
}

func (c *Client) do(ctx context.Context, method string, path string, body []byte, output any, accessToken string) error {
	return c.doWithHeaders(ctx, method, path, body, output, accessToken, nil)
}

func (c *Client) doWithHeaders(ctx context.Context, method string, path string, body []byte, output any, accessToken string, headers map[string]string) error {
	if c == nil || c.baseURL == nil || c.http == nil {
		return errors.New("Sub2API client is not configured")
	}
	target := c.baseURL.ResolveReference(&url.URL{Path: path})
	return c.doURLWithHeaders(ctx, method, target, body, output, accessToken, true, headers)
}

func (c *Client) doRaw(ctx context.Context, method string, path string, body []byte, output any, apiKey string) error {
	if c == nil || c.baseURL == nil || c.http == nil {
		return errors.New("Sub2API client is not configured")
	}
	target := c.baseURL.ResolveReference(&url.URL{Path: path})
	return c.doURL(ctx, method, target, body, output, apiKey, false)
}

func (c *Client) doURL(ctx context.Context, method string, target *url.URL, body []byte, output any, token string, envelopeExpected bool) error {
	return c.doURLWithHeaders(ctx, method, target, body, output, token, envelopeExpected, nil)
}

func (c *Client) doURLWithHeaders(ctx context.Context, method string, target *url.URL, body []byte, output any, token string, envelopeExpected bool, headers map[string]string) error {
	if target == nil {
		return errors.New("Sub2API target is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build Sub2API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token := strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			request.Header.Set(key, value)
		}
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("request Sub2API: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("read Sub2API response: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return fmt.Errorf("Sub2API response exceeds %d bytes", maxResponseSize)
	}

	if !envelopeExpected {
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
				return ErrUnauthorized
			}
			return &APIError{StatusCode: response.StatusCode}
		}
		if output != nil && len(responseBody) != 0 {
			if err := json.Unmarshal(responseBody, output); err != nil {
				return fmt.Errorf("decode Sub2API gateway response: %w", err)
			}
		}
		return nil
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Reason  string          `json:"reason"`
		Data    json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("decode Sub2API response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || envelope.Code != 0 {
		apiErr := &APIError{StatusCode: response.StatusCode, Code: envelope.Code, Message: envelope.Message, Reason: envelope.Reason}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%w: %w", ErrUnauthorized, apiErr)
		}
		return apiErr
	}
	if output == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err = json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("decode Sub2API response data: %w", err)
	}
	return nil
}
