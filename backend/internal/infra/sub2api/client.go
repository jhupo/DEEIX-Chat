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
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

const (
	requestTimeout  = 15 * time.Second
	maxResponseSize = 1 << 20
)

var ErrUnauthorized = errors.New("sub2api unauthorized")

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

type User struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	AvatarURL string `json:"avatar_url"`
}

// UserProfile is the account projection exposed by Sub2's user API.
type UserProfile struct {
	User
	Balance       float64 `json:"balance"`
	FrozenBalance float64 `json:"frozen_balance"`
}

type APIKey struct {
	ID        int64        `json:"id"`
	UserID    int64        `json:"user_id"`
	Name      string       `json:"name"`
	Key       string       `json:"key"`
	GroupID   *int64       `json:"group_id"`
	Group     *APIKeyGroup `json:"group,omitempty"`
	Status    string       `json:"status"`
	Quota     float64      `json:"quota"`
	QuotaUsed float64      `json:"quota_used"`
	ExpiresAt *time.Time   `json:"expires_at"`
}

type APIKeyGroup struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type APIKeyPage struct {
	Items []APIKey `json:"items"`
	Total int      `json:"total"`
}

type GatewayModel struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

type GatewayModelList struct {
	Data []GatewayModel `json:"data"`
}

type CheckoutInfo struct {
	Methods                   map[string]PaymentMethod `json:"methods"`
	GlobalDailyLimitUSD       *float64                 `json:"global_daily_limit_usd"`
	GlobalWeeklyLimitUSD      *float64                 `json:"global_weekly_limit_usd"`
	GlobalMonthlyLimitUSD     *float64                 `json:"global_monthly_limit_usd"`
	BalanceDisabled           bool                     `json:"balance_disabled"`
	BalanceRechargeMultiplier float64                  `json:"balance_recharge_multiplier"`
	SubscriptionUSDToCNYRate  float64                  `json:"subscription_usd_to_cny_rate"`
	RechargeFeeRate           float64                  `json:"recharge_fee_rate"`
	Plans                     []PaymentPlan            `json:"plans"`
}
type RedeemResult struct {
	ID    int64   `json:"id"`
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}
type CreatePaymentOrderInput struct {
	OrderType   string  `json:"order_type"`
	PlanID      int64   `json:"plan_id,omitempty"`
	Amount      float64 `json:"amount,omitempty"`
	PaymentType string  `json:"payment_type"`
	ReturnURL   string  `json:"return_url,omitempty"`
}
type CreatePaymentOrderResult struct {
	OrderID     int64      `json:"order_id"`
	Amount      float64    `json:"amount"`
	PayAmount   float64    `json:"pay_amount"`
	FeeRate     float64    `json:"fee_rate"`
	Status      string     `json:"status"`
	PaymentType string     `json:"payment_type"`
	OutTradeNo  string     `json:"out_trade_no"`
	PayURL      string     `json:"pay_url"`
	QRCode      string     `json:"qr_code"`
	Currency    string     `json:"currency"`
	ExpiresAt   *time.Time `json:"expires_at"`
}
type PaymentOrder struct {
	ID                  int64      `json:"id"`
	Amount              float64    `json:"amount"`
	PayAmount           float64    `json:"pay_amount"`
	FeeRate             float64    `json:"fee_rate"`
	Currency            string     `json:"currency"`
	PaymentType         string     `json:"payment_type"`
	OutTradeNo          string     `json:"out_trade_no"`
	Status              string     `json:"status"`
	OrderType           string     `json:"order_type"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	PaidAt              *time.Time `json:"paid_at"`
	CompletedAt         *time.Time `json:"completed_at"`
	RefundAmount        float64    `json:"refund_amount"`
	RefundReason        *string    `json:"refund_reason"`
	RefundRequestedAt   *time.Time `json:"refund_requested_at"`
	RefundRequestReason *string    `json:"refund_request_reason"`
	PlanID              *int64     `json:"plan_id"`
}
type PaymentOrderPage struct {
	Items    []PaymentOrder `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
type PaymentOrderQuery struct {
	Page, PageSize                 int
	Status, OrderType, PaymentType string
}
type RedemptionHistoryItem struct {
	ID           int64      `json:"id"`
	Code         string     `json:"code"`
	Type         string     `json:"type"`
	Value        float64    `json:"value"`
	Status       string     `json:"status"`
	UsedAt       *time.Time `json:"used_at"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	GroupID      *int64     `json:"group_id"`
	ValidityDays int        `json:"validity_days"`
}
type PaymentMethod struct {
	PaymentType string  `json:"payment_type"`
	Currency    string  `json:"currency"`
	SingleMin   float64 `json:"single_min"`
	SingleMax   float64 `json:"single_max"`
}
type PaymentPlan struct {
	ID                  int64           `json:"id"`
	GroupID             int64           `json:"group_id"`
	GroupName           string          `json:"group_name"`
	GroupPlatform       string          `json:"group_platform"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	Price               float64         `json:"price"`
	OriginalPrice       float64         `json:"original_price"`
	RateMultiplier      float64         `json:"rate_multiplier"`
	ModelRateMultiplier float64         `json:"model_rate_multiplier"`
	ValidityUnit        string          `json:"validity_unit"`
	ValidityDays        int             `json:"validity_days"`
	Features            json.RawMessage `json:"features"`
	ModelScopes         json.RawMessage `json:"model_scopes"`
	ForSale             bool            `json:"for_sale"`
	SortOrder           int             `json:"sort_order"`
	DailyLimitUSD       *float64        `json:"daily_limit_usd"`
	WeeklyLimitUSD      *float64        `json:"weekly_limit_usd"`
	MonthlyLimitUSD     *float64        `json:"monthly_limit_usd"`
}
type UsageRecord struct {
	ID           int64       `json:"id"`
	Model        string      `json:"model"`
	InputTokens  int64       `json:"input_tokens"`
	OutputTokens int64       `json:"output_tokens"`
	TotalTokens  int64       `json:"total_tokens"`
	ActualCost   json.Number `json:"actual_cost"`
	DurationMS   int64       `json:"duration_ms"`
	CreatedAt    time.Time   `json:"created_at"`
	GroupName    string      `json:"group_name"`
}
type UsagePage struct {
	Items    []UsageRecord `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}
type UsageQuery struct {
	Model       string
	BillingType string
	SortBy      string
	SortOrder   string
	Page        int
	PageSize    int
}
type TrendPoint struct {
	Date                string      `json:"date"`
	Requests            int64       `json:"requests"`
	InputTokens         int64       `json:"input_tokens"`
	OutputTokens        int64       `json:"output_tokens"`
	CacheCreationTokens int64       `json:"cache_creation_tokens"`
	CacheReadTokens     int64       `json:"cache_read_tokens"`
	TotalTokens         int64       `json:"total_tokens"`
	ActualCost          json.Number `json:"actual_cost"`
}
type UsageTrend struct {
	StartDate   string       `json:"start_date"`
	EndDate     string       `json:"end_date"`
	Granularity string       `json:"granularity"`
	Trend       []TrendPoint `json:"trend"`
}
type Subscription struct {
	ID              int64     `json:"id"`
	GroupID         int64     `json:"group_id"`
	StartsAt        time.Time `json:"starts_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	Status          string    `json:"status"`
	DailyUsageUSD   float64   `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64   `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64   `json:"monthly_usage_usd"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	User         *User  `json:"user,omitempty"`
}

type LoginResult struct {
	TokenPair
	Requires2FA     bool   `json:"requires_2fa"`
	TempToken       string `json:"temp_token"`
	UserEmailMasked string `json:"user_email_masked"`
}

type PublicSettings struct {
	RegistrationEnabled bool   `json:"registration_enabled"`
	EmailVerifyEnabled  bool   `json:"email_verify_enabled"`
	TurnstileEnabled    bool   `json:"turnstile_enabled"`
	TurnstileSiteKey    string `json:"turnstile_site_key"`
}

type VerificationCodeResult struct {
	Countdown int `json:"countdown"`
}

type APIError struct {
	StatusCode int
	Code       int
	Message    string
	Reason     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sub2api request failed: status=%d code=%d reason=%s", e.StatusCode, e.Code, e.Reason)
}

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

// GatewayModels uses the selected API key and intentionally bypasses the
// management envelope because /v1/models is OpenAI-shaped JSON.
func (c *Client) GatewayModels(ctx context.Context, apiKey string) (*GatewayModelList, error) {
	var result GatewayModelList
	if err := c.doRaw(ctx, http.MethodGet, "/v1/models", nil, &result, apiKey); err != nil {
		return nil, err
	}
	return &result, nil
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
	if c == nil || c.baseURL == nil || c.http == nil {
		return errors.New("Sub2API client is not configured")
	}
	target := c.baseURL.ResolveReference(&url.URL{Path: path})
	return c.doURL(ctx, method, target, body, output, accessToken, true)
}

func (c *Client) doRaw(ctx context.Context, method string, path string, body []byte, output any, apiKey string) error {
	if c == nil || c.baseURL == nil || c.http == nil {
		return errors.New("Sub2API client is not configured")
	}
	target := c.baseURL.ResolveReference(&url.URL{Path: path})
	return c.doURL(ctx, method, target, body, output, apiKey, false)
}

func (c *Client) doURL(ctx context.Context, method string, target *url.URL, body []byte, output any, token string, envelopeExpected bool) error {
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
