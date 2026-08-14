package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type InstallOptions struct {
	Server          string
	UserPublicID    string
	Workspace       string
	Name            string
	CodexExecutable string
	DataDir         string
}

type InstallResult struct {
	DeviceID     string `json:"deviceId"`
	CodexVersion string `json:"codexVersion"`
	WorkspaceID  string `json:"workspaceId"`
	Workspace    string `json:"workspace"`
	Updated      bool   `json:"updated"`
}

type DoctorReport struct {
	Healthy         bool   `json:"healthy"`
	DeviceID        string `json:"deviceId"`
	Server          string `json:"server"`
	CodexExecutable string `json:"codexExecutable"`
	CodexVersion    string `json:"codexVersion"`
	WorkspaceCount  int    `json:"workspaceCount"`
	Connection      string `json:"connection"`
}

func Install(ctx context.Context, options InstallOptions, stderr io.Writer) (InstallResult, error) {
	server, err := NormalizeCloudURL(strings.TrimSpace(options.Server))
	if err != nil {
		return InstallResult{}, err
	}
	userPublicID := strings.ToLower(strings.TrimSpace(options.UserPublicID))
	if !validHex(userPublicID, 32) {
		return InstallResult{}, errors.New("user must be a DEEIX public user ID")
	}
	name := strings.TrimSpace(options.Name)
	if name == "" || len(name) > 128 {
		return InstallResult{}, errors.New("device name is invalid")
	}
	workspace, err := CanonicalWorkspace(options.Workspace)
	if err != nil {
		return InstallResult{}, err
	}
	dataDir := options.DataDir
	if dataDir == "" {
		dataDir, err = DefaultDataDir()
		if err != nil {
			return InstallResult{}, err
		}
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return InstallResult{}, err
	}
	if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return InstallResult{}, err
	}
	codex := strings.TrimSpace(options.CodexExecutable)
	if codex == "" {
		codex = "codex"
	}
	resolvedCodex, version, err := ResolveCodex(ctx, codex)
	if err != nil {
		return InstallResult{}, err
	}
	configPath := filepath.Join(dataDir, "config.json")
	config, loadErr := LoadConfig(configPath)
	updated := loadErr == nil
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return InstallResult{}, loadErr
	}
	identityPath := filepath.Join(dataDir, "device-identity.json")
	var identity *DeviceIdentity
	if updated {
		identity, err = LoadIdentity(identityPath)
	} else {
		identity, err = LoadOrCreateIdentity(identityPath)
	}
	if err != nil {
		return InstallResult{}, fmt.Errorf("load device identity: %w", err)
	}
	if updated {
		if config.CloudURL != server || config.UserPublicID != userPublicID {
			return InstallResult{}, errors.New("this agent identity belongs to a different server or user")
		}
		config.CodexExecutable = resolvedCodex
		upsertWorkspace(&config.Workspaces, workspace)
	} else {
		config = Config{
			Version: configVersion, CloudURL: server, UserPublicID: userPublicID, ProfileID: "codex-default",
			CodexExecutable: resolvedCodex, Workspaces: []Workspace{workspace},
		}
	}
	state, err := OpenStateStore(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return InstallResult{}, err
	}
	adapter, err := StartCodexAdapter(ctx, config, state, stderr, func(json.RawMessage) error { return nil })
	if err != nil {
		return InstallResult{}, err
	}
	defer adapter.Close()
	if !updated {
		cloud := NewCloudClient(server)
		enrollContext, cancel := context.WithTimeout(ctx, 60*time.Second)
		config.DeviceID, err = cloud.Enroll(enrollContext, userPublicID, name, identity, adapter.ProveRuntimeAuth)
		cancel()
		if err != nil {
			return InstallResult{}, err
		}
	}
	if err = SaveConfig(configPath, config); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{DeviceID: config.DeviceID, CodexVersion: version, WorkspaceID: workspace.WorkspaceID, Workspace: workspace.Root, Updated: updated}, nil
}

func Doctor(ctx context.Context, dataDir string, stderr io.Writer) (DoctorReport, error) {
	if dataDir == "" {
		var err error
		dataDir, err = DefaultDataDir()
		if err != nil {
			return DoctorReport{}, err
		}
	}
	config, err := LoadConfig(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return DoctorReport{}, err
	}
	identity, err := LoadIdentity(filepath.Join(dataDir, "device-identity.json"))
	if err != nil {
		return DoctorReport{}, err
	}
	state, err := OpenStateStore(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return DoctorReport{}, err
	}
	adapter, err := StartCodexAdapter(ctx, config, state, stderr, func(json.RawMessage) error { return nil })
	if err != nil {
		return DoctorReport{}, err
	}
	defer adapter.Close()
	discoveryContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	workspaces, err := adapter.DiscoverWorkspaces(discoveryContext)
	cancel()
	if err != nil {
		return DoctorReport{}, err
	}
	connectionContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	_, err = NewCloudClient(config.CloudURL).ConnectionToken(connectionContext, config, identity)
	cancel()
	if err != nil {
		return DoctorReport{}, err
	}
	return DoctorReport{
		Healthy: true, DeviceID: config.DeviceID, Server: config.CloudURL, CodexExecutable: config.CodexExecutable,
		CodexVersion: adapter.version, WorkspaceCount: len(workspaces), Connection: "authenticated",
	}, nil
}

func ReadRuntimeStatus(dataDir string) (RuntimeStatus, error) {
	if dataDir == "" {
		var err error
		dataDir, err = DefaultDataDir()
		if err != nil {
			return RuntimeStatus{}, err
		}
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "runtime-status.json"))
	if err != nil {
		return RuntimeStatus{}, err
	}
	var status RuntimeStatus
	if json.Unmarshal(data, &status) != nil || status.Version != 1 || status.PID <= 0 || status.State == "" {
		return RuntimeStatus{}, errors.New("agent runtime status is invalid")
	}
	return status, nil
}

func PlatformName() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func upsertWorkspace(items *[]Workspace, workspace Workspace) {
	for index := range *items {
		if (*items)[index].WorkspaceID == workspace.WorkspaceID {
			(*items)[index] = workspace
			return
		}
	}
	*items = append(*items, workspace)
}
