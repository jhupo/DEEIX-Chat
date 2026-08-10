package config

import "testing"

func TestValidateUpdateConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		proxy string
		want  bool
	}{
		{name: "direct", want: true},
		{name: "http proxy", proxy: "http://127.0.0.1:7890", want: true},
		{name: "socks proxy", proxy: "socks5h://127.0.0.1:1080", want: true},
		{name: "unsupported scheme", proxy: "ftp://127.0.0.1:21", want: false},
		{name: "proxy query", proxy: "http://127.0.0.1:7890?x=1", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Load()
			cfg.Env = "dev"
			cfg.UpdateProxyURL = tc.proxy
			err := cfg.Validate()
			if tc.want && err != nil {
				t.Fatal(err)
			}
			if !tc.want && err == nil {
				t.Fatal("accepted invalid update configuration")
			}
		})
	}
}
