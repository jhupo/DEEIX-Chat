package model

import "time"

// RelayConnector stores one external identity/model gateway configuration.
type RelayConnector struct {
	ControlPlaneModel
	PublicID       string     `gorm:"size:64;not null;uniqueIndex:idx_relay_connectors_public_id;comment:连接器公开 ID"`
	Name           string     `gorm:"size:128;not null;uniqueIndex:idx_relay_connectors_name;comment:连接器名称"`
	Protocol       string     `gorm:"size:32;not null;index:idx_relay_connectors_protocol;comment:协议适配器"`
	AccountBaseURL string     `gorm:"size:512;not null;comment:账户 API origin"`
	ModelBaseURL   string     `gorm:"size:512;not null;comment:模型 API origin"`
	ConfigJSON     string     `gorm:"type:text;not null;default:'{}';comment:适配器配置 JSON"`
	Enabled        bool       `gorm:"not null;default:true;index:idx_relay_connectors_enabled;comment:是否启用"`
	LastProbeAt    *time.Time `gorm:"index:idx_relay_connectors_last_probe_at;comment:最近探测时间"`
	LastProbeError string     `gorm:"type:text;not null;default:'';comment:最近探测错误"`
}

func (RelayConnector) TableName() string { return "relay_connectors" }

// RelayIngressRoute maps an inbound hostname to exactly one connector.
type RelayIngressRoute struct {
	ControlPlaneModel
	Hostname    string `gorm:"size:255;not null;uniqueIndex:idx_relay_ingress_routes_hostname;comment:入站域名"`
	ConnectorID string `gorm:"size:64;not null;index:idx_relay_ingress_routes_connector;comment:连接器公开 ID"`
	Enabled     bool   `gorm:"not null;default:true;index:idx_relay_ingress_routes_enabled;comment:是否启用"`
}

func (RelayIngressRoute) TableName() string { return "relay_ingress_routes" }
