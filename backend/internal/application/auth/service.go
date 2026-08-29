package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	appstorage "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/objectstorage"
	userapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/userview"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/secretbox"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/token"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const accessTokenSessionClockSkew = 2 * time.Minute

type Service struct {
	cfg                 *config.Runtime
	repo                repository.AuthRepository
	geoResolver         geoResolver
	logger              *zap.Logger
	storeProvider       appstorage.Provider
	auditWriter         auditWriter
	avatarFileValidator avatarFileValidator
	sub2                sub2api.Client
	sub2RefreshLocks    sessionKeyedLock
}

type sessionLockKey struct {
	userID    uint
	sessionID string
}

type sessionLockEntry struct {
	mu   sync.Mutex
	refs int
}

// sessionKeyedLock serializes work for one browser session without retaining idle keys.
type sessionKeyedLock struct {
	mu      sync.Mutex
	entries map[sessionLockKey]*sessionLockEntry
}

func (l *sessionKeyedLock) lock(userID uint, sessionID string) func() {
	key := sessionLockKey{userID: userID, sessionID: sessionID}
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[sessionLockKey]*sessionLockEntry)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &sessionLockEntry{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}

type auditWriter interface {
	Write(context.Context, string, uint, string, string, string, string, string, interface{})
}
type avatarFileValidator interface {
	ValidateImageFile(context.Context, uint, string) error
}
type geoResolver interface {
	Lookup(context.Context, string) (requestmeta.SessionAuditContext, error)
}
type AuditInput struct {
	UserID                                                       uint
	RequestID, Action, Resource, ResourceID, ClientIP, UserAgent string
	Detail                                                       interface{}
}
type UpdateProfileInput struct{ AvatarURL, DisplayName, Timezone, Locale, ProfilePreferences, AppearancePreferences *string }
type UpdateCurrentSessionLocationInput struct {
	Latitude, Longitude float64
	AccuracyMeters      *float64
	Timezone            string
}
type issuedTokens struct {
	AccessToken, RefreshToken, AccessJTI string
	ExpiresAt, RefreshExpiresAt          time.Time
}

func NewServiceWithRuntime(cfg *config.Runtime, repo repository.AuthRepository, geo geoResolver, sub2 sub2api.Client) (*Service, error) {
	if sub2 == nil || sub2.InstanceID() == "" {
		return nil, ErrSub2ClientRequired
	}
	return &Service{cfg: cfg, repo: repo, geoResolver: geo, sub2: sub2, storeProvider: appstorage.NewRuntimeProvider(cfg, nil)}, nil
}
func (s *Service) SetLogger(v *zap.Logger) { s.logger = v }
func (s *Service) SetObjectStoreProvider(v appstorage.Provider) {
	if v != nil {
		s.storeProvider = v
	}
}
func (s *Service) SetAvatarFileValidator(v avatarFileValidator) { s.avatarFileValidator = v }
func (s *Service) SetAuditWriter(v auditWriter)                 { s.auditWriter = v }
func (s *Service) ShouldUseSecureCookies() bool {
	if s == nil || s.cfg == nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(s.cfg.Snapshot().Env))
	return v == "prod" || v == "production"
}
func (s *Service) RecordAudit(ctx context.Context, in AuditInput) {
	if s.auditWriter != nil {
		s.auditWriter.Write(ctx, strings.TrimSpace(in.RequestID), in.UserID, strings.TrimSpace(in.Action), strings.TrimSpace(in.Resource), strings.TrimSpace(in.ResourceID), strings.TrimSpace(in.ClientIP), strings.TrimSpace(in.UserAgent), in.Detail)
	}
}
func (s *Service) warn(msg string, fs ...zap.Field) {
	if s.logger != nil {
		s.logger.Warn(msg, fs...)
	}
}
func (s *Service) Login(ctx context.Context, email, password, turnstileToken, requestID string, audit requestmeta.SessionAuditContext) (*LoginResult, error) {
	principal, lookupErr := s.repo.FindByLoginIdentifier(ctx, email)
	if lookupErr != nil && !errors.Is(lookupErr, repository.ErrNotFound) {
		return nil, lookupErr
	}
	if lookupErr == nil && principal != nil && principal.AuthProvider == domainuser.AuthProviderLocal {
		if principal.Status != domainuser.StatusActive || bcrypt.CompareHashAndPassword([]byte(principal.PasswordHash), []byte(password)) != nil {
			s.RecordAuthEvent(ctx, principal.ID, requestID, "login", "failure", "local_rejected", audit.ClientIP, audit.UserAgent, "")
			return nil, ErrInvalidCredentials
		}
		result, issueErr := s.issueLoginResultWithSub2(ctx, principal, s.resolveSessionAuditContext(ctx, audit), time.Now(), nil)
		if issueErr != nil {
			return nil, issueErr
		}
		s.RecordAuthEvent(ctx, principal.ID, requestID, "login", "success", "", audit.ClientIP, audit.UserAgent, marshalAuthEventDetail(map[string]string{"authority": "local"}))
		return result, nil
	}
	return s.loginWithSub2(ctx, email, password, turnstileToken, requestID, s.resolveSessionAuditContext(ctx, audit))
}
func (s *Service) issueLoginResultWithSub2(ctx context.Context, u *domainuser.User, audit requestmeta.SessionAuditContext, now time.Time, creds *sub2SessionCredentials) (*LoginResult, error) {
	id := uuid.NewString()
	pair, err := s.buildSessionTokenPair(u, id, now)
	if err != nil {
		return nil, err
	}
	snap := buildSessionAuditSnapshot(audit)
	v := &domainuser.Session{
		SessionID:        id,
		UserID:           u.ID,
		RefreshTokenHash: hashToken(pair.RefreshToken),
		AccessJTI:        pair.AccessJTI,
		ClientIP:         snap.ClientIP,
		UserAgent:        snap.UserAgent,
		DeviceName:       snap.DeviceName,
		BrowserName:      snap.BrowserName,
		OSName:           snap.OSName,
		DeviceType:       snap.DeviceType,
		GeoSource:        snap.GeoSource,
		GeoAccuracy:      snap.GeoAccuracy,
		CountryCode:      snap.CountryCode,
		RegionName:       snap.RegionName,
		CityName:         snap.CityName,
		TimezoneName:     snap.TimezoneName,
		IPLatitude:       snap.IPLatitude,
		IPLongitude:      snap.IPLongitude,
		IssuedAt:         now,
		LastSeenAt:       &now,
		ExpiresAt:        pair.RefreshExpiresAt,
	}
	if creds != nil {
		v.Sub2AccessTokenEncrypted = creds.AccessTokenEncrypted
		v.Sub2RefreshTokenEncrypted = creds.RefreshTokenEncrypted
		v.Sub2AccessExpiresAt = &creds.AccessExpiresAt
		v.Sub2VerifiedAt = &creds.VerifiedAt
	}
	if err = s.repo.CreateSession(ctx, v); err != nil {
		return nil, err
	}
	if err = s.repo.UpdateLastLogin(ctx, u.ID); err != nil {
		return nil, err
	}
	return &LoginResult{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, SessionID: id, ExpiresAt: &pair.ExpiresAt, RefreshExpiresAt: &pair.RefreshExpiresAt}, nil
}
func (s *Service) GetProfile(ctx context.Context, id uint) (*domainuser.User, error) {
	return s.repo.GetByID(ctx, id)
}
func (s *Service) GetVerifiedProfile(ctx context.Context, id uint, sid string) (*domainuser.User, error) {
	u, e := s.repo.GetByID(ctx, id)
	if e != nil {
		return nil, e
	}
	v, e := s.repo.GetSessionByUserAndSessionID(ctx, id, strings.TrimSpace(sid))
	if e != nil {
		return nil, e
	}
	if isLocalPrincipal(u) {
		if !isActiveLocalPrincipal(u) || v.RevokedAt != nil || time.Now().After(v.ExpiresAt) {
			return nil, ErrSessionRevoked
		}
		return u, nil
	}
	return s.verifySub2SessionProfile(ctx, u, v)
}
func (s *Service) buildUserView(ctx context.Context, u domainuser.User) (userview.UserView, error) {
	return userview.FromUser(u), nil
}
func (s *Service) BuildUserView(ctx context.Context, u domainuser.User) (userview.UserView, error) {
	return s.buildUserView(ctx, u)
}
func (s *Service) UpdateProfile(ctx context.Context, id uint, in UpdateProfileInput) (*domainuser.User, error) {
	v := repository.UpdateUserFieldsInput{}
	if in.AvatarURL != nil {
		x := strings.TrimSpace(*in.AvatarURL)
		if file, ok := domainuser.ParseFileAvatarURL(x); ok {
			if s.avatarFileValidator == nil || s.avatarFileValidator.ValidateImageFile(ctx, id, file) != nil {
				return nil, ErrInvalidAvatarURL
			}
		}
		v.AvatarURL = &x
	}
	if in.DisplayName != nil {
		x, e := userapp.NormalizeDisplayName(*in.DisplayName)
		if e != nil {
			return nil, e
		}
		v.DisplayName = &x
	}
	if in.Timezone != nil {
		x := strings.TrimSpace(*in.Timezone)
		if x == "" {
			x = "Etc/UTC"
		}
		if _, e := time.LoadLocation(x); e != nil {
			return nil, ErrInvalidTimeZone
		}
		v.Timezone = &x
	}
	if in.Locale != nil {
		x := strings.ReplaceAll(strings.TrimSpace(*in.Locale), "_", "-")
		if x == "" {
			x = "en-US"
		}
		if x != "en" && x != "en-US" && x != "zh" && x != "zh-CN" {
			return nil, ErrInvalidLocale
		}
		v.Locale = &x
	}
	if in.ProfilePreferences != nil {
		x := strings.TrimSpace(*in.ProfilePreferences)
		v.ProfilePreferences = &x
	}
	if in.AppearancePreferences != nil {
		x := strings.TrimSpace(*in.AppearancePreferences)
		if x != "" && !json.Valid([]byte(x)) {
			return nil, ErrInvalidAppearancePreferences
		}
		v.AppearancePreferences = &x
	}
	return s.repo.UpdateProfile(ctx, id, v)
}
func (s *Service) resolveSessionAuditContext(ctx context.Context, a requestmeta.SessionAuditContext) requestmeta.SessionAuditContext {
	a = a.Normalize()
	if s.geoResolver == nil || a.CountryCode != "" || a.CityName != "" {
		return a
	}
	v, e := s.geoResolver.Lookup(ctx, a.ClientIP)
	if e == nil {
		return mergeSessionAuditContext(a, v)
	}
	return a
}
func mergeSessionAuditContext(a, b requestmeta.SessionAuditContext) requestmeta.SessionAuditContext {
	a = a.Normalize()
	b = b.Normalize()
	if a.CountryCode == "" {
		a.CountryCode = b.CountryCode
	}
	if a.RegionName == "" {
		a.RegionName = b.RegionName
	}
	if a.CityName == "" {
		a.CityName = b.CityName
	}
	if a.TimezoneName == "" {
		a.TimezoneName = b.TimezoneName
	}
	if a.IPLatitude == nil {
		a.IPLatitude = b.IPLatitude
	}
	if a.IPLongitude == nil {
		a.IPLongitude = b.IPLongitude
	}
	return a
}
func (s *Service) Refresh(ctx context.Context, raw, requestID string, a requestmeta.SessionAuditContext) (*LoginResult, error) {
	claims, e := token.Parse(s.cfg.Snapshot().JWTSecret, strings.TrimSpace(raw))
	if e != nil || claims.TokenType != "refresh" || claims.UserID == 0 || claims.SessionID == "" {
		return nil, ErrInvalidRefreshToken
	}
	release := s.sub2RefreshLocks.lock(claims.UserID, claims.SessionID)
	defer release()
	v, e := s.repo.GetSessionByUserAndSessionID(ctx, claims.UserID, claims.SessionID)
	if errors.Is(e, repository.ErrNotFound) {
		return nil, ErrInvalidRefreshToken
	}
	if e != nil {
		return nil, e
	}
	if v == nil || v.RevokedAt != nil || time.Now().After(v.ExpiresAt) || subtle.ConstantTimeCompare([]byte(v.RefreshTokenHash), []byte(hashToken(raw))) != 1 {
		return nil, ErrInvalidRefreshToken
	}
	u, e := s.repo.GetByID(ctx, claims.UserID)
	if e != nil {
		return nil, e
	}
	refreshedUser := u
	if isLocalPrincipal(u) {
		if !isActiveLocalPrincipal(u) {
			return nil, ErrSessionRevoked
		}
	} else {
		refreshedUser, e = s.refreshSub2SessionProfile(ctx, u, v)
	}
	if e != nil {
		if errors.Is(e, ErrInvalidRefreshToken) || errors.Is(e, ErrInvalidCredentials) || errors.Is(e, ErrSessionRevoked) {
			if revokeErr := s.repo.RevokeSession(ctx, u.ID, v.SessionID, "sub2_invalid_identity"); revokeErr != nil && !errors.Is(revokeErr, repository.ErrInvalidInput) {
				return nil, revokeErr
			}
		}
		return nil, e
	}
	u = refreshedUser
	now := time.Now()
	pair, e := s.buildSessionTokenPair(u, claims.SessionID, now)
	if e != nil {
		return nil, e
	}
	if e = s.repo.RotateSessionTokens(ctx, repository.RotateSessionTokensInput{
		UserID:               u.ID,
		SessionID:            claims.SessionID,
		PresentedRefreshHash: hashToken(raw),
		NextRefreshHash:      hashToken(pair.RefreshToken),
		NextAccessJTI:        pair.AccessJTI,
		IssuedAt:             now,
		ExpiresAt:            pair.RefreshExpiresAt,
	}); e != nil {
		if errors.Is(e, repository.ErrInvalidInput) {
			return nil, ErrSessionRevoked
		}
		return nil, e
	}
	return &LoginResult{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, SessionID: claims.SessionID, ExpiresAt: &pair.ExpiresAt, RefreshExpiresAt: &pair.RefreshExpiresAt}, nil
}
func (s *Service) Logout(ctx context.Context, id uint, sid, requestID string, a requestmeta.SessionAuditContext) error {
	v, e := s.repo.GetSessionByUserAndSessionID(ctx, id, sid)
	if e != nil {
		return ErrSessionRevoked
	}
	r, _ := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, v.Sub2RefreshTokenEncrypted)
	if e = s.repo.RevokeSession(ctx, id, sid, "user_logout"); e != nil {
		return e
	}
	if r != "" {
		if user, userErr := s.repo.GetByID(ctx, id); userErr == nil && isLocalPrincipal(user) {
			r = ""
		}
	}
	if r != "" {
		_ = s.sub2.Logout(ctx, r)
	}
	s.RecordAuthEvent(ctx, id, requestID, "logout", "success", "", a.ClientIP, a.UserAgent, "")
	return nil
}
func (s *Service) LogoutAll(ctx context.Context, id uint, requestID string, a requestmeta.SessionAuditContext) error {
	sessions, e := s.repo.ListActiveSessionsByUserID(ctx, id, time.Now())
	if e != nil {
		return e
	}
	refreshTokens := make([]string, 0, len(sessions))
	for _, session := range sessions {
		refreshToken, decryptErr := secretbox.DecryptString(s.cfg.Snapshot().DataEncryptionKey, session.Sub2RefreshTokenEncrypted)
		if decryptErr == nil && strings.TrimSpace(refreshToken) != "" {
			refreshTokens = append(refreshTokens, refreshToken)
		}
	}
	if e := s.repo.RevokeAllSessions(ctx, id, "user_logout_all"); e != nil {
		return e
	}
	user, userErr := s.repo.GetByID(ctx, id)
	if userErr == nil && !isLocalPrincipal(user) {
		for _, refreshToken := range refreshTokens {
			_ = s.sub2.Logout(ctx, refreshToken)
		}
	}
	s.RecordAuthEvent(ctx, id, requestID, "logout_all", "success", "", a.ClientIP, a.UserAgent, "")
	return nil
}
func (s *Service) ValidateAccessSession(ctx context.Context, id uint, sid string, issued time.Time, a requestmeta.SessionAuditContext) (*domainuser.User, error) {
	if id == 0 || sid == "" || issued.IsZero() {
		return nil, ErrSessionRevoked
	}
	v, e := s.repo.GetSessionByUserAndSessionID(ctx, id, sid)
	if errors.Is(e, repository.ErrNotFound) {
		return nil, ErrSessionRevoked
	}
	if e != nil {
		return nil, e
	}
	if v == nil || v.RevokedAt != nil || time.Now().After(v.ExpiresAt) || issued.Add(accessTokenSessionClockSkew).Before(v.CreatedAt) {
		return nil, ErrSessionRevoked
	}
	u, e := s.repo.GetByID(ctx, id)
	if e != nil {
		return nil, e
	}
	if isLocalPrincipal(u) {
		if !isActiveLocalPrincipal(u) {
			return nil, ErrSessionRevoked
		}
	} else {
		u, e = s.ensureSub2Session(ctx, id, v)
	}
	if e != nil {
		return nil, e
	}
	return u, nil
}
func (s *Service) ListCurrentActiveSessions(ctx context.Context, id uint, current string) ([]ActiveSessionResult, error) {
	rows, e := s.repo.ListActiveSessionsByUserID(ctx, id, time.Now())
	if e != nil {
		return nil, e
	}
	out := make([]ActiveSessionResult, 0, len(rows))
	for _, v := range rows {
		out = append(out, ActiveSessionResult{SessionID: v.SessionID, Current: v.SessionID == current, DeviceLabel: resolveSessionDeviceLabel(&v), DeviceName: v.DeviceName, BrowserName: v.BrowserName, OSName: v.OSName, DeviceType: v.DeviceType, ClientIP: v.ClientIP, LocationLabel: resolveSessionLocationLabel(&v), CountryCode: v.CountryCode, RegionName: v.RegionName, CityName: v.CityName, TimezoneName: v.TimezoneName, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, LastSeenAt: v.LastSeenAt, ExpiresAt: v.ExpiresAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Current || out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Service) UpdateCurrentSessionLocation(ctx context.Context, id uint, sid, requestID string, a requestmeta.SessionAuditContext, in UpdateCurrentSessionLocationInput) (*ActiveSessionResult, error) {
	if in.Latitude < -90 || in.Latitude > 90 || in.Longitude < -180 || in.Longitude > 180 {
		return nil, ErrInvalidLocation
	}
	now := time.Now()
	if e := s.repo.TouchSessionActivity(ctx, id, sid, repository.UpdateSessionActivityInput{PreciseLatitude: &in.Latitude, PreciseLongitude: &in.Longitude, PreciseAccuracyM: in.AccuracyMeters, PreciseLocatedAt: &now, LastSeenAt: &now}); e != nil {
		return nil, e
	}
	rows, e := s.ListCurrentActiveSessions(ctx, id, sid)
	if e != nil {
		return nil, e
	}
	for _, v := range rows {
		if v.SessionID == sid {
			return &v, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (s *Service) RecordAuthEvent(ctx context.Context, id uint, requestID, eventType, result, reason, ip, agent, detail string) {
	if e := s.repo.RecordAuthEvent(ctx, id, requestID, eventType, result, reason, ip, agent, detail); e != nil {
		s.warn("record_auth_event_failed", zap.Error(e))
	}
}
func (s *Service) buildSessionTokenPair(u *domainuser.User, sid string, now time.Time) (*issuedTokens, error) {
	c := s.cfg.Snapshot()
	accessTTL := time.Duration(c.TokenTTLHours) * time.Hour
	if accessTTL <= 0 {
		accessTTL = 24 * time.Hour
	}
	refreshTTL := time.Duration(c.RefreshTokenTTLHours) * time.Hour
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	accessJTI := uuid.NewString()
	refreshJTI := uuid.NewString()
	access, e := token.GenerateWithClaims(c.JWTSecret, u.ID, u.Username, u.Role, sid, accessJTI, "access", accessTTL)
	if e != nil {
		return nil, e
	}
	refresh, e := token.GenerateWithClaims(c.JWTSecret, u.ID, u.Username, u.Role, sid, refreshJTI, "refresh", refreshTTL)
	if e != nil {
		return nil, e
	}
	return &issuedTokens{AccessToken: access, RefreshToken: refresh, AccessJTI: accessJTI, ExpiresAt: now.Add(accessTTL), RefreshExpiresAt: now.Add(refreshTTL)}, nil
}
func hashToken(v string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(v)))
	return hex.EncodeToString(h[:])
}
func marshalAuthEventDetail(v interface{}) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	return string(b)
}
