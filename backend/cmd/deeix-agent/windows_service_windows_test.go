//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func TestValidateServiceArguments(t *testing.T) {
	dataDir, sid, err := validateServiceArguments(t.TempDir(), "S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dataDir) || sid != "S-1-5-18" {
		t.Fatalf("unexpected service arguments: %q %q", dataDir, sid)
	}
	if _, _, err = validateServiceArguments(dataDir, "not-a-sid"); err == nil {
		t.Fatal("invalid SID was accepted")
	}
}
