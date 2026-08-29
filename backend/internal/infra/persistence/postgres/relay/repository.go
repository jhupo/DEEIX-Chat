package relay

import (
	"context"
	"strings"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) ListConnectors(ctx context.Context) ([]domainrelay.Connector, error) {
	var rows []model.RelayConnector
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, normalize(err)
	}
	items := make([]domainrelay.Connector, 0, len(rows))
	for _, row := range rows {
		items = append(items, toConnector(row))
	}
	return items, nil
}

func (r *Repo) GetConnector(ctx context.Context, publicID string) (*domainrelay.Connector, error) {
	var row model.RelayConnector
	if err := r.db.WithContext(ctx).Where("public_id = ?", strings.TrimSpace(publicID)).First(&row).Error; err != nil {
		return nil, normalize(err)
	}
	item := toConnector(row)
	return &item, nil
}

func (r *Repo) GetConnectorByHostname(ctx context.Context, hostname string) (*domainrelay.Connector, error) {
	var row model.RelayConnector
	err := r.db.WithContext(ctx).Table("relay_connectors AS c").
		Select("c.*").Joins("JOIN relay_ingress_routes AS r ON r.connector_id = c.public_id").
		Where("LOWER(r.hostname) = LOWER(?) AND r.enabled = TRUE AND c.enabled = TRUE", strings.TrimSpace(hostname)).First(&row).Error
	if err != nil {
		return nil, normalize(err)
	}
	item := toConnector(row)
	return &item, nil
}

func (r *Repo) CreateConnector(ctx context.Context, in repository.RelayConnectorInput) (*domainrelay.Connector, error) {
	row := model.RelayConnector{PublicID: in.PublicID, Name: in.Name, Protocol: in.Protocol, AccountBaseURL: in.AccountBaseURL, ModelBaseURL: in.ModelBaseURL, ConfigJSON: in.ConfigJSON, Enabled: in.Enabled}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, normalize(err)
	}
	item := toConnector(row)
	return &item, nil
}

func (r *Repo) UpdateConnector(ctx context.Context, publicID string, in repository.RelayConnectorInput) (*domainrelay.Connector, error) {
	updates := map[string]any{"name": in.Name, "protocol": in.Protocol, "account_base_url": in.AccountBaseURL, "model_base_url": in.ModelBaseURL, "config_json": in.ConfigJSON, "enabled": in.Enabled}
	if strings.TrimSpace(in.ConfigJSON) == "" {
		delete(updates, "config_json")
	}
	q := r.db.WithContext(ctx).Model(&model.RelayConnector{}).Where("public_id = ?", strings.TrimSpace(publicID)).Updates(updates)
	if q.Error != nil {
		return nil, normalize(q.Error)
	}
	if q.RowsAffected == 0 {
		return nil, repository.ErrNotFound
	}
	return r.GetConnector(ctx, publicID)
}

func (r *Repo) DeleteConnector(ctx context.Context, publicID string) error {
	if err := r.db.WithContext(ctx).Where("public_id = ?", strings.TrimSpace(publicID)).Delete(&model.RelayConnector{}).Error; err != nil {
		return normalize(err)
	}
	return nil
}

func (r *Repo) ListIngressRoutes(ctx context.Context) ([]domainrelay.IngressRoute, error) {
	var rows []model.RelayIngressRoute
	if err := r.db.WithContext(ctx).Order("hostname ASC").Find(&rows).Error; err != nil {
		return nil, normalize(err)
	}
	items := make([]domainrelay.IngressRoute, 0, len(rows))
	for _, row := range rows {
		items = append(items, toRoute(row))
	}
	return items, nil
}

func (r *Repo) CreateIngressRoute(ctx context.Context, hostname, connectorID string, enabled bool) (*domainrelay.IngressRoute, error) {
	row := model.RelayIngressRoute{Hostname: hostname, ConnectorID: connectorID, Enabled: enabled}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, normalize(err)
	}
	item := toRoute(row)
	return &item, nil
}

func (r *Repo) UpdateIngressRoute(ctx context.Context, id uint, hostname, connectorID string, enabled bool) (*domainrelay.IngressRoute, error) {
	q := r.db.WithContext(ctx).Model(&model.RelayIngressRoute{}).Where("id = ?", id).Updates(map[string]any{"hostname": hostname, "connector_id": connectorID, "enabled": enabled})
	if q.Error != nil {
		return nil, normalize(q.Error)
	}
	if q.RowsAffected == 0 {
		return nil, repository.ErrNotFound
	}
	var row model.RelayIngressRoute
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, normalize(err)
	}
	item := toRoute(row)
	return &item, nil
}

func (r *Repo) DeleteIngressRoute(ctx context.Context, id uint) error {
	q := r.db.WithContext(ctx).Delete(&model.RelayIngressRoute{}, id)
	if q.Error != nil {
		return normalize(q.Error)
	}
	if q.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func toConnector(row model.RelayConnector) domainrelay.Connector {
	return domainrelay.Connector{ID: row.ID, PublicID: row.PublicID, Name: row.Name, Protocol: row.Protocol, AccountBaseURL: row.AccountBaseURL, ModelBaseURL: row.ModelBaseURL, Enabled: row.Enabled, LastProbeAt: row.LastProbeAt, LastProbeError: row.LastProbeError, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func toRoute(row model.RelayIngressRoute) domainrelay.IngressRoute {
	return domainrelay.IngressRoute{ID: row.ID, Hostname: row.Hostname, ConnectorID: row.ConnectorID, Enabled: row.Enabled, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func normalize(err error) error {
	if err == nil {
		return nil
	}
	if err == gorm.ErrRecordNotFound {
		return repository.ErrNotFound
	}
	if dberror.IsUniqueConstraint(err) {
		return repository.ErrDuplicate
	}
	return err
}
