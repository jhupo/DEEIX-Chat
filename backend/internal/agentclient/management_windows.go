//go:build windows

package agentclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func platformUpdate(_ context.Context, dataDir string, config Config) (UpdateResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return UpdateResult{}, err
	}
	scriptPath := filepath.Join(dataDir, "update-agent.ps1")
	workspace := config.Workspaces[0].Root
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
try {
  Wait-Process -Id %d -ErrorAction SilentlyContinue
  $env:DEEIX_AGENT_DATA_DIR = '%s'
  $env:DEEIX_AGENT_HOME = '%s'
  & ([scriptblock]::Create((irm '%s/agent/install.ps1'))) -Server '%s' -User '%s' -Workspace '%s' -Codex '%s'
} finally {
  Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
`, os.Getpid(), psLiteral(dataDir), psLiteral(filepath.Dir(executable)), psLiteral(config.CloudURL), psLiteral(config.CloudURL), psLiteral(config.UserPublicID), psLiteral(workspace), psLiteral(config.CodexExecutable))
	if err = writeFileAtomic(scriptPath, []byte(script), 0o600); err != nil {
		return UpdateResult{}, err
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	if err = command.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return UpdateResult{}, err
	}
	return UpdateResult{Scheduled: true, Message: "update will run after this command exits"}, nil
}

func platformUninstall(dataDir string, purge bool) (UninstallResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return UninstallResult{}, err
	}
	scriptPath := filepath.Join(dataDir, "uninstall-agent.ps1")
	purgeCommand := ""
	purgeLiteral := "$false"
	if purge {
		purgeCommand = fmt.Sprintf("  Remove-Item -LiteralPath '%s' -Recurse -Force -ErrorAction SilentlyContinue\n", psLiteral(dataDir))
		purgeLiteral = "$true"
	}
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
Unregister-ScheduledTask -TaskName 'DEEIX Agent' -Confirm:$false -ErrorAction SilentlyContinue
Wait-Process -Id %d -ErrorAction SilentlyContinue
Remove-Item -LiteralPath '%s' -Force -ErrorAction SilentlyContinue
%sif (-not %s) { Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue }
`, os.Getpid(), psLiteral(executable), purgeCommand, purgeLiteral)
	if err = writeFileAtomic(scriptPath, []byte(script), 0o600); err != nil {
		return UninstallResult{}, err
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	if err = command.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return UninstallResult{}, err
	}
	return UninstallResult{Scheduled: true, Purged: purge}, nil
}

func psLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
