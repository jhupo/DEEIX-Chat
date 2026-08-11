package user

import (
	"context"
	"strings"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }
func errFor(err error) error {
	if dberror.IsRecordNotFound(err) {
		return repository.ErrNotFound
	}
	if dberror.IsUniqueConstraint(err) {
		return repository.ErrDuplicate
	}
	return err
}
func toDomainUser(v model.User) *domainuser.User {
	return &domainuser.User{ID: v.ID, Sub2InstanceID: v.Sub2InstanceID, Sub2UserID: v.Sub2UserID, PublicID: v.PublicID, Username: v.Username, DisplayName: v.DisplayName, AvatarURL: v.AvatarURL, Email: v.Email, Role: v.Role, Status: v.Status, Timezone: v.Timezone, Locale: v.Locale, ProfilePreferences: v.ProfilePreferences, AppearancePreferences: v.AppearancePreferences, LastLoginAt: v.LastLoginAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toModelUser(v *domainuser.User) *model.User {
	return &model.User{BaseModel: model.BaseModel{ID: v.ID}, Sub2InstanceID: v.Sub2InstanceID, Sub2UserID: v.Sub2UserID, PublicID: v.PublicID, Username: v.Username, DisplayName: v.DisplayName, AvatarURL: v.AvatarURL, Email: v.Email, Role: v.Role, Status: v.Status, Timezone: v.Timezone, Locale: v.Locale}
}
func (r *Repo) GetByID(ctx context.Context, id uint) (*domainuser.User, error) {
	var v model.User
	if err := r.db.WithContext(ctx).First(&v, id).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainUser(v), nil
}
func (r *Repo) GetByPublicID(ctx context.Context, id string) (*domainuser.User, error) {
	var v model.User
	if err := r.db.WithContext(ctx).Where("public_id = ?", id).First(&v).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainUser(v), nil
}
func (r *Repo) UpsertSub2Principal(ctx context.Context, in *domainuser.User) (*domainuser.User, error) {
	if in == nil || in.Sub2UserID <= 0 || strings.TrimSpace(in.Sub2InstanceID) == "" {
		return nil, repository.ErrInvalidInput
	}
	var out model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sub2_instance_id = ? AND sub2_user_id = ?", in.Sub2InstanceID, in.Sub2UserID).First(&out).Error
		if dberror.IsRecordNotFound(err) {
			out = *toModelUser(in)
			return tx.Create(&out).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&out).Updates(map[string]any{
			"email":  in.Email,
			"role":   in.Role,
			"status": in.Status,
		}).Error
	})
	if err != nil {
		return nil, errFor(err)
	}
	return r.GetByID(ctx, out.ID)
}
func (r *Repo) UpdateProfile(ctx context.Context, id uint, in repository.UpdateUserFieldsInput) (*domainuser.User, error) {
	if in.IsZero() {
		return r.GetByID(ctx, id)
	}
	updates := map[string]any{}
	if in.AvatarURL != nil {
		updates["avatar_url"] = *in.AvatarURL
	}
	if in.DisplayName != nil {
		updates["display_name"] = *in.DisplayName
	}
	if in.Timezone != nil {
		updates["timezone"] = *in.Timezone
	}
	if in.Locale != nil {
		updates["locale"] = *in.Locale
	}
	if in.ProfilePreferences != nil {
		updates["profile_preferences"] = *in.ProfilePreferences
	}
	if in.AppearancePreferences != nil {
		updates["appearance_preferences"] = *in.AppearancePreferences
	}
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, errFor(err)
	}
	return r.GetByID(ctx, id)
}
func (r *Repo) ListUsers(ctx context.Context, off, limit int, f repository.UserListFilter) ([]domainuser.User, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.User{})
	if v := strings.TrimSpace(f.Query); v != "" {
		q = q.Where(
			"public_id ILIKE ? OR username ILIKE ? OR email ILIKE ? OR display_name ILIKE ?",
			"%"+v+"%",
			"%"+v+"%",
			"%"+v+"%",
			"%"+v+"%",
		)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, errFor(err)
	}
	var rows []model.User
	if err := q.Order("id DESC").Offset(off).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, errFor(err)
	}
	out := make([]domainuser.User, 0, len(rows))
	for _, v := range rows {
		out = append(out, *toDomainUser(v))
	}
	return out, total, nil
}
func (r *Repo) UpdateLastLogin(ctx context.Context, id uint) error {
	return errFor(r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("last_login_at", time.Now()).Error)
}
func (r *Repo) ListLatestSessionActivityByUserIDs(ctx context.Context, ids []uint) (map[uint]time.Time, error) {
	out := map[uint]time.Time{}
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		UserID uint
		Seen   time.Time
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.UserSession{}).Select("user_id, MAX(COALESCE(last_seen_at, created_at)) AS seen").Where("user_id IN ?", ids).Group("user_id").Scan(&rows).Error
	for _, v := range rows {
		out[v.UserID] = v.Seen
	}
	return out, errFor(err)
}
func (r *Repo) RecordAuthEvent(ctx context.Context, id uint, requestID, eventType, result, reason, ip, agent, detail string) error {
	return errFor(r.db.WithContext(ctx).Create(&model.UserAuthEvent{UserID: id, RequestID: requestID, EventType: eventType, Result: result, Reason: reason, ClientIP: ip, UserAgent: agent, DetailJSON: detail, OccurredAt: time.Now()}).Error)
}
func (r *Repo) CreateSession(ctx context.Context, in *domainuser.Session) error {
	return errFor(r.db.WithContext(ctx).Create(toModelSession(in)).Error)
}
func (r *Repo) GetSessionByUserAndSessionID(ctx context.Context, id uint, sid string) (*domainuser.Session, error) {
	var v model.UserSession
	if err := r.db.WithContext(ctx).Where("user_id = ? AND session_id = ?", id, sid).First(&v).Error; err != nil {
		return nil, errFor(err)
	}
	return toDomainSession(v), nil
}
func (r *Repo) RotateSessionTokens(ctx context.Context, in repository.RotateSessionTokensInput) error {
	q := r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("user_id = ? AND session_id = ? AND refresh_token_hash = ? AND revoked_at IS NULL", in.UserID, in.SessionID, in.PresentedRefreshHash).
		Updates(map[string]any{
			"refresh_token_hash": in.NextRefreshHash,
			"access_jti":         in.NextAccessJTI,
			"issued_at":          in.IssuedAt,
			"expires_at":         in.ExpiresAt,
		})
	if q.Error != nil {
		return errFor(q.Error)
	}
	if q.RowsAffected == 0 {
		return repository.ErrInvalidInput
	}
	return nil
}
func (r *Repo) UpdateSessionSub2Tokens(ctx context.Context, in repository.UpdateSessionSub2TokensInput) error {
	return r.updateSessionSub2Tokens(ctx, in, true)
}

func (r *Repo) StageSessionSub2Tokens(ctx context.Context, in repository.UpdateSessionSub2TokensInput) error {
	return r.updateSessionSub2Tokens(ctx, in, false)
}

func (r *Repo) updateSessionSub2Tokens(ctx context.Context, in repository.UpdateSessionSub2TokensInput, verified bool) error {
	updates := map[string]any{
		"sub2_access_token_encrypted":  in.AccessTokenEncrypted,
		"sub2_refresh_token_encrypted": in.RefreshTokenEncrypted,
		"sub2_access_expires_at":       in.AccessExpiresAt,
	}
	if verified {
		updates["sub2_verified_at"] = in.VerifiedAt
	}
	q := r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("user_id = ? AND session_id = ? AND revoked_at IS NULL", in.UserID, in.SessionID).
		Updates(updates)
	if q.Error != nil {
		return errFor(q.Error)
	}
	if q.RowsAffected == 0 {
		return repository.ErrInvalidInput
	}
	return nil
}
func (r *Repo) TouchSessionActivity(ctx context.Context, id uint, sid string, in repository.UpdateSessionActivityInput) error {
	updates := map[string]any{}
	if in.LastSeenAt != nil {
		updates["last_seen_at"] = *in.LastSeenAt
	}
	if in.ClientIP != nil {
		updates["client_ip"] = *in.ClientIP
	}
	if in.UserAgent != nil {
		updates["user_agent"] = *in.UserAgent
	}
	if in.DeviceName != nil {
		updates["device_name"] = *in.DeviceName
	}
	if in.BrowserName != nil {
		updates["browser_name"] = *in.BrowserName
	}
	if in.OSName != nil {
		updates["os_name"] = *in.OSName
	}
	if in.DeviceType != nil {
		updates["device_type"] = *in.DeviceType
	}
	if in.GeoSource != nil {
		updates["geo_source"] = *in.GeoSource
	}
	if in.GeoAccuracy != nil {
		updates["geo_accuracy"] = *in.GeoAccuracy
	}
	if in.CountryCode != nil {
		updates["country_code"] = *in.CountryCode
	}
	if in.RegionName != nil {
		updates["region_name"] = *in.RegionName
	}
	if in.CityName != nil {
		updates["city_name"] = *in.CityName
	}
	if in.TimezoneName != nil {
		updates["timezone_name"] = *in.TimezoneName
	}
	return errFor(r.db.WithContext(ctx).Model(&model.UserSession{}).Where("user_id = ? AND session_id = ?", id, sid).Updates(updates).Error)
}
func (r *Repo) RevokeSession(ctx context.Context, id uint, sid, reason string) error {
	q := r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("user_id = ? AND session_id = ? AND revoked_at IS NULL", id, sid).
		Updates(revokedSessionUpdates(reason))
	if q.Error != nil {
		return errFor(q.Error)
	}
	if q.RowsAffected == 0 {
		return repository.ErrInvalidInput
	}
	return nil
}
func (r *Repo) RevokeAllSessions(ctx context.Context, id uint, reason string) error {
	return errFor(r.db.WithContext(ctx).Model(&model.UserSession{}).Where("user_id = ? AND revoked_at IS NULL", id).Updates(revokedSessionUpdates(reason)).Error)
}

func revokedSessionUpdates(reason string) map[string]any {
	return map[string]any{
		"revoked_at":                   time.Now(),
		"revoke_reason":                reason,
		"sub2_access_token_encrypted":  "",
		"sub2_refresh_token_encrypted": "",
		"sub2_access_expires_at":       nil,
		"sub2_verified_at":             nil,
	}
}
func (r *Repo) ListActiveSessionsByUserID(ctx context.Context, id uint, now time.Time) ([]domainuser.Session, error) {
	var rows []model.UserSession
	if err := r.db.WithContext(ctx).Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", id, now).Find(&rows).Error; err != nil {
		return nil, errFor(err)
	}
	out := make([]domainuser.Session, 0, len(rows))
	for _, v := range rows {
		out = append(out, *toDomainSession(v))
	}
	return out, nil
}
func (r *Repo) ListAuthEvents(ctx context.Context, id uint, eventType, result string, off, limit int) ([]domainuser.AuthEvent, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.UserAuthEvent{})
	if id > 0 {
		q = q.Where("user_id = ?", id)
	}
	if eventType != "" {
		q = q.Where("event_type = ?", eventType)
	}
	if result != "" {
		q = q.Where("result = ?", result)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, errFor(err)
	}
	var rows []model.UserAuthEvent
	if err := q.Order("occurred_at DESC").Offset(off).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, errFor(err)
	}
	out := make([]domainuser.AuthEvent, 0, len(rows))
	for _, v := range rows {
		out = append(out, domainuser.AuthEvent{ID: v.ID, UserID: v.UserID, RequestID: v.RequestID, EventType: v.EventType, Result: v.Result, Reason: v.Reason, ClientIP: v.ClientIP, UserAgent: v.UserAgent, DetailJSON: v.DetailJSON, OccurredAt: v.OccurredAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt})
	}
	return out, total, nil
}
func toModelSession(v *domainuser.Session) *model.UserSession {
	return &model.UserSession{
		SessionID:                 v.SessionID,
		UserID:                    v.UserID,
		RefreshTokenHash:          v.RefreshTokenHash,
		AccessJTI:                 v.AccessJTI,
		Sub2AccessTokenEncrypted:  v.Sub2AccessTokenEncrypted,
		Sub2RefreshTokenEncrypted: v.Sub2RefreshTokenEncrypted,
		Sub2AccessExpiresAt:       v.Sub2AccessExpiresAt,
		Sub2VerifiedAt:            v.Sub2VerifiedAt,
		ClientIP:                  v.ClientIP,
		UserAgent:                 v.UserAgent,
		DeviceName:                v.DeviceName,
		BrowserName:               v.BrowserName,
		OSName:                    v.OSName,
		DeviceType:                v.DeviceType,
		GeoSource:                 v.GeoSource,
		GeoAccuracy:               v.GeoAccuracy,
		CountryCode:               v.CountryCode,
		RegionName:                v.RegionName,
		CityName:                  v.CityName,
		TimezoneName:              v.TimezoneName,
		IPLatitude:                v.IPLatitude,
		IPLongitude:               v.IPLongitude,
		IssuedAt:                  v.IssuedAt,
		LastSeenAt:                v.LastSeenAt,
		ExpiresAt:                 v.ExpiresAt,
	}
}
func toDomainSession(v model.UserSession) *domainuser.Session {
	return &domainuser.Session{
		ID:                        v.ID,
		SessionID:                 v.SessionID,
		UserID:                    v.UserID,
		RefreshTokenHash:          v.RefreshTokenHash,
		AccessJTI:                 v.AccessJTI,
		Sub2AccessTokenEncrypted:  v.Sub2AccessTokenEncrypted,
		Sub2RefreshTokenEncrypted: v.Sub2RefreshTokenEncrypted,
		Sub2AccessExpiresAt:       v.Sub2AccessExpiresAt,
		Sub2VerifiedAt:            v.Sub2VerifiedAt,
		ClientIP:                  v.ClientIP,
		UserAgent:                 v.UserAgent,
		DeviceName:                v.DeviceName,
		BrowserName:               v.BrowserName,
		OSName:                    v.OSName,
		DeviceType:                v.DeviceType,
		GeoSource:                 v.GeoSource,
		GeoAccuracy:               v.GeoAccuracy,
		CountryCode:               v.CountryCode,
		RegionName:                v.RegionName,
		CityName:                  v.CityName,
		TimezoneName:              v.TimezoneName,
		IPLatitude:                v.IPLatitude,
		IPLongitude:               v.IPLongitude,
		PreciseLatitude:           v.PreciseLatitude,
		PreciseLongitude:          v.PreciseLongitude,
		PreciseAccuracyM:          v.PreciseAccuracyM,
		PreciseLocatedAt:          v.PreciseLocatedAt,
		IssuedAt:                  v.IssuedAt,
		LastSeenAt:                v.LastSeenAt,
		ExpiresAt:                 v.ExpiresAt,
		RevokedAt:                 v.RevokedAt,
		RevokeReason:              v.RevokeReason,
		CreatedAt:                 v.CreatedAt,
		UpdatedAt:                 v.UpdatedAt,
	}
}
