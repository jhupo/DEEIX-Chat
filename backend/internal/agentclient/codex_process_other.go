//go:build !windows

package agentclient

import "os/exec"

func configureCodexCommand(*exec.Cmd) (func(), error) { return func() {}, nil }
