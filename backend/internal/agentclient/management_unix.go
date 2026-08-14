//go:build !windows

package agentclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func platformUpdate(ctx context.Context, dataDir string, config Config) (UpdateResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.CloudURL+"/agent/install.sh", nil)
	if err != nil {
		return UpdateResult{}, err
	}
	client := newAgentHTTPClient()
	response, err := client.Do(request)
	if err != nil {
		return UpdateResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return UpdateResult{}, fmt.Errorf("download update installer: %s", response.Status)
	}
	script, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil || len(script) == 0 {
		return UpdateResult{}, errors.New("downloaded update installer is invalid")
	}
	executable, err := os.Executable()
	if err != nil {
		return UpdateResult{}, err
	}
	workspace := config.Workspaces[0].Root
	command := exec.CommandContext(ctx, "sh", "-s", "--", "--server", config.CloudURL, "--user", config.UserPublicID, "--workspace", workspace, "--codex", config.CodexExecutable)
	command.Stdin = bytes.NewReader(script)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.Env = append(os.Environ(), "DEEIX_AGENT_DATA_DIR="+dataDir, "DEEIX_AGENT_HOME="+filepath.Dir(executable))
	if err = command.Run(); err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{Scheduled: false, Message: "agent was updated"}, nil
}

func platformUninstall(dataDir string, purge bool) (UninstallResult, error) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/com.deeix.agent", os.Getuid())).Run()
		home, _ := os.UserHomeDir()
		_ = os.Remove(filepath.Join(home, "Library", "LaunchAgents", "com.deeix.agent.plist"))
	} else {
		_ = exec.Command("systemctl", "--user", "disable", "--now", "deeix-agent.service").Run()
		home, _ := os.UserHomeDir()
		_ = os.Remove(filepath.Join(home, ".config", "systemd", "user", "deeix-agent.service"))
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	executable, err := os.Executable()
	if err != nil {
		return UninstallResult{}, err
	}
	if err = os.Remove(executable); err != nil && !errors.Is(err, os.ErrNotExist) {
		return UninstallResult{}, err
	}
	home, _ := os.UserHomeDir()
	_ = os.Remove(filepath.Join(home, ".local", "bin", "deeix-agent"))
	if purge {
		if err = os.RemoveAll(dataDir); err != nil {
			return UninstallResult{}, err
		}
	}
	return UninstallResult{Scheduled: false, Purged: purge}, nil
}
