package repository

import "context"

type Sub2PaymentOperation struct {
	RequestHash     string
	State           string
	ExternalOrderID string
}

type Sub2PaymentOperationRepository interface {
	ClaimPaymentOperation(context.Context, uint, string, string) (*Sub2PaymentOperation, bool, error)
	GetPaymentOperation(context.Context, uint, string) (*Sub2PaymentOperation, error)
	FinishPaymentOperation(context.Context, uint, string, string, string) error
}
