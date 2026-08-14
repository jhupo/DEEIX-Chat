//go:build windows

package agentclient

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureCodexCommandHidesUserProcess(t *testing.T) {
	t.Setenv(WindowsUserSIDEnvironment, "")
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	cleanup, err := configureCodexCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow || command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("Codex command is not configured as a background process")
	}
}
