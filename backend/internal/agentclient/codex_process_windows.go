//go:build windows

package agentclient

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const WindowsUserSIDEnvironment = "DEEIX_AGENT_WINDOWS_USER_SID"

func configureCodexCommand(command *exec.Cmd) (func(), error) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	expectedSID := strings.TrimSpace(os.Getenv(WindowsUserSIDEnvironment))
	if expectedSID == "" {
		return func() {}, nil
	}
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == ^uint32(0) {
		return nil, errors.New("the configured Windows user is not signed in")
	}
	var sessionToken windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &sessionToken); err != nil {
		return nil, fmt.Errorf("open active Windows user session: %w", err)
	}
	defer sessionToken.Close()
	user, err := sessionToken.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read active Windows user: %w", err)
	}
	if !strings.EqualFold(user.User.Sid.String(), expectedSID) {
		return nil, errors.New("the configured Windows user is not signed in")
	}
	var primaryToken windows.Token
	if err = windows.DuplicateTokenEx(sessionToken, windows.MAXIMUM_ALLOWED, nil, windows.SecurityImpersonation, windows.TokenPrimary, &primaryToken); err != nil {
		return nil, fmt.Errorf("create Windows user process token: %w", err)
	}
	environment, err := tokenEnvironment(primaryToken)
	if err != nil {
		primaryToken.Close()
		return nil, err
	}
	command.Env = environment
	command.SysProcAttr.Token = syscall.Token(primaryToken)
	return func() { _ = primaryToken.Close() }, nil
}

func tokenEnvironment(token windows.Token) ([]string, error) {
	var block *uint16
	if err := windows.CreateEnvironmentBlock(&block, token, false); err != nil {
		return nil, fmt.Errorf("read Windows user environment: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(block)
	const maxEnvironmentUTF16 = 1 << 19
	values := unsafe.Slice(block, maxEnvironmentUTF16)
	environment := make([]string, 0, 64)
	for start := 0; start < len(values); {
		end := start
		for end < len(values) && values[end] != 0 {
			end++
		}
		if end == len(values) {
			return nil, errors.New("Windows user environment is invalid")
		}
		if end == start {
			return environment, nil
		}
		environment = append(environment, windows.UTF16ToString(values[start:end]))
		start = end + 1
	}
	return nil, errors.New("Windows user environment is too large")
}
