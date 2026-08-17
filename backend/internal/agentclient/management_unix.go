//go:build !windows

package agentclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	installerPath := filepath.Join(dataDir, "update-installer.sh")
	wrapperPath := filepath.Join(dataDir, "update-agent.sh")
	logPath := filepath.Join(dataDir, "update.log")
	if err = writeFileAtomic(installerPath, script, 0o700); err != nil {
		return UpdateResult{}, err
	}
	wrapper := fmt.Sprintf(`#!/bin/sh
while kill -0 %d 2>/dev/null; do sleep 1; done
cleanup() { rm -f %s %s; }
trap cleanup EXIT INT TERM
DEEIX_AGENT_DATA_DIR=%s DEEIX_AGENT_HOME=%s sh %s --server %s --user %s --workspace %s --codex %s >>%s 2>&1
`, os.Getpid(), shellLiteral(wrapperPath), shellLiteral(installerPath), shellLiteral(dataDir), shellLiteral(filepath.Dir(executable)),
		shellLiteral(installerPath), shellLiteral(config.CloudURL), shellLiteral(config.UserPublicID), shellLiteral(workspace), shellLiteral(config.CodexExecutable), shellLiteral(logPath))
	if err = writeFileAtomic(wrapperPath, []byte(wrapper), 0o700); err != nil {
		_ = os.Remove(installerPath)
		return UpdateResult{}, err
	}
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("launchctl", "submit", "-l", fmt.Sprintf("com.deeix.agent.update.%d", os.Getpid()), "--", "/bin/sh", wrapperPath)
	} else {
		command = exec.Command("systemd-run", "--user", "--collect", "--unit", fmt.Sprintf("deeix-agent-update-%d", os.Getpid()), "/bin/sh", wrapperPath)
	}
	if err = command.Run(); err != nil {
		_ = os.Remove(wrapperPath)
		_ = os.Remove(installerPath)
		return UpdateResult{}, err
	}
	return UpdateResult{Scheduled: true, Message: "update will run after this command exits"}, nil
}

func shellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
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
