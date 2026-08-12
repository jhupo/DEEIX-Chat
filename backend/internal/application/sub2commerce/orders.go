package sub2commerce

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type OrdersInput struct {
	Page, PageSize                 int
	Status, OrderType, PaymentType string
}
type OrdersData struct {
	Results    []Order
	Total      int64
	Page       int
	PageSize   int
	ObservedAt time.Time
}
type OrderData struct {
	Order      Order
	ObservedAt time.Time
}
type Order struct {
	ID                  int64
	AmountUSD           float64
	PayAmount           float64
	FeeRate             float64
	Currency            string
	PaymentType         string
	OrderNo             string
	Status              string
	OrderType           string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	PaidAt              *time.Time
	CompletedAt         *time.Time
	RefundAmount        float64
	RefundReason        *string
	RefundRequestedAt   *time.Time
	RefundRequestReason *string
	PlanID              *int64
}
type RedemptionsData struct {
	Results    []RedemptionHistory
	ObservedAt time.Time
}
type RedemptionHistory struct {
	ID           int64
	Code         string
	Type         string
	Value        float64
	Status       string
	UsedAt       *time.Time
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	GroupID      *int64
	ValidityDays int
}

func (s *Service) Orders(ctx context.Context, userID uint, sessionID string, input OrdersInput) (*OrdersData, error) {
	if input.Page < 1 || input.PageSize < 1 || input.PageSize > 100 ||
		!validOrderFilter(input.Status) || !validOrderFilter(input.PaymentType) ||
		(input.OrderType != "" && input.OrderType != "balance" && input.OrderType != "subscription") {
		return nil, ErrInvalidWrite
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	page, err := s.client.PaymentOrders(ctx, token, sub2api.PaymentOrderQuery{
		Page: input.Page, PageSize: input.PageSize, Status: input.Status,
		OrderType: input.OrderType, PaymentType: input.PaymentType,
	})
	if err != nil {
		return nil, err
	}
	results := make([]Order, len(page.Items))
	for i := range page.Items {
		results[i] = orderFromRemote(page.Items[i])
	}
	return &OrdersData{Results: results, Total: page.Total, Page: page.Page, PageSize: page.PageSize, ObservedAt: time.Now().UTC()}, nil
}

func (s *Service) Order(ctx context.Context, userID uint, sessionID string, id int64) (*OrderData, error) {
	if id <= 0 {
		return nil, ErrInvalidWrite
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	item, err := s.client.PaymentOrder(ctx, token, id)
	if err != nil {
		return nil, err
	}
	return &OrderData{Order: orderFromRemote(*item), ObservedAt: time.Now().UTC()}, nil
}

func (s *Service) VerifyOrder(ctx context.Context, userID uint, sessionID, operationID string) (*OrderData, error) {
	operationID = strings.TrimSpace(operationID)
	if !validUUID(operationID) {
		return nil, ErrInvalidWrite
	}
	operation, err := s.repo.GetPaymentOperation(ctx, userID, operationID)
	if errors.Is(err, repository.ErrNotFound) || operation == nil || (operation.State != "completed_success" && operation.State != "outcome_unknown") || strings.TrimSpace(operation.ExternalOrderID) == "" {
		return nil, ErrInvalidWrite
	}
	if err != nil {
		return nil, err
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	item, err := s.client.VerifyPaymentOrder(ctx, token, operation.ExternalOrderID)
	if err != nil {
		return nil, err
	}
	s.invalidateReadCache(userID, sessionID)
	if operation.State == "outcome_unknown" {
		if err := s.repo.FinishPaymentOperation(ctx, userID, operationID, "completed_success", operation.ExternalOrderID); err != nil {
			return nil, ErrOutcomeUnknown
		}
	}
	return &OrderData{Order: orderFromRemote(*item), ObservedAt: time.Now().UTC()}, nil
}

func (s *Service) CancelOrder(ctx context.Context, userID uint, sessionID string, id int64) error {
	if id <= 0 {
		return ErrInvalidWrite
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	return s.client.CancelPaymentOrder(ctx, token, id)
}

func (s *Service) RequestRefund(ctx context.Context, userID uint, sessionID string, id int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if id <= 0 || reason == "" || len(reason) > 500 {
		return ErrInvalidWrite
	}
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	return s.client.RequestPaymentRefund(ctx, token, id, reason)
}

func (s *Service) RedemptionHistory(ctx context.Context, userID uint, sessionID string) (*RedemptionsData, error) {
	token, err := s.tokens.Sub2AccessTokenForSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	items, err := s.client.RedemptionHistory(ctx, token)
	if err != nil {
		return nil, err
	}
	results := make([]RedemptionHistory, len(items))
	for i, item := range items {
		results[i] = RedemptionHistory{ID: item.ID, Code: item.Code, Type: item.Type, Value: item.Value, Status: item.Status, UsedAt: item.UsedAt, CreatedAt: item.CreatedAt, ExpiresAt: item.ExpiresAt, GroupID: item.GroupID, ValidityDays: item.ValidityDays}
	}
	return &RedemptionsData{Results: results, ObservedAt: time.Now().UTC()}, nil
}

func validOrderFilter(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 32 {
		return false
	}
	for _, ch := range value {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func orderFromRemote(item sub2api.PaymentOrder) Order {
	return Order{
		ID: item.ID, AmountUSD: item.Amount, PayAmount: item.PayAmount, FeeRate: item.FeeRate,
		Currency: item.Currency, PaymentType: item.PaymentType, OrderNo: item.OutTradeNo,
		Status: item.Status, OrderType: item.OrderType, CreatedAt: item.CreatedAt,
		ExpiresAt: item.ExpiresAt, PaidAt: item.PaidAt, CompletedAt: item.CompletedAt,
		RefundAmount: item.RefundAmount, RefundReason: item.RefundReason,
		RefundRequestedAt: item.RefundRequestedAt, RefundRequestReason: item.RefundRequestReason,
		PlanID: item.PlanID,
	}
}
