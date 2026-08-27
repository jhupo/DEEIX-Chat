package agentclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnixInstallerRestoresBinaryAndConfigAfterConnectionFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Unix installer test requires Linux")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	script := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", "frontend", "public", "agent", "install.sh"))
	root := t.TempDir()
	home := filepath.Join(root, "home")
	dataDir := filepath.Join(root, "data")
	installDir := filepath.Join(root, "install")
	binDir := filepath.Join(root, "bin")
	mockBin := filepath.Join(root, "mock-bin")
	for _, directory := range []string{home, dataDir, installDir, binDir, mockBin} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	oldBinary := filepath.Join(installDir, "deeix-agent")
	configPath := filepath.Join(dataDir, "config.json")
	write(oldBinary, "old agent\n")
	write(configPath, "old config\n")
	asset := filepath.Join(root, "asset")
	checksum := filepath.Join(root, "asset.sha256")
	write(asset, "#!/bin/sh\nprintf 'new config\\n' > \"$DEEIX_AGENT_DATA_DIR/config.json\"\n")
	write(checksum, "fixture\n")
	write(filepath.Join(mockBin, "uname"), "#!/bin/sh\n[ \"$1\" = -m ] && echo x86_64 || echo Linux\n")
	write(filepath.Join(mockBin, "curl"), `#!/bin/sh
url=$1
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then out=$2; shift 2; else shift; fi
done
case "$url" in *.sha256) cp "$DEEIX_TEST_CHECKSUM" "$out" ;; *) cp "$DEEIX_TEST_ASSET" "$out" ;; esac
`)
	write(filepath.Join(mockBin, "sha256sum"), "#!/bin/sh\nexit 0\n")
	write(filepath.Join(mockBin, "shasum"), "#!/bin/sh\nexit 91\n")
	write(filepath.Join(mockBin, "systemctl"), "#!/bin/sh\nexit 0\n")
	write(filepath.Join(mockBin, "sleep"), "#!/bin/sh\nexit 0\n")

	command := exec.Command("/bin/sh", script, "--server", "https://example.com", "--user", "0123456789abcdef0123456789abcdef")
	command.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+mockBin+":/usr/bin:/bin",
		"DEEIX_AGENT_DATA_DIR="+dataDir,
		"DEEIX_AGENT_HOME="+installDir,
		"DEEIX_AGENT_BIN="+binDir,
		"DEEIX_TEST_ASSET="+asset,
		"DEEIX_TEST_CHECKSUM="+checksum,
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "service did not connect") {
		t.Fatalf("installer error = %v, output = %q", err, output)
	}
	for path, want := range map[string]string{oldBinary: "old agent\n", configPath: "old config\n"} {
		content, readErr := os.ReadFile(path)
		if readErr != nil || string(content) != want {
			t.Fatalf("restored %s = %q, %v", path, content, readErr)
		}
	}
}
