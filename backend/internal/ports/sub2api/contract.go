// Package sub2api defines the application-facing Sub2 account and commerce contract.
package sub2api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrUnauthorized = errors.New("sub2api unauthorized")

type Client interface {
	InstanceID() string
	Login(context.Context, string, string, string) (*LoginResult, error)
	VerifyLogin2FA(context.Context, string, string) (*TokenPair, error)
	SendRegistrationCode(context.Context, string, string) (*VerificationCodeResult, error)
	Register(context.Context, string, string, string, string) (*TokenPair, error)
	Refresh(context.Context, string) (*TokenPair, error)
	Logout(context.Context, string) error
	ChangePassword(context.Context, string, string, string) error
	Me(context.Context, string) (*User, error)
	UserProfile(context.Context, string) (*UserProfile, error)
	ListAPIKeys(context.Context, string, int, int) (*APIKeyPage, error)
	AvailableGroups(context.Context, string) ([]AvailableGroup, error)
	CreateAPIKey(context.Context, string, CreateAPIKeyInput, string) (*APIKey, error)
	Announcements(context.Context, string) ([]Announcement, error)
	MarkAnnouncementRead(context.Context, string, int64) error
	CheckoutInfo(context.Context, string) (CheckoutInfo, error)
	Redeem(context.Context, string, string) (*RedeemResult, error)
	CreatePaymentOrder(context.Context, string, CreatePaymentOrderInput) (*CreatePaymentOrderResult, error)
	PaymentOrders(context.Context, string, PaymentOrderQuery) (*PaymentOrderPage, error)
	PaymentOrder(context.Context, string, int64) (*PaymentOrder, error)
	VerifyPaymentOrder(context.Context, string, string) (*PaymentOrder, error)
	CancelPaymentOrder(context.Context, string, int64) error
	RequestPaymentRefund(context.Context, string, int64, string) error
	RedemptionHistory(context.Context, string) ([]RedemptionHistoryItem, error)
	PaymentPlans(context.Context, string) ([]PaymentPlan, error)
	ActiveSubscription(context.Context, string) (json.RawMessage, error)
	ActiveSubscriptions(context.Context, string) ([]Subscription, error)
	Usage(context.Context, string, UsageQuery) (*UsagePage, error)
	Trend(context.Context, string, string, string, string) (*UsageTrend, error)
	Settings(context.Context) (*PublicSettings, error)
}

type User struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	AvatarURL string `json:"avatar_url"`
}

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

type AvailableGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Platform    string `json:"platform"`
}

type CreateAPIKeyInput struct {
	Name    string `json:"name"`
	GroupID int64  `json:"group_id"`
}

type APIKeyPage struct {
	Items []APIKey `json:"items"`
	Total int      `json:"total"`
}

type Announcement struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	NotifyMode string     `json:"notify_mode"`
	StartsAt   *time.Time `json:"starts_at"`
	EndsAt     *time.Time `json:"ends_at"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
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
	OrderType     string  `json:"order_type"`
	PlanID        int64   `json:"plan_id,omitempty"`
	Amount        float64 `json:"amount,omitempty"`
	PaymentType   string  `json:"payment_type"`
	PaymentSource string  `json:"payment_source,omitempty"`
	IsMobile      bool    `json:"is_mobile"`
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
	Currency            string          `json:"currency"`
	RateMultiplier      float64         `json:"rate_multiplier"`
	ModelRateMultiplier float64         `json:"model_rate_multiplier"`
	ValidityUnit        string          `json:"validity_unit"`
	ValidityDays        int             `json:"validity_days"`
	Features            json.RawMessage `json:"features"`
	ModelScopes         json.RawMessage `json:"supported_model_scopes"`
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
	ID              int64              `json:"id"`
	GroupID         int64              `json:"group_id"`
	StartsAt        time.Time          `json:"starts_at"`
	ExpiresAt       time.Time          `json:"expires_at"`
	Status          string             `json:"status"`
	DailyUsageUSD   float64            `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64            `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64            `json:"monthly_usage_usd"`
	Group           *SubscriptionGroup `json:"group,omitempty"`
}

type SubscriptionGroup struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Platform        string   `json:"platform"`
	RateMultiplier  float64  `json:"rate_multiplier"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
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
