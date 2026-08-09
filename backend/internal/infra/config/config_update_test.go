package config

import "testing"

func TestValidateUpdateSocketPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{{"/run/deeix-updater/deeix-updater.sock", true}, {"relative.sock", false}, {"/run/socket\n", false}} {
		cfg := Load()
		cfg.Env = "dev"
		cfg.UpdateSocketPath = tc.path
		err := cfg.Validate()
		if tc.want && err != nil {
			t.Fatalf("%q: %v", tc.path, err)
		}
		if !tc.want && err == nil {
			t.Fatalf("%q accepted", tc.path)
		}
	}
}
