package agentclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

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
	return platformUpdate(ctx, resolved, config)
}

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
