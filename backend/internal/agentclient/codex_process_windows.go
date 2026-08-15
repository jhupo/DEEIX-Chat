//go:build windows

package agentclient

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const WindowsUserSIDEnvironment = "DEEIX_AGENT_WINDOWS_USER_SID"

func configureCodexCommand(command *exec.Cmd) (func(), error) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	sessionToken, configured, err := configuredWindowsSessionToken()
	if err != nil {
		return nil, err
	}
	if !configured {
		return func() {}, nil
	}
	defer sessionToken.Close()
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

func configuredWindowsSessionToken() (windows.Token, bool, error) {
	expectedSID := strings.TrimSpace(os.Getenv(WindowsUserSIDEnvironment))
	if expectedSID == "" {
		return 0, false, nil
	}
	var sessionInfo *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessionInfo, &count); err != nil {
		return 0, false, fmt.Errorf("enumerate Windows user sessions: %w", err)
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessionInfo)))
	sessions := append([]windows.WTS_SESSION_INFO(nil), unsafe.Slice(sessionInfo, count)...)
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].SessionID < sessions[j].SessionID })
	for _, state := range []uint32{windows.WTSActive, windows.WTSConnected} {
		for _, session := range sessions {
			if session.State != state {
				continue
			}
			var token windows.Token
			if err := windows.WTSQueryUserToken(session.SessionID, &token); err != nil {
				continue
			}
			user, err := token.GetTokenUser()
			if err == nil && strings.EqualFold(user.User.Sid.String(), expectedSID) {
				return token, true, nil
			}
			token.Close()
		}
	}
	return 0, false, errors.New("the configured Windows user is not signed in")
}

func runAsConfiguredUser(operation func() error) error {
	sessionToken, configured, err := configuredWindowsSessionToken()
	if err != nil || !configured {
		if err != nil {
			return err
		}
		return operation()
	}
	defer sessionToken.Close()
	var impersonationToken windows.Token
	if err = windows.DuplicateTokenEx(sessionToken, windows.MAXIMUM_ALLOWED, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &impersonationToken); err != nil {
		return fmt.Errorf("create Windows user impersonation token: %w", err)
	}
	defer impersonationToken.Close()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = windows.SetThreadToken(nil, impersonationToken); err != nil {
		return fmt.Errorf("impersonate configured Windows user: %w", err)
	}
	defer windows.RevertToSelf()
	return operation()
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
