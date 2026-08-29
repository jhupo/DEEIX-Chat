package sub2commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrInvalidWrite           = errors.New("invalid Sub2 commerce request")
	ErrIdempotencyConflict    = errors.New("Sub2 commerce idempotency conflict")
	ErrOutcomeUnknown         = errors.New("Sub2 commerce outcome unknown")
	ErrCheckoutAlreadyCreated = errors.New("Sub2 checkout already created")
)

const (
	paymentOutcomePersistTimeout = 3 * time.Second
	maxQRCodePayloadBytes        = 858
)

type CheckoutInput struct {
	OrderType        string
	PriceID          int64
	AmountMinorUnits int64
	PaymentProvider  string
}
type CheckoutResult struct {
	OrderNo            string
	OrderType          string
	Provider           string
	Status             string
	ExternalCheckoutID string
	CheckoutURL        string
	QRCode             string
	BaseAmountCents    int64
	BaseCurrency       string
	PayAmountCents     int64
	PayCurrency        string
	FXRate             string
	CreditNanousd      int64
	CreditUSD          float64
	ExpiredAt          *time.Time
}
type RedeemResult struct {
	Redemption RedemptionResult
	Account    Account
	Overview   Overview
	ObservedAt time.Time
}

func (s *Service) Checkout(ctx context.Context, userID uint, sessionID, idempotencyKey string, input CheckoutInput) (*CheckoutResult, error) {
	if s.repo == nil || !validUUID(idempotencyKey) || !validCheckout(input) {
		return nil, ErrInvalidWrite
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	info, err := s.checkoutInfo(ctx, userID, sessionID, token)
	if err != nil {
		return nil, err
	}
	method, ok := info.Methods[input.PaymentProvider]
	if !ok {
		return nil, ErrInvalidWrite
	}
	hash := requestHash(input)
	op, claimed, err := s.repo.ClaimPaymentOperation(ctx, userID, idempotencyKey, hash)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return replayPaymentOperation(op, hash)
	}
	order := sub2api.CreatePaymentOrderInput{
		OrderType:     mapOrderType(input.OrderType),
		PaymentType:   input.PaymentProvider,
		PaymentSource: "hosted_redirect",
		IsMobile:      false,
	}
	if input.OrderType == "subscription" {
		order.PlanID = input.PriceID
	} else {
		order.Amount = float64(input.AmountMinorUnits) / 100
	}
	remote, err := s.client.CreatePaymentOrder(ctx, token, order)
	if err != nil {
		return nil, errors.Join(ErrOutcomeUnknown, s.markOutcomeUnknown(ctx, userID, idempotencyKey, ""))
	}
	remoteID := firstNonEmpty(remote.OutTradeNo, strconv.FormatInt(remote.OrderID, 10))
	baseCents := cents(remote.Amount)
	if baseCents == 0 {
		baseCents = input.AmountMinorUnits
	}
	payCents := cents(remote.PayAmount)
	if payCents == 0 {
		payCents = baseCents
	}
	currency := strings.ToUpper(strings.TrimSpace(remote.Currency))
	if currency == "" {
		currency = "USD"
	}
	checkoutURL := trustedCheckoutURL(remote.PayURL)
	qrCode := validQRCodePayload(remote.QRCode)
	fxRate := "1"
	if baseCents > 0 {
		fxRate = strconv.FormatFloat(float64(payCents)/float64(baseCents), 'f', -1, 64)
	}
	creditUSD := float64(baseCents) / 100
	if input.OrderType == "topup" {
		multiplier := info.BalanceRechargeMultiplier
		if multiplier <= 0 {
			multiplier = 1
		}
		creditUSD *= multiplier
	}
	if input.OrderType == "topup" && method.Currency != "" {
		currency = strings.ToUpper(method.Currency)
	}
	result := &CheckoutResult{OrderNo: remoteID, OrderType: input.OrderType, Provider: firstNonEmpty(remote.PaymentType, input.PaymentProvider), Status: firstNonEmpty(remote.Status, "pending"), ExternalCheckoutID: remoteID, CheckoutURL: checkoutURL, QRCode: qrCode, BaseAmountCents: baseCents, BaseCurrency: "USD", PayAmountCents: payCents, PayCurrency: currency, FXRate: fxRate, CreditNanousd: usdNanousd(creditUSD), CreditUSD: creditUSD, ExpiredAt: remote.ExpiresAt}
	if err := s.repo.FinishPaymentOperation(ctx, userID, idempotencyKey, "completed_success", result.ExternalCheckoutID); err != nil {
		return nil, errors.Join(ErrOutcomeUnknown, s.markOutcomeUnknown(ctx, userID, idempotencyKey, result.ExternalCheckoutID))
	}
	return result, nil
}
func (s *Service) Redeem(ctx context.Context, userID uint, sessionID, code string) (*RedeemResult, error) {
	if len(strings.TrimSpace(code)) < 3 || len(strings.TrimSpace(code)) > 64 {
		return nil, ErrInvalidWrite
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	item, err := s.client.Redeem(ctx, token, code)
	if err != nil {
		return nil, err
	}
	s.invalidateReadCache(userID, sessionID)
	overview, err := s.Overview(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	account, err := s.Account(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	return &RedeemResult{Redemption: RedemptionResult{ID: item.ID, Type: item.Type, Value: item.Value}, Account: account.Account, Overview: overview.Overview, ObservedAt: time.Now().UTC()}, nil
}
func replayPaymentOperation(op *repository.Sub2PaymentOperation, hash string) (*CheckoutResult, error) {
	if op == nil {
		return nil, ErrOutcomeUnknown
	}
	if op.RequestHash != hash {
		return nil, ErrIdempotencyConflict
	}
	if op.State == "outcome_unknown" || op.State == "send_started" {
		return nil, ErrOutcomeUnknown
	}
	if op.State != "completed_success" {
		return nil, ErrInvalidWrite
	}
	return nil, ErrCheckoutAlreadyCreated
}
func (s *Service) markOutcomeUnknown(ctx context.Context, userID uint, key, remoteID string) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), paymentOutcomePersistTimeout)
	defer cancel()
	return s.repo.FinishPaymentOperation(persistCtx, userID, key, "outcome_unknown", remoteID)
}
func validCheckout(in CheckoutInput) bool {
	if strings.TrimSpace(in.PaymentProvider) == "" || len(in.PaymentProvider) > 32 {
		return false
	}
	if in.OrderType == "subscription" {
		return in.PriceID > 0 && in.AmountMinorUnits == 0
	}
	return in.OrderType == "topup" && in.AmountMinorUnits >= 100 && in.AmountMinorUnits <= math.MaxInt32
}
func mapOrderType(value string) string {
	if value == "topup" {
		return "balance"
	}
	return "subscription"
}
func requestHash(input CheckoutInput) string {
	raw, _ := json.Marshal(input)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validUUID(value string) bool {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil && parsed.String() == strings.ToLower(strings.TrimSpace(value))
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func cents(value float64) int64 { return int64(math.Round(value * 100)) }
func trustedCheckoutURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

func validQRCodePayload(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxQRCodePayloadBytes || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}
