package sub2commerce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

type overviewTokenResolver struct{}

func writeKeyEnvelope(w http.ResponseWriter, value any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": value})
}

func (overviewTokenResolver) Sub2AccessTokenForSession(context.Context, uint, string) (string, error) {
	return "token", nil
}

type atomicPaymentRepo struct {
	mu                    sync.Mutex
	operations            map[string]*repository.Sub2PaymentOperation
	failNextCompleteWrite bool
	cancelBeforeComplete  context.CancelFunc
}

func (r *atomicPaymentRepo) ClaimPaymentOperation(_ context.Context, principal uint, key, hash string) (*repository.Sub2PaymentOperation, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.operations == nil {
		r.operations = map[string]*repository.Sub2PaymentOperation{}
	}
	compound := strconv.FormatUint(uint64(principal), 10) + ":" + key
	if existing := r.operations[compound]; existing != nil {
		value := *existing
		return &value, false, nil
	}
	item := &repository.Sub2PaymentOperation{RequestHash: hash, State: "send_started"}
	r.operations[compound] = item
	value := *item
	return &value, true, nil
}
func (r *atomicPaymentRepo) GetPaymentOperation(_ context.Context, principal uint, key string) (*repository.Sub2PaymentOperation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item := r.operations[strconv.FormatUint(uint64(principal), 10)+":"+key]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, repository.ErrNotFound
}
func (r *atomicPaymentRepo) FinishPaymentOperation(ctx context.Context, principal uint, key, state, remoteID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	item := r.operations[strconv.FormatUint(uint64(principal), 10)+":"+key]
	if item == nil || (item.State != "send_started" && item.State != "outcome_unknown") {
		return repository.ErrNotFound
	}
	if state == "completed_success" && r.failNextCompleteWrite {
		r.failNextCompleteWrite = false
		return errors.New("temporary database failure")
	}
	if state == "completed_success" && r.cancelBeforeComplete != nil {
		r.cancelBeforeComplete()
		r.cancelBeforeComplete = nil
		return ctx.Err()
	}
	item.State, item.ExternalOrderID = state, remoteID
	return nil
}

func TestCheckoutPersistsKnownRemoteOrderAfterRequestCancellation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payment/checkout-info", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"methods": map[string]any{"alipay": map[string]any{}}})
	})
	mux.HandleFunc("/api/v1/payment/orders", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"order_id": 100, "out_trade_no": "trade-cancelled", "pay_url": "https://pay.example.test/cancelled"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	repo := &atomicPaymentRepo{cancelBeforeComplete: cancel}
	service := NewService(overviewTokenResolver{}, client, repo)
	key := "f47ac10b-58cc-4372-a567-0e02b2c3d483"
	input := CheckoutInput{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay"}
	if _, err = service.Checkout(ctx, 1, "s", key, input); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("checkout error = %v", err)
	}
	operation, err := repo.GetPaymentOperation(context.Background(), 1, key)
	if err != nil || operation.State != "outcome_unknown" || operation.ExternalOrderID != "trade-cancelled" {
		t.Fatalf("cancelled request lost remote order: %#v err=%v", operation, err)
	}
}

func TestCheckoutRecoversKnownRemoteOrderAfterCompletionWriteFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payment/checkout-info", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"methods": map[string]any{"alipay": map[string]any{}}})
	})
	mux.HandleFunc("/api/v1/payment/orders", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"order_id": 99, "out_trade_no": "trade-recover", "pay_url": "https://pay.example.test/recover"})
	})
	mux.HandleFunc("/api/v1/payment/orders/verify", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"id": 99, "out_trade_no": "trade-recover", "status": "pending"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	repo := &atomicPaymentRepo{failNextCompleteWrite: true}
	service := NewService(overviewTokenResolver{}, client, repo)
	key := "f47ac10b-58cc-4372-a567-0e02b2c3d481"
	input := CheckoutInput{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay"}
	if _, err = service.Checkout(context.Background(), 1, "s", key, input); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("checkout error = %v", err)
	}
	operation, err := repo.GetPaymentOperation(context.Background(), 1, key)
	if err != nil || operation.State != "outcome_unknown" || operation.ExternalOrderID != "trade-recover" {
		t.Fatalf("operation lost remote order: %#v err=%v", operation, err)
	}
	verified, err := service.VerifyOrder(context.Background(), 1, "s", key)
	if err != nil || verified.Order.OrderNo != "trade-recover" {
		t.Fatalf("verify: result=%#v err=%v", verified, err)
	}
	operation, err = repo.GetPaymentOperation(context.Background(), 1, key)
	if err != nil || operation.State != "completed_success" || operation.ExternalOrderID != "trade-recover" {
		t.Fatalf("operation did not converge: %#v err=%v", operation, err)
	}
}
func TestCheckoutIdempotencyStateMachine(t *testing.T) {
	var posts int
	failing := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payment/checkout-info", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"methods": map[string]any{"alipay": map[string]any{}}})
	})
	mux.HandleFunc("/api/v1/payment/orders", func(w http.ResponseWriter, r *http.Request) {
		posts++
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatalf("decode QR checkout request: %#v", request)
		}
		_, hasReturnURL := request["return_url"]
		if request["is_mobile"] != false || request["payment_source"] != "hosted_redirect" || hasReturnURL {
			t.Fatalf("unexpected QR checkout request: %#v", request)
		}
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeKeyEnvelope(w, map[string]any{"order_id": 99, "out_trade_no": "trade-99", "pay_url": "https://pay.example.test/99", "client_secret": "secret"})
	})
	mux.HandleFunc("/api/v1/payment/orders/verify", func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if json.NewDecoder(r.Body).Decode(&input) != nil || input["out_trade_no"] != "trade-99" {
			t.Fatalf("unexpected verify request: %#v", input)
		}
		writeKeyEnvelope(w, map[string]any{"id": 99, "out_trade_no": "trade-99", "status": "completed"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	repo := &atomicPaymentRepo{}
	service := NewService(overviewTokenResolver{}, client, repo)
	key := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	input := CheckoutInput{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay"}
	first, err := service.Checkout(context.Background(), 1, "s", key, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExternalCheckoutID != "trade-99" || first.CheckoutURL == "" || first.CreditUSD != 1 {
		t.Fatalf("bad checkout %#v", first)
	}
	verified, err := service.VerifyOrder(context.Background(), 1, "s", key)
	if err != nil || verified.Order.OrderNo != "trade-99" || verified.Order.Status != "completed" {
		t.Fatalf("verify order: result=%#v err=%v", verified, err)
	}
	if _, err = service.Checkout(context.Background(), 1, "s", key, input); !errors.Is(err, ErrCheckoutAlreadyCreated) || posts != 1 {
		t.Fatalf("replay err=%v posts=%d", err, posts)
	}
	if _, err = service.Checkout(context.Background(), 1, "s", key, CheckoutInput{OrderType: "topup", AmountMinorUnits: 200, PaymentProvider: "alipay"}); !errors.Is(err, ErrIdempotencyConflict) || posts != 1 {
		t.Fatalf("conflict err=%v posts=%d", err, posts)
	}
	failing = true
	unknownKey := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	if _, err = service.Checkout(context.Background(), 1, "s", unknownKey, input); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("unknown error %v", err)
	}
	if _, err = service.Checkout(context.Background(), 1, "s", unknownKey, input); !errors.Is(err, ErrOutcomeUnknown) || posts != 2 {
		t.Fatalf("unknown replay err=%v posts=%d", err, posts)
	}
}

func TestQRCodePayloadIsKeptSeparateFromCheckoutURL(t *testing.T) {
	if got := validQRCodePayload("weixin://wxpay/example"); got != "weixin://wxpay/example" {
		t.Fatalf("QR payload = %q", got)
	}
	if got := trustedCheckoutURL("weixin://wxpay/example"); got != "" {
		t.Fatalf("custom QR scheme became checkout URL: %q", got)
	}
	if got := validQRCodePayload("bad\nvalue"); got != "" {
		t.Fatalf("control characters accepted: %q", got)
	}
	if got := validQRCodePayload(strings.Repeat("x", maxQRCodePayloadBytes+1)); got != "" {
		t.Fatalf("oversized QR payload accepted: %d bytes", len(got))
	}
}

func TestCheckoutCachePrunesExpiredEntriesAndStaysBounded(t *testing.T) {
	now := time.Now()
	service := &Service{checkoutCache: make(map[string]checkoutCacheEntry)}
	service.checkoutCache["expired"] = checkoutCacheEntry{expiresAt: now.Add(-time.Second)}
	for i := 0; i < checkoutCacheMax+8; i++ {
		service.checkoutCache[strconv.Itoa(i)] = checkoutCacheEntry{expiresAt: now.Add(time.Duration(i+1) * time.Second)}
	}

	service.pruneCheckoutCacheLocked(now, 0)
	if _, found := service.checkoutCache["expired"]; found {
		t.Fatal("expired checkout entry remained cached")
	}
	if len(service.checkoutCache) != checkoutCacheMax {
		t.Fatalf("cache size = %d, want %d", len(service.checkoutCache), checkoutCacheMax)
	}
}

func TestAggregateMonthlyUsesUTCMonthStart(t *testing.T) {
	rows := []DailyRow{
		{UsageDate: "2026-01-31", CallCount: 1, RecordCount: 1, InputTokens: 2, OutputTokens: 3, TotalTokens: 5, ActualCost: "1.000000000001"},
		{UsageDate: "2026-01-30", CallCount: 1, RecordCount: 1, InputTokens: 1, OutputTokens: 1, TotalTokens: 2, ActualCost: "0.000000000002"},
		{UsageDate: "2026-02-01", CallCount: 2, RecordCount: 2, InputTokens: 4, OutputTokens: 6, TotalTokens: 10, ActualCost: "2.000000000002"},
	}
	got, err := aggregateMonthly(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got["2026-01"].MonthStartAt != "2026-01-01" || got["2026-02"].CallCount != 2 {
		t.Fatalf("unexpected aggregation: %#v", got)
	}
	if got["2026-01"].ActualCost != "1.000000000003" || got["2026-02"].ActualCost != "2.000000000002" {
		t.Fatalf("cost precision lost: %#v", got)
	}
}

func TestExactActualCostSupportsJSONExponentWithoutPrecisionLoss(t *testing.T) {
	got, err := exactActualCost(json.Number("1.23456789123e-4"))
	if err != nil || got != "0.000123456789123" {
		t.Fatalf("actual cost = %q, %v", got, err)
	}
}

func TestUsageMapsAuthoritativeSub2CostAndTrend(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/usage", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{
			"total": 1,
			"items": []any{map[string]any{
				"id": 4, "model": "gpt-test", "input_tokens": 10, "output_tokens": 5,
				"total_tokens": 15, "actual_cost": 0.125000000123, "duration_ms": 2500,
				"created_at": "2026-08-11T01:02:03Z",
			}},
		})
	})
	mux.HandleFunc("/api/v1/usage/dashboard/trend", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{
			"start_date": "2026-08-01", "end_date": "2026-08-11", "granularity": "day",
			"trend": []any{map[string]any{
				"date": "2026-08-11", "requests": 2, "input_tokens": 20, "output_tokens": 10,
				"total_tokens": 30, "actual_cost": 0.250000000456,
			}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(overviewTokenResolver{}, client, nil)
	usage, err := service.Usage(context.Background(), 1, "s", UsageInput{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(usage.Results) != 1 || usage.Results[0].Model != "gpt-test" || usage.Results[0].ActualCost != "0.125000000123" || usage.Results[0].DurationMS != 2500 {
		t.Fatalf("bad usage mapping: %#v", usage.Results)
	}
	trend, err := service.Trend(context.Background(), 1, "s", "2026-08-01", "2026-08-11", "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Results) != 1 || trend.Results[0].CallCount != 2 || trend.Results[0].ActualCost != "0.250000000456" {
		t.Fatalf("bad trend mapping: %#v", trend.Results)
	}
}

func TestTrendUsesSub2HourlyGranularity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/usage/dashboard/trend", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("granularity"); got != "hour" {
			t.Fatalf("granularity = %q", got)
		}
		writeKeyEnvelope(w, map[string]any{
			"granularity": "hour",
			"trend":       []any{map[string]any{"date": "2026-08-12 09:00", "requests": 1, "total_tokens": 12, "actual_cost": 0.1}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewService(overviewTokenResolver{}, client, nil).Trend(context.Background(), 1, "s", "2026-08-12", "2026-08-12", "hour")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].UsageDate != "2026-08-12 09:00" || result.Results[0].TotalTokens != 12 {
		t.Fatalf("hourly trend = %#v", result.Results)
	}
}

func TestOverviewMapsSub2Entitlement(t *testing.T) {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}
	mux.HandleFunc("/api/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"id": 7, "balance": 9.5, "status": "active"})
	})
	mux.HandleFunc("/api/v1/subscriptions/active", func(w http.ResponseWriter, r *http.Request) {
		write(w, []any{map[string]any{
			"id": 12, "group_id": 3, "starts_at": "2026-01-01T00:00:00Z", "expires_at": "2099-02-01T00:00:00Z",
			"status": "active", "monthly_usage_usd": 2.5,
			"group": map[string]any{"name": "Pro Group", "description": "Group entitlement", "platform": "openai", "monthly_limit_usd": 5},
		}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewService(overviewTokenResolver{}, client, nil).Overview(context.Background(), 1, "s")
	if err != nil {
		t.Fatal(err)
	}
	overview := result.Overview
	if overview.Account.Balance != 9.5 || overview.Plan == nil || overview.Plan.ID != 0 || overview.Plan.Name != "Pro Group" || overview.Plan.PeriodCreditUSD != 5 || len(overview.Plan.Prices) != 0 || !overview.Plan.IsActive {
		t.Fatalf("bad overview %#v", overview)
	}
	ent := overview.SubscriptionEntitlements[0]
	if !ent.IsCurrent || ent.PlanID != 0 || ent.PriceID != 0 || ent.Plan.Name != "Pro Group" || overview.PeriodCreditUSD != 5 || overview.PeriodUsedUSD != 2.5 || overview.PeriodRemainingUSD != 2.5 {
		t.Fatalf("bad entitlement %#v", overview)
	}
}

func TestPlansPreservePerPlanCurrency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payment/checkout-info", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{
			"methods": map[string]any{
				"alipay": map[string]any{"currency": "CNY"},
				"stripe": map[string]any{"currency": "USD"},
			},
			"plans": []any{
				map[string]any{"id": 1, "group_id": 3, "name": "Monthly", "price": 68, "currency": "CNY", "validity_unit": "month"},
				map[string]any{"id": 2, "group_id": 3, "name": "Annual", "price": 99, "currency": "USD", "validity_unit": "year"},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewService(overviewTokenResolver{}, client, nil).Plans(context.Background(), 1, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plans) != 2 || result.Plans[0].Prices[0].Currency != "CNY" || result.Plans[1].Prices[0].Currency != "USD" {
		t.Fatalf("plan currencies = %#v", result.Plans)
	}
}

func TestOverviewDoesNotGuessPlanFromSameGroupCatalog(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/user/profile", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"id": 7, "status": "active"})
	})
	mux.HandleFunc("/api/v1/subscriptions/active", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, []any{map[string]any{
			"id": 12, "group_id": 3, "starts_at": "2026-01-01T00:00:00Z", "expires_at": "2099-02-01T00:00:00Z", "status": "active",
			"group": map[string]any{"name": "Pro Group", "monthly_limit_usd": 5},
		}})
	})
	mux.HandleFunc("/api/v1/payment/checkout-info", func(w http.ResponseWriter, _ *http.Request) {
		writeKeyEnvelope(w, map[string]any{"plans": []any{
			map[string]any{"id": 9, "group_id": 3, "name": "Monthly", "price": 10, "currency": "USD"},
			map[string]any{"id": 10, "group_id": 3, "name": "Annual", "price": 100, "currency": "USD"},
		}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := sub2api.New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(overviewTokenResolver{}, client, nil)
	if _, err := service.Plans(context.Background(), 1, "s"); err != nil {
		t.Fatal(err)
	}
	result, err := service.Overview(context.Background(), 1, "s")
	if err != nil {
		t.Fatal(err)
	}
	entitlement := result.Overview.SubscriptionEntitlements[0]
	if entitlement.PlanID != 0 || entitlement.PriceID != 0 || entitlement.Plan.Name != "Pro Group" || len(entitlement.Plan.Prices) != 0 {
		t.Fatalf("subscription guessed a checkout plan: %#v", entitlement)
	}
}
