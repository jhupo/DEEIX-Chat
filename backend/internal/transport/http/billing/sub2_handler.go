package billing

import (
	"errors"
	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2commerce"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Sub2Handler struct{ service *app.Service }

type ErrorDoc struct {
	ErrorMsg string `json:"errorMsg"`
}

type Sub2ConfigResponseDoc struct {
	ErrorMsg string            `json:"errorMsg"`
	Data     sub2ConfigDataDTO `json:"data"`
}
type Sub2AccountResponseDoc struct {
	ErrorMsg string             `json:"errorMsg"`
	Data     sub2AccountDataDTO `json:"data"`
}
type Sub2OverviewResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     sub2OverviewDataDTO `json:"data"`
}
type Sub2PlansResponseDoc struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     sub2PlansDataDTO `json:"data"`
}
type Sub2UsageResponseDoc struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     sub2UsageDataDTO `json:"data"`
}
type Sub2DailyResponseDoc struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     sub2DailyDataDTO `json:"data"`
}
type Sub2HourlyResponseDoc struct {
	ErrorMsg string            `json:"errorMsg"`
	Data     sub2HourlyDataDTO `json:"data"`
}
type Sub2MonthlyResponseDoc struct {
	ErrorMsg string             `json:"errorMsg"`
	Data     sub2MonthlyDataDTO `json:"data"`
}
type Sub2CheckoutResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     sub2CheckoutDataDTO `json:"data"`
}
type Sub2RedeemResponseDoc struct {
	ErrorMsg string                `json:"errorMsg"`
	Data     sub2RedemptionDataDTO `json:"data"`
}
type Sub2OrdersResponseDoc struct {
	ErrorMsg string            `json:"errorMsg"`
	Data     sub2OrdersDataDTO `json:"data"`
}
type Sub2OrderResponseDoc struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     sub2OrderDataDTO `json:"data"`
}
type Sub2RedemptionsResponseDoc struct {
	ErrorMsg string                 `json:"errorMsg"`
	Data     sub2RedemptionsDataDTO `json:"data"`
}

func NewSub2Handler(service *app.Service) *Sub2Handler { return &Sub2Handler{service: service} }
func (h *Sub2Handler) write(c *gin.Context, fn func() (any, error)) {
	c.Header("Cache-Control", "no-store")
	data, err := fn()
	if err != nil {
		response.Error(c, http.StatusBadGateway, "Sub2 service unavailable")
		return
	}
	response.Success(c, data)
}

// Config godoc
// @Summary Get Sub2 commerce configuration
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2ConfigResponseDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/config [get]
func (h *Sub2Handler) Config(c *gin.Context) {
	h.write(c, func() (any, error) {
		data, err := h.service.Config(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c))
		if err != nil {
			return nil, err
		}
		return toSub2ConfigData(*data), nil
	})
}

// Account godoc
// @Summary Get Sub2 account balance
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2AccountResponseDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/account [get]
func (h *Sub2Handler) Account(c *gin.Context) {
	h.write(c, func() (any, error) {
		data, err := h.service.Account(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c))
		if err != nil {
			return nil, err
		}
		return sub2AccountDataDTO{Account: toSub2Account(data.Account), ObservedAt: data.ObservedAt}, nil
	})
}

// Overview godoc
// @Summary Get Sub2 subscription overview
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2OverviewResponseDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/overview [get]
func (h *Sub2Handler) Overview(c *gin.Context) {
	h.write(c, func() (any, error) {
		data, err := h.service.Overview(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c))
		if err != nil {
			return nil, err
		}
		return sub2OverviewDataDTO{Overview: toSub2Overview(data.Overview), ObservedAt: data.ObservedAt}, nil
	})
}

// Plans godoc
// @Summary List Sub2 commerce plans
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2PlansResponseDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/plans [get]
func (h *Sub2Handler) Plans(c *gin.Context) {
	h.write(c, func() (any, error) {
		data, err := h.service.Plans(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c))
		if err != nil {
			return nil, err
		}
		return sub2PlansDataDTO{Plans: toSub2Plans(data.Plans), ObservedAt: data.ObservedAt}, nil
	})
}

// Usage godoc
// @Summary List Sub2 usage
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page"
// @Param page_size query int false "Page size"
// @Success 200 {object} Sub2UsageResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/usage [get]
func (h *Sub2Handler) Usage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	input, err := usageQuery(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid query")
		return
	}
	h.write(c, func() (any, error) {
		data, err := h.service.Usage(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), input)
		if err != nil {
			return nil, err
		}
		return toSub2UsageData(*data), nil
	})
}

// Daily godoc
// @Summary Get Sub2 daily usage
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2DailyResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/usage/daily [get]
func (h *Sub2Handler) Daily(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	start, end, err := trendRange(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid query")
		return
	}
	h.write(c, func() (any, error) {
		data, err := h.service.Trend(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), start, end, "day")
		if err != nil {
			return nil, err
		}
		return toSub2DailyData(*data), nil
	})
}

// Hourly godoc
// @Summary Get Sub2 hourly usage
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2HourlyResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/usage/hourly [get]
func (h *Sub2Handler) Hourly(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	start, end, err := trendRange(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid query")
		return
	}
	h.write(c, func() (any, error) {
		data, err := h.service.Trend(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), start, end, "hour")
		if err != nil {
			return nil, err
		}
		return toSub2HourlyData(*data), nil
	})
}

// Monthly godoc
// @Summary Get Sub2 monthly usage
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Param months query int false "Months"
// @Success 200 {object} Sub2MonthlyResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/usage/monthly [get]
func (h *Sub2Handler) Monthly(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	months := 12
	var err error
	if c.Query("months") != "" {
		months, err = strconv.Atoi(c.Query("months"))
	}
	if err != nil || months < 1 || months > 24 {
		response.Error(c, http.StatusBadRequest, "invalid query")
		return
	}
	h.write(c, func() (any, error) {
		data, err := h.service.MonthlyTrend(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), months)
		if err != nil {
			return nil, err
		}
		return toSub2MonthlyData(*data), nil
	})
}

type Sub2CheckoutRequest struct {
	OrderType        string `json:"orderType"`
	PriceID          int64  `json:"priceID"`
	AmountMinorUnits int64  `json:"amountMinorUnits"`
	Cycles           *int   `json:"cycles"`
	PaymentProvider  string `json:"paymentProvider"`
}
type Sub2RedeemRequest struct {
	Code string `json:"code"`
}
type Sub2VerifyOrderRequest struct {
	OperationID string `json:"operationID"`
}
type Sub2RefundRequest struct {
	Reason string `json:"reason"`
}

// Orders godoc
// @Summary List current Sub2 payment orders
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2OrdersResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/orders [get]
func (h *Sub2Handler) Orders(c *gin.Context) {
	allowed := map[string]struct{}{"page": {}, "page_size": {}, "status": {}, "order_type": {}, "payment_type": {}}
	for key, values := range c.Request.URL.Query() {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			response.Error(c, http.StatusBadRequest, "invalid order query")
			return
		}
	}
	page, pageSize, err := pagination(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid order query")
		return
	}
	data, err := h.service.Orders(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), app.OrdersInput{Page: page, PageSize: pageSize, Status: strings.TrimSpace(c.Query("status")), OrderType: strings.TrimSpace(c.Query("order_type")), PaymentType: strings.TrimSpace(c.Query("payment_type"))})
	if err != nil {
		h.writeWriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, toSub2OrdersData(*data))
}

// Order godoc
// @Summary Get a current-user Sub2 payment order
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Param id path int true "Order ID"
// @Success 200 {object} Sub2OrderResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/orders/{id} [get]
func (h *Sub2Handler) Order(c *gin.Context) {
	id, err := positiveOrderID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid order id")
		return
	}
	data, err := h.service.Order(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), id)
	if err != nil {
		h.writeWriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, toSub2OrderData(*data))
}

// VerifyOrder godoc
// @Summary Verify a Sub2 payment after returning from checkout
// @Tags billing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body Sub2VerifyOrderRequest true "Payment operation"
// @Success 200 {object} Sub2OrderResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/orders/verify [post]
func (h *Sub2Handler) VerifyOrder(c *gin.Context) {
	var req Sub2VerifyOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil || !canonicalUUID(req.OperationID) {
		response.Error(c, http.StatusBadRequest, "invalid order verification")
		return
	}
	data, err := h.service.VerifyOrder(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), req.OperationID)
	if err != nil {
		h.writeWriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, toSub2OrderData(*data))
}

// CancelOrder godoc
// @Summary Cancel a pending Sub2 payment order
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Param id path int true "Order ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/orders/{id}/cancel [post]
func (h *Sub2Handler) CancelOrder(c *gin.Context) {
	id, err := positiveOrderID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid order id")
		return
	}
	if err = h.service.CancelOrder(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), id); err != nil {
		h.writeWriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"cancelled": true})
}

// RequestRefund godoc
// @Summary Request a refund for a Sub2 payment order
// @Tags billing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Order ID"
// @Param body body Sub2RefundRequest true "Refund request"
// @Success 200 {object} map[string]any
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/orders/{id}/refund-request [post]
func (h *Sub2Handler) RequestRefund(c *gin.Context) {
	id, err := positiveOrderID(c.Param("id"))
	var req Sub2RefundRequest
	if err != nil || c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Reason) == "" || len(strings.TrimSpace(req.Reason)) > 500 {
		response.Error(c, http.StatusBadRequest, "invalid refund request")
		return
	}
	if err = h.service.RequestRefund(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), id, req.Reason); err != nil {
		h.writeWriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"requested": true})
}

// RedemptionHistory godoc
// @Summary List current user's Sub2 redemption history
// @Tags billing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} Sub2RedemptionsResponseDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/redemptions [get]
func (h *Sub2Handler) RedemptionHistory(c *gin.Context) {
	data, err := h.service.RedemptionHistory(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c))
	if err != nil {
		h.writeWriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, toSub2RedemptionsData(*data))
}

// Checkout godoc
// @Summary Create Sub2 payment order
// @Tags billing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param Idempotency-Key header string true "UUID"
// @Param body body Sub2CheckoutRequest true "Checkout request"
// @Success 200 {object} Sub2CheckoutResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 409 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/payments/checkout [post]
func (h *Sub2Handler) Checkout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req Sub2CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validSub2CheckoutRequest(req) || !canonicalUUID(c.GetHeader("Idempotency-Key")) {
		response.Error(c, http.StatusBadRequest, "invalid checkout request")
		return
	}
	result, err := h.service.Checkout(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), strings.TrimSpace(c.GetHeader("Idempotency-Key")), app.CheckoutInput{OrderType: req.OrderType, PriceID: req.PriceID, AmountMinorUnits: req.AmountMinorUnits, PaymentProvider: req.PaymentProvider})
	if err != nil {
		h.writeWriteError(c, err)
		return
	}
	response.Success(c, sub2CheckoutDataDTO{Checkout: toSub2Checkout(*result), ObservedAt: time.Now().UTC()})
}

// Redeem godoc
// @Summary Redeem a Sub2 code
// @Tags billing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body Sub2RedeemRequest true "Redeem request"
// @Success 200 {object} Sub2RedeemResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 502 {object} ErrorDoc
// @Router /billing/redemptions [post]
func (h *Sub2Handler) Redeem(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req Sub2RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(strings.TrimSpace(req.Code)) < 3 || len(strings.TrimSpace(req.Code)) > 64 {
		response.Error(c, http.StatusBadRequest, "invalid redemption code")
		return
	}
	result, err := h.service.Redeem(c.Request.Context(), middleware.MustUserID(c), middleware.MustSessionID(c), req.Code)
	if err != nil {
		h.writeWriteError(c, err)
		return
	}
	response.Success(c, toSub2RedemptionData(*result))
}
func (h *Sub2Handler) writeWriteError(c *gin.Context, err error) {
	if errors.Is(err, app.ErrInvalidWrite) {
		response.Error(c, http.StatusBadRequest, "invalid billing request")
		return
	}
	if errors.Is(err, app.ErrIdempotencyConflict) {
		response.Error(c, http.StatusConflict, "idempotency key conflicts with request")
		return
	}
	if errors.Is(err, app.ErrCheckoutAlreadyCreated) {
		response.Error(c, http.StatusConflict, "checkout operation already created")
		return
	}
	if errors.Is(err, app.ErrOutcomeUnknown) {
		response.Error(c, http.StatusBadGateway, "payment outcome unknown")
		return
	}
	response.Error(c, http.StatusBadGateway, "Sub2 service unavailable")
}
func validSub2CheckoutRequest(req Sub2CheckoutRequest) bool {
	if req.Cycles != nil && *req.Cycles != 1 {
		return false
	}
	if req.OrderType != "subscription" && req.OrderType != "topup" {
		return false
	}
	if len(strings.TrimSpace(req.PaymentProvider)) < 1 || len(strings.TrimSpace(req.PaymentProvider)) > 32 {
		return false
	}
	if req.OrderType == "subscription" && (req.PriceID <= 0 || req.AmountMinorUnits != 0) {
		return false
	}
	if req.OrderType == "topup" && (req.AmountMinorUnits < 100 || req.AmountMinorUnits > 2147483647) {
		return false
	}
	return true
}
func canonicalUUID(raw string) bool {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.String() == strings.ToLower(strings.TrimSpace(raw))
}
func positiveOrderID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid order id")
	}
	return id, nil
}
func pagination(c *gin.Context) (int, int, error) {
	p := 1
	s := 10
	var err error
	if c.Query("page") != "" {
		p, err = strconv.Atoi(c.Query("page"))
	}
	if err == nil && c.Query("page_size") != "" {
		s, err = strconv.Atoi(c.Query("page_size"))
	}
	if err != nil || p < 1 || s < 1 || s > 100 {
		return 0, 0, errors.New("invalid")
	}
	return p, s, nil
}
func usageQuery(c *gin.Context) (app.UsageInput, error) {
	allowed := map[string]struct{}{"page": {}, "page_size": {}, "query": {}, "billing_type": {}, "sort_by": {}, "sort_order": {}}
	for key, values := range c.Request.URL.Query() {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			return app.UsageInput{}, errors.New("invalid usage query")
		}
	}
	page, size, err := pagination(c)
	if err != nil {
		return app.UsageInput{}, err
	}
	input := app.UsageInput{Page: page, PageSize: size, Model: strings.TrimSpace(c.Query("query")), BillingType: strings.TrimSpace(c.Query("billing_type")), SortBy: "created_at", SortOrder: "desc"}
	if len(input.Model) > 128 {
		return app.UsageInput{}, errors.New("invalid model query")
	}
	if input.BillingType != "" && input.BillingType != "balance" && input.BillingType != "subscription" {
		return app.UsageInput{}, errors.New("invalid billing type")
	}
	if raw := strings.TrimSpace(c.Query("sort_by")); raw != "" {
		input.SortBy = raw
	}
	if raw := strings.TrimSpace(c.Query("sort_order")); raw != "" {
		input.SortOrder = raw
	}
	if input.SortBy != "created_at" && input.SortBy != "actual_cost" && input.SortBy != "total_tokens" && input.SortBy != "duration_ms" {
		return app.UsageInput{}, errors.New("invalid sort")
	}
	if input.SortOrder != "asc" && input.SortOrder != "desc" {
		return app.UsageInput{}, errors.New("invalid sort order")
	}
	return input, nil
}
func trendRange(c *gin.Context) (string, string, error) {
	start := c.Query("start_date")
	end := c.Query("end_date")
	days := strings.TrimSpace(c.Query("days"))
	if days != "" {
		if start != "" || end != "" {
			return "", "", errors.New("invalid")
		}
		count, err := strconv.Atoi(days)
		if err != nil || count < 1 || count > 366 {
			return "", "", errors.New("invalid")
		}
		now := time.Now().UTC()
		return now.AddDate(0, 0, -count+1).Format("2006-01-02"), now.Format("2006-01-02"), nil
	}
	if (start == "") != (end == "") {
		return "", "", errors.New("invalid")
	}
	if start == "" {
		now := time.Now().UTC()
		end = now.Format("2006-01-02")
		start = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	return start, end, nil
}
