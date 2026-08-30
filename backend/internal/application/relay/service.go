package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
	"github.com/google/uuid"
)

var (
	ErrInvalidConnector = errors.New("invalid relay connector")
	ErrInvalidRoute     = errors.New("invalid relay ingress route")
	ErrResourceNotFound = errors.New("relay resource not found")
	ErrResourceConflict = errors.New("relay resource already exists")
)

type Service struct {
	repo         repository.RelayRepository
	requireHTTPS bool
}

type ConnectorInput struct {
	Name           string
	Protocol       string
	AccountBaseURL string
	ModelBaseURL   string
	ConfigJSON     string
	Enabled        bool
}

type RouteInput struct {
	Hostname    string
	ConnectorID string
	Enabled     bool
}

func NewService(repo repository.RelayRepository, requireHTTPS bool) *Service {
	return &Service{repo: repo, requireHTTPS: requireHTTPS}
}

func (s *Service) ListConnectors(ctx context.Context) ([]domainrelay.Connector, error) {
	return s.repo.ListConnectors(ctx)
}
func (s *Service) ListIngressRoutes(ctx context.Context) ([]domainrelay.IngressRoute, error) {
	return s.repo.ListIngressRoutes(ctx)
}

func (s *Service) CreateConnector(ctx context.Context, input ConnectorInput) (*domainrelay.Connector, error) {
	normalized, err := normalizeConnector(input, s.requireHTTPS)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.CreateConnector(ctx, repository.RelayConnectorInput{
		PublicID: uuid.NewString(), Name: normalized.Name, Protocol: normalized.Protocol,
		AccountBaseURL: normalized.AccountBaseURL, ModelBaseURL: normalized.ModelBaseURL,
		ConfigJSON: normalized.ConfigJSON, Enabled: normalized.Enabled,
	})
	return item, normalizeRepoError(err)
}

func (s *Service) UpdateConnector(ctx context.Context, publicID string, input ConnectorInput) (*domainrelay.Connector, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil, ErrInvalidConnector
	}
	configProvided := strings.TrimSpace(input.ConfigJSON) != ""
	normalized, err := normalizeConnector(input, s.requireHTTPS)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetConnector(ctx, publicID)
	if err != nil {
		return nil, normalizeRepoError(err)
	}
	if existing.Protocol != normalized.Protocol || existing.AccountBaseURL != normalized.AccountBaseURL {
		return nil, ErrResourceConflict
	}
	if !configProvided {
		normalized.ConfigJSON = ""
	}
	item, err := s.repo.UpdateConnector(ctx, publicID, repository.RelayConnectorInput{
		Name: normalized.Name, Protocol: normalized.Protocol, AccountBaseURL: normalized.AccountBaseURL,
		ModelBaseURL: normalized.ModelBaseURL,
		ConfigJSON:   normalized.ConfigJSON, Enabled: normalized.Enabled,
	})
	return item, normalizeRepoError(err)
}

func (s *Service) DeleteConnector(ctx context.Context, publicID string) error {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return ErrInvalidConnector
	}
	return normalizeRepoError(s.repo.DeleteConnector(ctx, publicID))
}

func (s *Service) CreateIngressRoute(ctx context.Context, input RouteInput) (*domainrelay.IngressRoute, error) {
	host, err := normalizeHostname(input.Hostname)
	if err != nil {
		return nil, err
	}
	if _, err = s.repo.GetConnector(ctx, input.ConnectorID); err != nil {
		return nil, normalizeRepoError(err)
	}
	item, err := s.repo.CreateIngressRoute(ctx, host, strings.TrimSpace(input.ConnectorID), input.Enabled)
	return item, normalizeRepoError(err)
}

func (s *Service) UpdateIngressRoute(ctx context.Context, id uint, input RouteInput) (*domainrelay.IngressRoute, error) {
	if id == 0 {
		return nil, ErrInvalidRoute
	}
	host, err := normalizeHostname(input.Hostname)
	if err != nil {
		return nil, err
	}
	if _, err = s.repo.GetConnector(ctx, input.ConnectorID); err != nil {
		return nil, normalizeRepoError(err)
	}
	item, err := s.repo.UpdateIngressRoute(ctx, id, host, strings.TrimSpace(input.ConnectorID), input.Enabled)
	return item, normalizeRepoError(err)
}

func (s *Service) DeleteIngressRoute(ctx context.Context, id uint) error {
	if id == 0 {
		return ErrInvalidRoute
	}
	return normalizeRepoError(s.repo.DeleteIngressRoute(ctx, id))
}

func normalizeConnector(input ConnectorInput, requireHTTPS bool) (ConnectorInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	var err error
	input.AccountBaseURL, err = normalizeOrigin(input.AccountBaseURL, requireHTTPS)
	if err != nil {
		return ConnectorInput{}, err
	}
	modelBaseURL, err := normalizeOrigin(input.ModelBaseURL, requireHTTPS)
	if err != nil {
		return ConnectorInput{}, err
	}
	input.ModelBaseURL = modelBaseURL
	if input.ModelBaseURL == "" {
		input.ModelBaseURL = input.AccountBaseURL
	}
	input.ConfigJSON = strings.TrimSpace(input.ConfigJSON)
	if input.ConfigJSON == "" {
		input.ConfigJSON = "{}"
	}
	var config map[string]any
	if json.Unmarshal([]byte(input.ConfigJSON), &config) != nil {
		return ConnectorInput{}, ErrInvalidConnector
	}
	if input.Name == "" || len(input.Name) > 128 || input.AccountBaseURL == "" || input.ModelBaseURL == "" {
		return ConnectorInput{}, ErrInvalidConnector
	}
	if input.Protocol != "sub2api" {
		return ConnectorInput{}, ErrInvalidConnector
	}
	return input, nil
}

func normalizeOrigin(raw string, requireHTTPS bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", ErrInvalidConnector
	}
	origin, err := sharedsecurity.HTTPOrigin(raw)
	if err != nil {
		return "", ErrInvalidConnector
	}
	if err = sharedsecurity.ValidateTrustedOutboundHTTPURL(origin); err != nil {
		return "", ErrInvalidConnector
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return "", ErrInvalidConnector
	}
	return strings.TrimRight(origin, "/"), nil
}

func normalizeHostname(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" || len(host) > 255 || strings.ContainsAny(host, "/?#@") {
		return "", ErrInvalidRoute
	}
	if parsed, err := url.Parse("//" + host); err != nil || parsed.Host != host || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidRoute
	}
	if strings.Contains(host, ":") {
		if _, _, err := net.SplitHostPort(host); err != nil {
			return "", ErrInvalidRoute
		}
	}
	return host, nil
}

func normalizeRepoError(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrResourceNotFound
	}
	if errors.Is(err, repository.ErrDuplicate) {
		return ErrResourceConflict
	}
	if errors.Is(err, repository.ErrConflict) {
		return ErrResourceConflict
	}
	return err
}
