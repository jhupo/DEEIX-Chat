package billing

import (
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2commerce"
)

// These are HTTP boundary models. Application models intentionally carry no
// serialization annotations so they can remain independent of the transport.
type sub2ConfigDataDTO struct {
	Config     sub2ConfigDTO `json:"config"`
	ObservedAt time.Time     `json:"observedAt"`
}
type sub2ConfigDTO struct {
	Mode                      string                 `json:"mode"`
	PaymentMethods            []sub2PaymentMethodDTO `json:"paymentMethods"`
	DisplayCurrency           string                 `json:"displayCurrency"`
	USDToCNYRate              float64                `json:"usdToCNYRate"`
	BalanceDisabled           bool                   `json:"balanceDisabled"`
	BalanceRechargeMultiplier float64                `json:"balanceRechargeMultiplier"`
	RechargeFeeRate           float64                `json:"rechargeFeeRate"`
	GlobalDailyLimitUSD       *float64               `json:"globalDailyLimitUSD"`
	GlobalWeeklyLimitUSD      *float64               `json:"globalWeeklyLimitUSD"`
	GlobalMonthlyLimitUSD     *float64               `json:"globalMonthlyLimitUSD"`
	Plans                     []sub2PlanDTO          `json:"plans"`
}
type sub2PaymentMethodDTO struct {
	ID       string  `json:"id"`
	Currency string  `json:"currency"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}
type sub2AccountDataDTO struct {
	Account    sub2AccountDTO `json:"account"`
	ObservedAt time.Time      `json:"observedAt"`
}
type sub2AccountDTO struct {
	Balance       float64 `json:"balance"`
	FrozenBalance float64 `json:"frozenBalance"`
	Status        string  `json:"status"`
}
type sub2PlanDTO struct {
	ID                  int64              `json:"id"`
	Code                string             `json:"code"`
	Name                string             `json:"name"`
	Description         string             `json:"description"`
	FeatureJSON         string             `json:"featureJSON"`
	GroupPlatform       string             `json:"groupPlatform"`
	RateMultiplier      float64            `json:"rateMultiplier"`
	ModelRateMultiplier float64            `json:"modelRateMultiplier"`
	DailyLimitUSD       *float64           `json:"dailyLimitUSD"`
	WeeklyLimitUSD      *float64           `json:"weeklyLimitUSD"`
	MonthlyLimitUSD     *float64           `json:"monthlyLimitUSD"`
	PeriodCreditUSD     float64            `json:"periodCreditUSD"`
	ValidityDays        int                `json:"validityDays"`
	OriginalPriceCents  int64              `json:"originalPriceCents"`
	ModelScopesJSON     string             `json:"modelScopesJSON"`
	SortOrder           int                `json:"sortOrder"`
	IsActive            bool               `json:"isActive"`
	Prices              []sub2PlanPriceDTO `json:"prices"`
}
type sub2PlanPriceDTO struct {
	ID              int64  `json:"id"`
	PlanID          int64  `json:"planID"`
	Code            string `json:"code"`
	BillingInterval string `json:"billingInterval"`
	Currency        string `json:"currency"`
	AmountCents     int64  `json:"amountCents"`
	IsActive        bool   `json:"isActive"`
	IsDefault       bool   `json:"isDefault"`
}
type sub2PlansDataDTO struct {
	Plans      []sub2PlanDTO `json:"plans"`
	ObservedAt time.Time     `json:"observedAt"`
}
type sub2OverviewDataDTO struct {
	Overview   sub2OverviewDTO `json:"overview"`
	ObservedAt time.Time       `json:"observedAt"`
}
type sub2OverviewDTO struct {
	Mode                     string                           `json:"mode"`
	Account                  sub2AccountDTO                   `json:"account"`
	Plan                     *sub2PlanDTO                     `json:"plan"`
	PeriodStartAt            *time.Time                       `json:"periodStartAt"`
	PeriodEndAt              *time.Time                       `json:"periodEndAt"`
	PeriodCreditUSD          float64                          `json:"periodCreditUSD"`
	PeriodCreditNanousd      int64                            `json:"periodCreditNanousd"`
	PeriodUsedUSD            float64                          `json:"periodUsedUSD"`
	PeriodUsedNanousd        int64                            `json:"periodUsedNanousd"`
	PeriodRemainingUSD       float64                          `json:"periodRemainingUSD"`
	PeriodRemainingNanousd   int64                            `json:"periodRemainingNanousd"`
	SubscriptionEntitlements []sub2SubscriptionEntitlementDTO `json:"subscriptionEntitlements"`
}
type sub2SubscriptionEntitlementDTO struct {
	ID                   int64       `json:"id"`
	UserID               uint        `json:"userID"`
	PlanID               int64       `json:"planID"`
	PriceID              int64       `json:"priceID"`
	Status               string      `json:"status"`
	StartAt              time.Time   `json:"startAt"`
	CurrentPeriodStartAt time.Time   `json:"currentPeriodStartAt"`
	CurrentPeriodEndAt   time.Time   `json:"currentPeriodEndAt"`
	CancelAtPeriodEnd    bool        `json:"cancelAtPeriodEnd"`
	AutoRenew            bool        `json:"autoRenew"`
	Plan                 sub2PlanDTO `json:"plan"`
	IsCurrent            bool        `json:"isCurrent"`
}
type sub2UsageDataDTO struct {
	Results    []sub2UsageRowDTO `json:"results"`
	Total      int64             `json:"total"`
	ObservedAt time.Time         `json:"observedAt"`
}
type sub2UsageRowDTO struct {
	ID           int64     `json:"id"`
	Model        string    `json:"model"`
	InputTokens  int64     `json:"inputTokens"`
	OutputTokens int64     `json:"outputTokens"`
	TotalTokens  int64     `json:"totalTokens"`
	ActualCost   string    `json:"actualCost"`
	DurationMS   int64     `json:"durationMS"`
	CreatedAt    time.Time `json:"createdAt"`
}
type sub2DailyDataDTO struct {
	Results    []sub2DailyRowDTO `json:"results"`
	ObservedAt time.Time         `json:"observedAt"`
}
type sub2DailyRowDTO struct {
	UsageDate        string `json:"usageDate"`
	CallCount        int64  `json:"callCount"`
	RecordCount      int64  `json:"recordCount"`
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  int64  `json:"cacheReadTokens"`
	CacheWriteTokens int64  `json:"cacheWriteTokens"`
	TotalTokens      int64  `json:"totalTokens"`
	ActualCost       string `json:"actualCost"`
}
type sub2MonthlyDataDTO struct {
	Results    []sub2MonthlyRowDTO `json:"results"`
	ObservedAt time.Time           `json:"observedAt"`
}
type sub2MonthlyRowDTO struct {
	MonthStartAt     string `json:"monthStartAt"`
	CallCount        int64  `json:"callCount"`
	RecordCount      int64  `json:"recordCount"`
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  int64  `json:"cacheReadTokens"`
	CacheWriteTokens int64  `json:"cacheWriteTokens"`
	TotalTokens      int64  `json:"totalTokens"`
	ActualCost       string `json:"actualCost"`
}
type sub2CheckoutDataDTO struct {
	Checkout   sub2CheckoutDTO `json:"checkout"`
	ObservedAt time.Time       `json:"observedAt"`
}
type sub2CheckoutDTO struct {
	OrderNo            string     `json:"orderNo"`
	OrderType          string     `json:"orderType"`
	Provider           string     `json:"provider"`
	Status             string     `json:"status"`
	ExternalCheckoutID string     `json:"externalCheckoutID"`
	CheckoutURL        string     `json:"checkoutURL"`
	BaseAmountCents    int64      `json:"baseAmountCents"`
	BaseCurrency       string     `json:"baseCurrency"`
	PayAmountCents     int64      `json:"payAmountCents"`
	PayCurrency        string     `json:"payCurrency"`
	FXRate             string     `json:"fxRate"`
	CreditNanousd      int64      `json:"creditNanousd"`
	CreditUSD          float64    `json:"creditUSD"`
	ExpiredAt          *time.Time `json:"expiredAt"`
}
type sub2RedemptionDataDTO struct {
	Redemption sub2RedemptionDTO `json:"redemption"`
	Account    sub2AccountDTO    `json:"account"`
	Overview   sub2OverviewDTO   `json:"overview"`
	ObservedAt time.Time         `json:"observedAt"`
}
type sub2RedemptionDTO struct {
	ID    int64   `json:"id"`
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}
type sub2OrdersDataDTO struct {
	Results    []sub2OrderDTO `json:"results"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	ObservedAt time.Time      `json:"observedAt"`
}
type sub2OrderDataDTO struct {
	Order      sub2OrderDTO `json:"order"`
	ObservedAt time.Time    `json:"observedAt"`
}
type sub2OrderDTO struct {
	ID                  int64      `json:"id"`
	AmountUSD           float64    `json:"amountUSD"`
	PayAmount           float64    `json:"payAmount"`
	FeeRate             float64    `json:"feeRate"`
	Currency            string     `json:"currency"`
	PaymentType         string     `json:"paymentType"`
	OrderNo             string     `json:"orderNo"`
	Status              string     `json:"status"`
	OrderType           string     `json:"orderType"`
	CreatedAt           time.Time  `json:"createdAt"`
	ExpiresAt           time.Time  `json:"expiresAt"`
	PaidAt              *time.Time `json:"paidAt"`
	CompletedAt         *time.Time `json:"completedAt"`
	RefundAmount        float64    `json:"refundAmount"`
	RefundReason        *string    `json:"refundReason"`
	RefundRequestedAt   *time.Time `json:"refundRequestedAt"`
	RefundRequestReason *string    `json:"refundRequestReason"`
	PlanID              *int64     `json:"planID"`
}
type sub2RedemptionsDataDTO struct {
	Results    []sub2RedemptionHistoryDTO `json:"results"`
	ObservedAt time.Time                  `json:"observedAt"`
}
type sub2RedemptionHistoryDTO struct {
	ID           int64      `json:"id"`
	Code         string     `json:"code"`
	Type         string     `json:"type"`
	Value        float64    `json:"value"`
	Status       string     `json:"status"`
	UsedAt       *time.Time `json:"usedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	GroupID      *int64     `json:"groupID"`
	ValidityDays int        `json:"validityDays"`
}

func toSub2ConfigData(v app.ConfigData) sub2ConfigDataDTO {
	return sub2ConfigDataDTO{Config: toSub2Config(v.Config), ObservedAt: v.ObservedAt}
}
func toSub2Config(v app.Config) sub2ConfigDTO {
	methods := make([]sub2PaymentMethodDTO, len(v.PaymentMethods))
	for i, x := range v.PaymentMethods {
		methods[i] = sub2PaymentMethodDTO{ID: x.ID, Currency: x.Currency, Min: x.Min, Max: x.Max}
	}
	return sub2ConfigDTO{Mode: v.Mode, PaymentMethods: methods, DisplayCurrency: v.DisplayCurrency, USDToCNYRate: v.USDToCNYRate, BalanceDisabled: v.BalanceDisabled, BalanceRechargeMultiplier: v.BalanceRechargeMultiplier, RechargeFeeRate: v.RechargeFeeRate, GlobalDailyLimitUSD: v.GlobalDailyLimitUSD, GlobalWeeklyLimitUSD: v.GlobalWeeklyLimitUSD, GlobalMonthlyLimitUSD: v.GlobalMonthlyLimitUSD, Plans: toSub2Plans(v.Plans)}
}
func toSub2Account(v app.Account) sub2AccountDTO {
	return sub2AccountDTO{Balance: v.Balance, FrozenBalance: v.FrozenBalance, Status: v.Status}
}
func toSub2Plan(v app.Plan) sub2PlanDTO {
	prices := make([]sub2PlanPriceDTO, len(v.Prices))
	for i, x := range v.Prices {
		prices[i] = sub2PlanPriceDTO{ID: x.ID, PlanID: x.PlanID, Code: x.Code, BillingInterval: x.BillingInterval, Currency: x.Currency, AmountCents: x.AmountCents, IsActive: x.IsActive, IsDefault: x.IsDefault}
	}
	return sub2PlanDTO{ID: v.ID, Code: v.Code, Name: v.Name, Description: v.Description, FeatureJSON: v.FeatureJSON, GroupPlatform: v.GroupPlatform, RateMultiplier: v.RateMultiplier, ModelRateMultiplier: v.ModelRateMultiplier, DailyLimitUSD: v.DailyLimitUSD, WeeklyLimitUSD: v.WeeklyLimitUSD, MonthlyLimitUSD: v.MonthlyLimitUSD, PeriodCreditUSD: v.PeriodCreditUSD, ValidityDays: v.ValidityDays, OriginalPriceCents: v.OriginalPriceCents, ModelScopesJSON: v.ModelScopesJSON, SortOrder: v.SortOrder, IsActive: v.IsActive, Prices: prices}
}
func toSub2Plans(v []app.Plan) []sub2PlanDTO {
	result := make([]sub2PlanDTO, len(v))
	for i, x := range v {
		result[i] = toSub2Plan(x)
	}
	return result
}
func toSub2Overview(v app.Overview) sub2OverviewDTO {
	entitlements := make([]sub2SubscriptionEntitlementDTO, len(v.SubscriptionEntitlements))
	for i, x := range v.SubscriptionEntitlements {
		entitlements[i] = sub2SubscriptionEntitlementDTO{ID: x.ID, UserID: x.UserID, PlanID: x.PlanID, PriceID: x.PriceID, Status: x.Status, StartAt: x.StartAt, CurrentPeriodStartAt: x.CurrentPeriodStartAt, CurrentPeriodEndAt: x.CurrentPeriodEndAt, CancelAtPeriodEnd: x.CancelAtPeriodEnd, AutoRenew: x.AutoRenew, Plan: toSub2Plan(x.Plan), IsCurrent: x.IsCurrent}
	}
	var plan *sub2PlanDTO
	if v.Plan != nil {
		converted := toSub2Plan(*v.Plan)
		plan = &converted
	}
	return sub2OverviewDTO{Mode: v.Mode, Account: toSub2Account(v.Account), Plan: plan, PeriodStartAt: v.PeriodStartAt, PeriodEndAt: v.PeriodEndAt, PeriodCreditUSD: v.PeriodCreditUSD, PeriodCreditNanousd: v.PeriodCreditNanousd, PeriodUsedUSD: v.PeriodUsedUSD, PeriodUsedNanousd: v.PeriodUsedNanousd, PeriodRemainingUSD: v.PeriodRemainingUSD, PeriodRemainingNanousd: v.PeriodRemainingNanousd, SubscriptionEntitlements: entitlements}
}
func toSub2UsageData(v app.UsageData) sub2UsageDataDTO {
	rows := make([]sub2UsageRowDTO, len(v.Results))
	for i, x := range v.Results {
		rows[i] = sub2UsageRowDTO{ID: x.ID, Model: x.Model, InputTokens: x.InputTokens, OutputTokens: x.OutputTokens, TotalTokens: x.TotalTokens, ActualCost: x.ActualCost, DurationMS: x.DurationMS, CreatedAt: x.CreatedAt}
	}
	return sub2UsageDataDTO{Results: rows, Total: v.Total, ObservedAt: v.ObservedAt}
}
func toSub2DailyRow(v app.DailyRow) sub2DailyRowDTO {
	return sub2DailyRowDTO{UsageDate: v.UsageDate, CallCount: v.CallCount, RecordCount: v.RecordCount, InputTokens: v.InputTokens, OutputTokens: v.OutputTokens, CacheReadTokens: v.CacheReadTokens, CacheWriteTokens: v.CacheWriteTokens, TotalTokens: v.TotalTokens, ActualCost: v.ActualCost}
}
func toSub2DailyData(v app.DailyData) sub2DailyDataDTO {
	rows := make([]sub2DailyRowDTO, len(v.Results))
	for i, x := range v.Results {
		rows[i] = toSub2DailyRow(x)
	}
	return sub2DailyDataDTO{Results: rows, ObservedAt: v.ObservedAt}
}
func toSub2MonthlyData(v app.MonthlyData) sub2MonthlyDataDTO {
	rows := make([]sub2MonthlyRowDTO, len(v.Results))
	for i, x := range v.Results {
		rows[i] = sub2MonthlyRowDTO{
			MonthStartAt: x.MonthStartAt, CallCount: x.CallCount, RecordCount: x.RecordCount,
			InputTokens: x.InputTokens, OutputTokens: x.OutputTokens,
			CacheReadTokens: x.CacheReadTokens, CacheWriteTokens: x.CacheWriteTokens,
			TotalTokens: x.TotalTokens, ActualCost: x.ActualCost,
		}
	}
	return sub2MonthlyDataDTO{Results: rows, ObservedAt: v.ObservedAt}
}
func toSub2Checkout(v app.CheckoutResult) sub2CheckoutDTO {
	return sub2CheckoutDTO{OrderNo: v.OrderNo, OrderType: v.OrderType, Provider: v.Provider, Status: v.Status, ExternalCheckoutID: v.ExternalCheckoutID, CheckoutURL: v.CheckoutURL, BaseAmountCents: v.BaseAmountCents, BaseCurrency: v.BaseCurrency, PayAmountCents: v.PayAmountCents, PayCurrency: v.PayCurrency, FXRate: v.FXRate, CreditNanousd: v.CreditNanousd, CreditUSD: v.CreditUSD, ExpiredAt: v.ExpiredAt}
}
func toSub2RedemptionData(v app.RedeemResult) sub2RedemptionDataDTO {
	return sub2RedemptionDataDTO{Redemption: sub2RedemptionDTO{ID: v.Redemption.ID, Type: v.Redemption.Type, Value: v.Redemption.Value}, Account: toSub2Account(v.Account), Overview: toSub2Overview(v.Overview), ObservedAt: v.ObservedAt}
}
func toSub2Order(v app.Order) sub2OrderDTO {
	return sub2OrderDTO{ID: v.ID, AmountUSD: v.AmountUSD, PayAmount: v.PayAmount, FeeRate: v.FeeRate, Currency: v.Currency, PaymentType: v.PaymentType, OrderNo: v.OrderNo, Status: v.Status, OrderType: v.OrderType, CreatedAt: v.CreatedAt, ExpiresAt: v.ExpiresAt, PaidAt: v.PaidAt, CompletedAt: v.CompletedAt, RefundAmount: v.RefundAmount, RefundReason: v.RefundReason, RefundRequestedAt: v.RefundRequestedAt, RefundRequestReason: v.RefundRequestReason, PlanID: v.PlanID}
}
func toSub2OrdersData(v app.OrdersData) sub2OrdersDataDTO {
	results := make([]sub2OrderDTO, len(v.Results))
	for i, item := range v.Results {
		results[i] = toSub2Order(item)
	}
	return sub2OrdersDataDTO{Results: results, Total: v.Total, Page: v.Page, PageSize: v.PageSize, ObservedAt: v.ObservedAt}
}
func toSub2OrderData(v app.OrderData) sub2OrderDataDTO {
	return sub2OrderDataDTO{Order: toSub2Order(v.Order), ObservedAt: v.ObservedAt}
}
func toSub2RedemptionsData(v app.RedemptionsData) sub2RedemptionsDataDTO {
	results := make([]sub2RedemptionHistoryDTO, len(v.Results))
	for i, item := range v.Results {
		results[i] = sub2RedemptionHistoryDTO{ID: item.ID, Code: item.Code, Type: item.Type, Value: item.Value, Status: item.Status, UsedAt: item.UsedAt, CreatedAt: item.CreatedAt, ExpiresAt: item.ExpiresAt, GroupID: item.GroupID, ValidityDays: item.ValidityDays}
	}
	return sub2RedemptionsDataDTO{Results: results, ObservedAt: v.ObservedAt}
}
