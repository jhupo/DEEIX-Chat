package relay

import "time"

type Connector struct {
	ID             uint
	PublicID       string
	Name           string
	Protocol       string
	AccountBaseURL string
	ModelBaseURL   string
	Enabled        bool
	LastProbeAt    *time.Time
	LastProbeError string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type IngressRoute struct {
	ID          uint
	Hostname    string
	ConnectorID string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
