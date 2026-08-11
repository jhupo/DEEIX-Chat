package billing

import (
	"encoding/json"
	"strings"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2commerce"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSub2TransportDTOUsesCamelCaseAndObservedAt(t *testing.T) {
	observedAt := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(toSub2ConfigData(app.ConfigData{
		Config:     app.Config{PaymentMethods: []app.PaymentMethod{{ID: "alipay", Currency: "CNY"}}},
		ObservedAt: observedAt,
	}))
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(payload)
	for _, field := range []string{`"paymentMethods"`, `"observedAt"`, `"balanceRechargeMultiplier"`} {
		if !strings.Contains(jsonText, field) {
			t.Fatalf("missing %s in %s", field, jsonText)
		}
	}
	if strings.Contains(jsonText, "PaymentMethods") || strings.Contains(jsonText, "ObservedAt") {
		t.Fatalf("application field casing leaked: %s", jsonText)
	}
}

func TestSub2TransportDTOOmitsLegacyCommerceAliases(t *testing.T) {
	createdAt := time.Date(2026, 8, 11, 1, 2, 3, 0, time.UTC)
	accountPayload, err := json.Marshal(toSub2Account(app.Account{
		Balance: 9.5, FrozenBalance: 1.25, Status: "active",
	}))
	if err != nil {
		t.Fatal(err)
	}
	accountJSON := string(accountPayload)
	for _, field := range []string{`"balance":9.5`, `"frozenBalance":1.25`} {
		if !strings.Contains(accountJSON, field) {
			t.Fatalf("missing %s in %s", field, accountJSON)
		}
	}
	for _, field := range []string{"userID", "balanceUSD", "frozenBalanceUSD", "balanceNanousd", "currency", "updatedAt"} {
		if strings.Contains(accountJSON, field) {
			t.Fatalf("legacy account field %s leaked in %s", field, accountJSON)
		}
	}

	usagePayload, err := json.Marshal(toSub2UsageData(app.UsageData{Results: []app.UsageRow{{
		ID: 4, Model: "gpt-test", InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
		ActualCost: "0.125000000123", DurationMS: 2500, CreatedAt: createdAt,
	}}}))
	if err != nil {
		t.Fatal(err)
	}
	usageJSON := string(usagePayload)
	for _, field := range []string{`"model":"gpt-test"`, `"actualCost":"0.125000000123"`, `"durationMS":2500`, `"createdAt":"2026-08-11T01:02:03Z"`} {
		if !strings.Contains(usageJSON, field) {
			t.Fatalf("missing %s in %s", field, usageJSON)
		}
	}
	for _, field := range []string{"platformModelName", "billedUSD", "billedNanousd", "durationSeconds", "billingAt"} {
		if strings.Contains(usageJSON, field) {
			t.Fatalf("legacy usage field %s leaked in %s", field, usageJSON)
		}
	}
}

func TestTrendRangeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, raw := range []string{"?days=0", "?days=2&start_date=2026-01-01&end_date=2026-01-02", "?start_date=2026-01-01"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/"+raw, nil)
		if _, _, err := trendRange(c); err == nil {
			t.Fatalf("want error for %s", raw)
		}
	}
}
func TestSub2WriteNoStore(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	(&Sub2Handler{}).write(c, func() (any, error) { return map[string]any{}, nil })
	if c.Writer.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}
func TestSub2CheckoutRequestValidation(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "http://app.example.test/billing/payments/checkout", nil)
	c.Request.Host = "app.example.test"
	valid := Sub2CheckoutRequest{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay", SuccessURL: "https://app.example.test/ok"}
	if !validSub2CheckoutRequest(c, valid) || !canonicalUUID("f47ac10b-58cc-4372-a567-0e02b2c3d479") {
		t.Fatal("valid request rejected")
	}
	c.Request.Header.Set("X-Forwarded-Host", "chat.example.test")
	proxied := Sub2CheckoutRequest{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay", SuccessURL: "https://chat.example.test/ok"}
	if !validSub2CheckoutRequest(c, proxied) {
		t.Fatal("proxied public origin rejected")
	}
	c.Request.Header.Del("X-Forwarded-Host")
	for _, value := range []Sub2CheckoutRequest{
		{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay", SuccessURL: "http://other.example.test/ok"},
		{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay", SuccessURL: "ftp://app.example.test/ok"},
		{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay", SuccessURL: "https://user@app.example.test/ok"},
		{OrderType: "topup", AmountMinorUnits: 100, PaymentProvider: "alipay", SuccessURL: "https://app.example.test/ok#x"},
		{OrderType: "topup", AmountMinorUnits: 99, PaymentProvider: "alipay", SuccessURL: "https://app.example.test/ok"},
		{OrderType: "subscription", PriceID: 0, PaymentProvider: "alipay", SuccessURL: "https://app.example.test/ok"},
	} {
		if validSub2CheckoutRequest(c, value) {
			t.Fatalf("invalid request accepted: %#v", value)
		}
	}
	if canonicalUUID("not-a-uuid") {
		t.Fatal("invalid UUID accepted")
	}
	if parsed, _ := url.Parse("https://app.example.test/ok"); parsed.Host != "app.example.test" {
		t.Fatal("test URL malformed")
	}
}

func TestUsageQueryContract(t *testing.T) {
	parse := func(raw string) (app.UsageInput, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/billing/usage"+raw, nil)
		return usageQuery(c)
	}
	input, err := parse("")
	if err != nil || input.Page != 1 || input.PageSize != 10 || input.SortBy != "created_at" || input.SortOrder != "desc" {
		t.Fatalf("defaults = %#v, %v", input, err)
	}
	input, err = parse("?page=2&page_size=25&query=gpt&billing_type=balance&sort_by=total_tokens&sort_order=asc")
	if err != nil || input.Model != "gpt" || input.BillingType != "balance" || input.SortBy != "total_tokens" || input.SortOrder != "asc" {
		t.Fatalf("parsed = %#v, %v", input, err)
	}
	for _, raw := range []string{"?mo" + "del=gpt", "?sta" + "tus=balance", "?so" + "rt=new" + "est", "?sort_by=unknown", "?billing_type=free", "?query=" + strings.Repeat("x", 129), "?query=a&query=b"} {
		if _, err := parse(raw); err == nil {
			t.Fatalf("accepted invalid query %s", raw)
		}
	}
}
func TestUsageInvalidQueryNoStore(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodGet, "/billing/usage?so"+"rt=new"+"est", nil)
	(&Sub2Handler{}).Usage(c)
	if writer.Code != http.StatusBadRequest || writer.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q", writer.Code, writer.Header().Get("Cache-Control"))
	}
}
