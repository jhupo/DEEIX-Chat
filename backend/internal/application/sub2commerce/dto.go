package sub2commerce

import "time"

type ConfigData struct {
	Config     Config
	ObservedAt time.Time
}
type Config struct {
	Mode                      string
	PaymentMethods            []PaymentMethod
	DisplayCurrency           string
	USDToCNYRate              float64
	BalanceDisabled           bool
	BalanceRechargeMultiplier float64
	RechargeFeeRate           float64
	GlobalDailyLimitUSD       *float64
	GlobalWeeklyLimitUSD      *float64
	GlobalMonthlyLimitUSD     *float64
	Plans                     []Plan
}
type PaymentMethod struct {
	ID       string
	Currency string
	Min      float64
	Max      float64
}
type AccountData struct {
	Account    Account
	ObservedAt time.Time
}
type Account struct {
	Balance       float64
	FrozenBalance float64
	Status        string
}
type PlansData struct {
	Plans      []Plan
	ObservedAt time.Time
}
type Plan struct {
	ID                  int64
	Code                string
	Name                string
	Description         string
	FeatureJSON         string
	GroupPlatform       string
	RateMultiplier      float64
	ModelRateMultiplier float64
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	PeriodCreditUSD     float64
	ValidityDays        int
	OriginalPriceCents  int64
	ModelScopesJSON     string
	SortOrder           int
	IsActive            bool
	Prices              []PlanPrice
}
type OverviewData struct {
	Overview   Overview
	ObservedAt time.Time
}
type Overview struct {
	Mode                     string
	Account                  Account
	Plan                     *Plan
	PeriodStartAt            *time.Time
	PeriodEndAt              *time.Time
	PeriodCreditUSD          float64
	PeriodCreditNanousd      int64
	PeriodUsedUSD            float64
	PeriodUsedNanousd        int64
	PeriodRemainingUSD       float64
	PeriodRemainingNanousd   int64
	SubscriptionEntitlements []SubscriptionEntitlement
}
type SubscriptionEntitlement struct {
	ID                   int64
	UserID               uint
	PlanID               int64
	PriceID              int64
	Status               string
	StartAt              time.Time
	CurrentPeriodStartAt time.Time
	CurrentPeriodEndAt   time.Time
	CancelAtPeriodEnd    bool
	AutoRenew            bool
	Plan                 Plan
	IsCurrent            bool
}
type PlanPrice struct {
	ID              int64
	PlanID          int64
	Code            string
	BillingInterval string
	Currency        string
	AmountCents     int64
	IsActive        bool
	IsDefault       bool
}
type UsageData struct {
	Results    []UsageRow
	Total      int64
	ObservedAt time.Time
}
type UsageRow struct {
	ID           int64
	Model        string
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	ActualCost   string
	DurationMS   int64
	CreatedAt    time.Time
}
type DailyData struct {
	Results    []DailyRow
	ObservedAt time.Time
}
type DailyRow struct {
	UsageDate        string
	CallCount        int64
	RecordCount      int64
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
	ActualCost       string
}
type MonthlyData struct {
	Results    []MonthlyRow
	ObservedAt time.Time
}
type MonthlyRow struct {
	MonthStartAt string
	DailyRow
}
type CheckoutData struct {
	Checkout   CheckoutResult
	ObservedAt time.Time
}
type RedemptionData struct {
	Redemption RedemptionResult
	Account    Account
	Overview   Overview
	ObservedAt time.Time
}
type RedemptionResult struct {
	ID    int64
	Type  string
	Value float64
}
