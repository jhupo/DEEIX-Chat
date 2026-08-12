package sub2commerce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"golang.org/x/sync/singleflight"
)

type TokenResolver interface {
	Sub2AccessTokenForSession(context.Context, uint, string) (string, error)
}
type Service struct {
	tokens        TokenResolver
	client        *sub2api.Client
	repo          repository.Sub2PaymentOperationRepository
	checkoutMu    sync.Mutex
	checkoutCache map[string]checkoutCacheEntry
	checkoutGroup singleflight.Group
	readMu        sync.Mutex
	readCache     map[string]readCacheEntry
	readGroup     singleflight.Group
}

type checkoutCacheEntry struct {
	info      sub2api.CheckoutInfo
	expiresAt time.Time
}

type readCacheEntry struct {
	value     any
	expiresAt time.Time
}

const (
	checkoutCacheTTL = time.Minute
	checkoutCacheMax = 256
	accountCacheTTL  = 20 * time.Second
	overviewCacheTTL = 20 * time.Second
	usageCacheTTL    = 10 * time.Second
	trendCacheTTL    = 30 * time.Second
	readCacheMax     = 1024
)

func NewService(tokens TokenResolver, client *sub2api.Client, repo repository.Sub2PaymentOperationRepository) *Service {
	return &Service{
		tokens: tokens, client: client, repo: repo,
		checkoutCache: make(map[string]checkoutCacheEntry),
		readCache:     make(map[string]readCacheEntry),
	}
}

func (s *Service) cachedRead(key string, ttl time.Duration, load func() (any, error)) (any, error) {
	now := time.Now()
	s.readMu.Lock()
	if entry, ok := s.readCache[key]; ok && now.Before(entry.expiresAt) {
		s.readMu.Unlock()
		return entry.value, nil
	}
	delete(s.readCache, key)
	s.readMu.Unlock()

	value, err, _ := s.readGroup.Do(key, func() (any, error) {
		now := time.Now()
		s.readMu.Lock()
		if entry, ok := s.readCache[key]; ok && now.Before(entry.expiresAt) {
			s.readMu.Unlock()
			return entry.value, nil
		}
		s.readMu.Unlock()

		loaded, loadErr := load()
		if loadErr != nil {
			return nil, loadErr
		}
		s.readMu.Lock()
		for cacheKey, entry := range s.readCache {
			if !now.Before(entry.expiresAt) {
				delete(s.readCache, cacheKey)
			}
		}
		for len(s.readCache) >= readCacheMax {
			for cacheKey := range s.readCache {
				delete(s.readCache, cacheKey)
				break
			}
		}
		s.readCache[key] = readCacheEntry{value: loaded, expiresAt: now.Add(ttl)}
		s.readMu.Unlock()
		return loaded, nil
	})
	return value, err
}

func commerceCacheKey(userID uint, sessionID, kind, suffix string) string {
	return strconv.FormatUint(uint64(userID), 10) + ":" + strings.TrimSpace(sessionID) + ":" + kind + ":" + suffix
}

func (s *Service) invalidateReadCache(userID uint, sessionID string) {
	prefix := strconv.FormatUint(uint64(userID), 10) + ":" + strings.TrimSpace(sessionID) + ":"
	s.readMu.Lock()
	for key := range s.readCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.readCache, key)
		}
	}
	s.readMu.Unlock()
}

func (s *Service) checkoutInfo(ctx context.Context, userID uint, sessionID, token string) (sub2api.CheckoutInfo, error) {
	key := strconv.FormatUint(uint64(userID), 10) + ":" + strings.TrimSpace(sessionID)
	now := time.Now()
	s.checkoutMu.Lock()
	s.pruneCheckoutCacheLocked(now, 0)
	entry, found := s.checkoutCache[key]
	if found {
		s.checkoutMu.Unlock()
		return entry.info, nil
	}
	s.checkoutMu.Unlock()

	value, err, _ := s.checkoutGroup.Do(key, func() (any, error) {
		now := time.Now()
		s.checkoutMu.Lock()
		if cached, ok := s.checkoutCache[key]; ok && now.Before(cached.expiresAt) {
			s.checkoutMu.Unlock()
			return cached.info, nil
		}
		s.checkoutMu.Unlock()
		info, loadErr := s.client.CheckoutInfo(ctx, token)
		if loadErr != nil {
			return nil, loadErr
		}
		storedAt := time.Now()
		s.checkoutMu.Lock()
		reserve := 1
		if _, exists := s.checkoutCache[key]; exists {
			reserve = 0
		}
		s.pruneCheckoutCacheLocked(storedAt, reserve)
		s.checkoutCache[key] = checkoutCacheEntry{info: info, expiresAt: storedAt.Add(checkoutCacheTTL)}
		s.checkoutMu.Unlock()
		return info, nil
	})
	if err != nil {
		return sub2api.CheckoutInfo{}, err
	}
	return value.(sub2api.CheckoutInfo), nil
}

func (s *Service) pruneCheckoutCacheLocked(now time.Time, reserve int) {
	for key, entry := range s.checkoutCache {
		if !now.Before(entry.expiresAt) {
			delete(s.checkoutCache, key)
		}
	}
	for len(s.checkoutCache)+reserve > checkoutCacheMax {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range s.checkoutCache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.checkoutCache, oldestKey)
	}
}
func (s *Service) Account(ctx context.Context, userID uint, sessionID string) (*AccountData, error) {
	value, err := s.cachedRead(commerceCacheKey(userID, sessionID, "account", "current"), accountCacheTTL, func() (any, error) {
		token, tokenErr := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		profile, profileErr := s.client.UserProfile(ctx, token)
		if profileErr != nil {
			return nil, profileErr
		}
		return &AccountData{Account: Account{Balance: profile.Balance, FrozenBalance: profile.FrozenBalance, Status: profile.Status}, ObservedAt: time.Now().UTC()}, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*AccountData), nil
}
func (s *Service) Config(ctx context.Context, userID uint, sessionID string) (*ConfigData, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	info, err := s.checkoutInfo(ctx, userID, sessionID, token)
	if err != nil {
		return nil, err
	}
	methods := make([]PaymentMethod, 0, len(info.Methods))
	for id, method := range info.Methods {
		methods = append(methods, PaymentMethod{ID: id, Currency: method.Currency, Min: method.SingleMin, Max: method.SingleMax})
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].ID < methods[j].ID })
	rate := info.SubscriptionUSDToCNYRate
	if rate < 0 {
		rate = 0
	}
	planCurrency := checkoutPlanCurrency(info.Methods)
	mode := "usage"
	if len(info.Plans) > 0 {
		mode = "period"
	}
	return &ConfigData{Config: Config{Mode: mode, PaymentMethods: methods, DisplayCurrency: planCurrency, USDToCNYRate: rate, BalanceDisabled: info.BalanceDisabled, BalanceRechargeMultiplier: nonNegative(info.BalanceRechargeMultiplier), RechargeFeeRate: nonNegative(info.RechargeFeeRate), GlobalDailyLimitUSD: info.GlobalDailyLimitUSD, GlobalWeeklyLimitUSD: info.GlobalWeeklyLimitUSD, GlobalMonthlyLimitUSD: info.GlobalMonthlyLimitUSD, Plans: plansFromRemote(info.Plans, planCurrency)}, ObservedAt: time.Now().UTC()}, nil
}
func (s *Service) Plans(ctx context.Context, userID uint, sessionID string) (*PlansData, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	info, err := s.checkoutInfo(ctx, userID, sessionID, token)
	if err != nil {
		return nil, err
	}
	return &PlansData{Plans: plansFromRemote(info.Plans, checkoutPlanCurrency(info.Methods)), ObservedAt: time.Now().UTC()}, nil
}
func (s *Service) Overview(ctx context.Context, userID uint, sessionID string) (*OverviewData, error) {
	value, err := s.cachedRead(commerceCacheKey(userID, sessionID, "overview", "current"), overviewCacheTTL, func() (any, error) {
		return s.loadOverview(ctx, userID, sessionID)
	})
	if err != nil {
		return nil, err
	}
	return value.(*OverviewData), nil
}

func (s *Service) loadOverview(ctx context.Context, userID uint, sessionID string) (*OverviewData, error) {
	account, err := s.Account(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	subscriptions, err := s.client.ActiveSubscriptions(ctx, token)
	if err != nil {
		return nil, err
	}
	entitlements := make([]SubscriptionEntitlement, 0, len(subscriptions))
	var current *SubscriptionEntitlement
	now := time.Now().UTC()
	for _, sub := range subscriptions {
		p := planFromSubscription(sub)
		currentFlag := sub.StartsAt.Before(now) && sub.ExpiresAt.After(now)
		entitlement := SubscriptionEntitlement{ID: sub.ID, UserID: userID, Status: sub.Status, StartAt: sub.StartsAt, CurrentPeriodStartAt: sub.StartsAt, CurrentPeriodEndAt: sub.ExpiresAt, Plan: p, IsCurrent: currentFlag}
		entitlements = append(entitlements, entitlement)
		if currentFlag && (current == nil || sub.StartsAt.After(current.StartAt)) {
			current = &entitlements[len(entitlements)-1]
		}
	}
	mode := "usage"
	if len(entitlements) > 0 {
		mode = "period"
	}
	overview := Overview{Mode: mode, Account: account.Account, SubscriptionEntitlements: entitlements}
	if current != nil {
		overview.Plan = &current.Plan
		overview.PeriodStartAt = &current.CurrentPeriodStartAt
		overview.PeriodEndAt = &current.CurrentPeriodEndAt
		for _, sub := range subscriptions {
			if sub.ID == current.ID {
				used := sub.MonthlyUsageUSD
				limit := current.Plan.PeriodCreditUSD
				overview.PeriodCreditUSD = limit
				overview.PeriodCreditNanousd = usdNanousd(limit)
				overview.PeriodUsedUSD = used
				overview.PeriodUsedNanousd = usdNanousd(used)
				overview.PeriodRemainingUSD = math.Max(0, limit-used)
				overview.PeriodRemainingNanousd = usdNanousd(math.Max(0, limit-used))
				break
			}
		}
	}
	return &OverviewData{Overview: overview, ObservedAt: now}, nil
}

func planFromSubscription(sub sub2api.Subscription) Plan {
	plan := Plan{
		Code:            "sub2_group_" + strconv.FormatInt(sub.GroupID, 10),
		Name:            "Subscription",
		FeatureJSON:     "[]",
		ModelScopesJSON: "[]",
		IsActive:        true,
		Prices:          []PlanPrice{},
	}
	if sub.Group == nil {
		return plan
	}
	plan.Name = firstNonEmpty(sub.Group.Name, plan.Name)
	plan.Description = sub.Group.Description
	plan.GroupPlatform = sub.Group.Platform
	plan.RateMultiplier = sub.Group.RateMultiplier
	plan.DailyLimitUSD = sub.Group.DailyLimitUSD
	plan.WeeklyLimitUSD = sub.Group.WeeklyLimitUSD
	plan.MonthlyLimitUSD = sub.Group.MonthlyLimitUSD
	if sub.Group.MonthlyLimitUSD != nil {
		plan.PeriodCreditUSD = *sub.Group.MonthlyLimitUSD
	}
	return plan
}
func plansFromRemote(plans []sub2api.PaymentPlan, currency string) []Plan {
	out := make([]Plan, 0, len(plans))
	for _, plan := range plans {
		out = append(out, planFromRemote(plan, currency))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SortOrder < out[j].SortOrder || (out[i].SortOrder == out[j].SortOrder && out[i].ID < out[j].ID)
	})
	return out
}
func planFromRemote(p sub2api.PaymentPlan, currency string) Plan {
	active := p.Price > 0
	interval := "month"
	if p.ValidityUnit == "year" {
		interval = "year"
	}
	if p.ValidityUnit == "lifetime" {
		interval = "lifetime"
	}
	periodCreditUSD := 0.0
	if p.MonthlyLimitUSD != nil {
		periodCreditUSD = *p.MonthlyLimitUSD
	}
	planCurrency := strings.ToUpper(strings.TrimSpace(p.Currency))
	if planCurrency == "" {
		planCurrency = currency
	}
	return Plan{ID: p.ID, Code: fmtID(p.ID), Name: p.Name, Description: p.Description, FeatureJSON: featureJSON(p.Features), GroupPlatform: p.GroupPlatform, RateMultiplier: p.RateMultiplier, ModelRateMultiplier: p.ModelRateMultiplier, DailyLimitUSD: p.DailyLimitUSD, WeeklyLimitUSD: p.WeeklyLimitUSD, MonthlyLimitUSD: p.MonthlyLimitUSD, PeriodCreditUSD: periodCreditUSD, ValidityDays: p.ValidityDays, OriginalPriceCents: cents(p.OriginalPrice), ModelScopesJSON: featureJSON(p.ModelScopes), SortOrder: p.SortOrder, IsActive: active, Prices: []PlanPrice{{ID: p.ID, PlanID: p.ID, Code: fmtID(p.ID), BillingInterval: interval, Currency: planCurrency, AmountCents: cents(p.Price), IsActive: active, IsDefault: true}}}
}

func checkoutPlanCurrency(methods map[string]sub2api.PaymentMethod) string {
	currency := ""
	for _, method := range methods {
		candidate := strings.ToUpper(strings.TrimSpace(method.Currency))
		if candidate == "" {
			continue
		}
		if currency != "" && currency != candidate {
			return ""
		}
		currency = candidate
	}
	return currency
}

type UsageInput struct {
	Model, BillingType, SortBy, SortOrder string
	Page, PageSize                        int
}

func (s *Service) Usage(ctx context.Context, userID uint, sessionID string, input UsageInput) (*UsageData, error) {
	if input.Page < 1 || input.PageSize < 1 || input.PageSize > 100 || !validUsageInput(input) {
		return nil, ErrInvalidQuery
	}
	key := commerceCacheKey(userID, sessionID, "usage", fmt.Sprintf("%s|%s|%s|%s|%d|%d", input.Model, input.BillingType, input.SortBy, input.SortOrder, input.Page, input.PageSize))
	value, err := s.cachedRead(key, usageCacheTTL, func() (any, error) {
		return s.loadUsage(ctx, userID, sessionID, input)
	})
	if err != nil {
		return nil, err
	}
	return value.(*UsageData), nil
}

func (s *Service) loadUsage(ctx context.Context, userID uint, sessionID string, input UsageInput) (*UsageData, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	data, err := s.client.Usage(ctx, token, sub2api.UsageQuery{Model: input.Model, BillingType: input.BillingType, SortBy: input.SortBy, SortOrder: input.SortOrder, Page: input.Page, PageSize: input.PageSize})
	if err != nil {
		return nil, err
	}
	out := make([]UsageRow, 0, len(data.Items))
	for _, x := range data.Items {
		actualCost, err := exactActualCost(x.ActualCost)
		if err != nil {
			return nil, err
		}
		out = append(out, UsageRow{ID: x.ID, Model: x.Model, InputTokens: x.InputTokens, OutputTokens: x.OutputTokens, TotalTokens: x.TotalTokens, ActualCost: actualCost, DurationMS: x.DurationMS, CreatedAt: x.CreatedAt})
	}
	return &UsageData{Results: out, Total: data.Total, ObservedAt: time.Now().UTC()}, nil
}
func validUsageInput(in UsageInput) bool {
	if len(in.Model) > 128 || (in.BillingType != "" && in.BillingType != "balance" && in.BillingType != "subscription") {
		return false
	}
	if in.SortBy != "" && in.SortBy != "created_at" && in.SortBy != "actual_cost" && in.SortBy != "total_tokens" && in.SortBy != "duration_ms" {
		return false
	}
	return in.SortOrder == "" || in.SortOrder == "asc" || in.SortOrder == "desc"
}

var ErrInvalidQuery = errors.New("invalid query")

func (s *Service) Trend(ctx context.Context, userID uint, sessionID, start, end, granularity string) (*DailyData, error) {
	if granularity != "day" && granularity != "hour" {
		return nil, ErrInvalidQuery
	}
	a, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, ErrInvalidQuery
	}
	b, err := time.Parse("2006-01-02", end)
	if err != nil || b.Before(a) || b.Sub(a) > 366*24*time.Hour {
		return nil, ErrInvalidQuery
	}
	key := commerceCacheKey(userID, sessionID, "trend", start+"|"+end+"|"+granularity)
	value, err := s.cachedRead(key, trendCacheTTL, func() (any, error) {
		return s.loadTrend(ctx, userID, sessionID, start, end, granularity)
	})
	if err != nil {
		return nil, err
	}
	return value.(*DailyData), nil
}

func (s *Service) loadTrend(ctx context.Context, userID uint, sessionID, start, end, granularity string) (*DailyData, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	trend, err := s.client.Trend(ctx, token, start, end, granularity)
	if err != nil {
		return nil, err
	}
	out := make([]DailyRow, 0, len(trend.Trend))
	for _, x := range trend.Trend {
		actualCost, err := exactActualCost(x.ActualCost)
		if err != nil {
			return nil, err
		}
		out = append(out, DailyRow{UsageDate: x.Date, CallCount: x.Requests, RecordCount: x.Requests, InputTokens: x.InputTokens, OutputTokens: x.OutputTokens, CacheReadTokens: x.CacheReadTokens, CacheWriteTokens: x.CacheCreationTokens, TotalTokens: x.TotalTokens, ActualCost: actualCost})
	}
	return &DailyData{Results: out, ObservedAt: time.Now().UTC()}, nil
}
func (s *Service) MonthlyTrend(ctx context.Context, userID uint, sessionID string, months int) (*MonthlyData, error) {
	if months < 1 || months > 24 {
		return nil, ErrInvalidQuery
	}
	now := time.Now().UTC()
	end := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	start := time.Date(now.Year(), now.Month()-time.Month(months)+1, 1, 0, 0, 0, 0, time.UTC)
	daily, err := s.Trend(ctx, userID, sessionID, start.Format("2006-01-02"), end.Format("2006-01-02"), "day")
	if err != nil {
		return nil, err
	}
	buckets, err := aggregateMonthly(daily.Results)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]MonthlyRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, buckets[k])
	}
	return &MonthlyData{Results: out, ObservedAt: time.Now().UTC()}, nil
}
func aggregateMonthly(daily []DailyRow) (map[string]MonthlyRow, error) {
	buckets := map[string]MonthlyRow{}
	for _, row := range daily {
		date, _ := time.Parse("2006-01-02", row.UsageDate)
		monthStart := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
		key := monthStart.Format("2006-01")
		bucket := buckets[key]
		if bucket.MonthStartAt == "" {
			bucket.MonthStartAt = monthStart.Format("2006-01-02")
		}
		bucket.CallCount += row.CallCount
		bucket.RecordCount += row.RecordCount
		bucket.InputTokens += row.InputTokens
		bucket.OutputTokens += row.OutputTokens
		bucket.CacheReadTokens += row.CacheReadTokens
		bucket.CacheWriteTokens += row.CacheWriteTokens
		bucket.TotalTokens += row.TotalTokens
		actualCost, err := addActualCosts(firstNonEmpty(bucket.ActualCost, "0"), firstNonEmpty(row.ActualCost, "0"))
		if err != nil {
			return nil, err
		}
		bucket.ActualCost = actualCost
		buckets[key] = bucket
	}
	return buckets, nil
}
func usdNanousd(v float64) int64 {
	if v <= 0 || v > float64(math.MaxInt64)/1e9 {
		return 0
	}
	return int64(math.Round(v * 1e9))
}
func nonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
func fmtID(id int64) string { return "sub2_" + strconv.FormatInt(id, 10) }
func featureJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	if _, ok := value.(string); ok {
		return string(encoded)
	}
	return string(encoded)
}
