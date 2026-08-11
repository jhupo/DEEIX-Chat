package auth

import "errors"

var (
	ErrInvalidCredentials           = errors.New("invalid email or password")
	ErrInvalidTimeZone              = errors.New("invalid time zone")
	ErrInvalidLocale                = errors.New("invalid user locale")
	ErrInvalidAppearancePreferences = errors.New("invalid appearance preferences")
	ErrInvalidAvatarURL             = errors.New("invalid avatar url")
	ErrInvalidLocation              = errors.New("invalid location")
	ErrInvalidRefreshToken          = errors.New("invalid refresh token")
	ErrSessionRevoked               = errors.New("session revoked")
	ErrTwoFactorChallengeExpired    = errors.New("two factor challenge expired")
	ErrSub2ClientRequired           = errors.New("Sub2API client is required")
	ErrSub2Unavailable              = errors.New("Sub2API unavailable")
	ErrEmailAlreadyExists           = errors.New("email already exists")
	ErrEmailRegistrationDisabled    = errors.New("email registration is disabled")
	ErrEmailVerificationRequired    = errors.New("email verification is required")
	ErrVerificationCodeInvalid      = errors.New("verification code is invalid or expired")
	ErrEmailDomainNotAllowed        = errors.New("email domain is not allowed")
	ErrHumanVerificationFailed      = errors.New("turnstile verification failed")
	ErrInvalidCurrentPassword       = errors.New("invalid current password")
)
