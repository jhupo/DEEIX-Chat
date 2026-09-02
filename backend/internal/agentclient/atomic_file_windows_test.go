//go:build windows

package agentclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWriteFileAtomicRetriesTransientWindowsSharingViolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(75 * time.Millisecond)
		_ = windows.CloseHandle(handle)
		close(released)
	}()
	if err = writeFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	<-released
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("atomic replacement = %q, %v", data, err)
	}
}
