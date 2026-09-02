//go:build !windows

package agentclient

import "os"

func replaceFileAtomic(source, target string) error {
	return os.Rename(source, target)
}
