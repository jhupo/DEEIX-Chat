package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrUpdateScheduled = errors.New("agent update scheduled")

const pendingUpdateFile = "pending-update.json"

type pendingUpdate struct {
	Version string `json:"version"`
}

type UpdateResult struct {
	Scheduled bool   `json:"scheduled"`
	Message   string `json:"message"`
}

type UninstallResult struct {
	Scheduled bool `json:"scheduled"`
	Purged    bool `json:"purged"`
}

func Update(ctx context.Context, dataDir string) (UpdateResult, error) {
	resolved, config, err := managementConfig(dataDir)
	if err != nil {
		return UpdateResult{}, err
	}
	result, err := platformUpdate(ctx, resolved, config)
	if err == nil {
		_ = os.Remove(filepath.Join(resolved, pendingUpdateFile))
	}
	return result, err
}

func preparePendingUpdate(dataDir, version string) error {
	version = strings.TrimSpace(version)
	if !validAgentVersion(version) {
		return errors.New("agent update version is invalid")
	}
	encoded, err := json.Marshal(pendingUpdate{Version: version})
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dataDir, pendingUpdateFile), encoded, 0o600)
}

func hasPendingUpdate(dataDir string) bool {
	content, err := os.ReadFile(filepath.Join(dataDir, pendingUpdateFile))
	if err != nil {
		return false
	}
	var update pendingUpdate
	return json.Unmarshal(content, &update) == nil && validAgentVersion(update.Version)
}

func clearPendingUpdate(dataDir string) { _ = os.Remove(filepath.Join(dataDir, pendingUpdateFile)) }

func Uninstall(dataDir string, purge bool) (UninstallResult, error) {
	resolved, _, err := managementConfig(dataDir)
	if err != nil {
		return UninstallResult{}, err
	}
	if purge {
		if err = validatePurgeDirectory(resolved); err != nil {
			return UninstallResult{}, err
		}
	}
	return platformUninstall(resolved, purge)
}

func managementConfig(dataDir string) (string, Config, error) {
	var err error
	if dataDir == "" {
		dataDir, err = DefaultDataDir()
		if err != nil {
			return "", Config{}, err
		}
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return "", Config{}, err
	}
	config, err := LoadConfig(filepath.Join(dataDir, "config.json"))
	return dataDir, config, err
}

func validatePurgeDirectory(dataDir string) error {
	clean := filepath.Clean(dataDir)
	volume := filepath.VolumeName(clean)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	home, _ := os.UserHomeDir()
	if clean == root || home != "" && clean == filepath.Clean(home) {
		return errors.New("agent data directory is not safe to purge")
	}
	if _, err := LoadConfig(filepath.Join(clean, "config.json")); err != nil {
		return errors.New("agent data directory does not contain a valid config")
	}
	return nil
}
