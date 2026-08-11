package repository

import "errors"

var (
	ErrNotFound                      = errors.New("record not found")
	ErrDuplicate                     = errors.New("duplicate record")
	ErrConflict                      = errors.New("resource conflict")
	ErrInvalidInput                  = errors.New("invalid input")
	ErrInsufficientBalance           = errors.New("insufficient balance")
	ErrUsageReservationLimitExceeded = errors.New("usage reservation limit exceeded")
	ErrRedemptionUnavailable         = errors.New("redemption unavailable")
	ErrRedemptionExhausted           = errors.New("redemption exhausted")
	ErrRedemptionUserLimitExceeded   = errors.New("redemption user limit exceeded")
	ErrUpstreamNotFound              = errors.New("upstream not found")
	ErrModelNotFound                 = errors.New("model not found")
	ErrDuplicatePlatformModelName    = errors.New("duplicate platform model name")
	ErrUpstreamModelNotFound         = errors.New("upstream model not found")
	ErrUpstreamModelConflict         = errors.New("upstream model conflict")
	ErrLLMSettingNotFound            = errors.New("llm setting not found")
)
