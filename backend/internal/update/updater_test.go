package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func bundleBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, contents := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testManifest(t *testing.T, version string, bundle []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(bundle)
	manifest := manifest{
		SchemaVersion: 2,
		Repository:    "owner/repo",
		Tag:           "v" + version,
		Version:       version,
		Commit:        "0123456789abcdef0123456789abcdef01234567",
		PublishedAt:   time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		ReleaseURL:    "https://github.com/owner/repo/releases/tag/v" + version,
		Bundles: []manifestBundle{{
			Platform: "linux/" + runtime.GOARCH,
			URL:      "https://github.com/owner/repo/releases/download/v" + version + "/deeix-chat-linux-" + runtime.GOARCH + ".tar.gz",
			SHA256:   "sha256:" + hex.EncodeToString(sum[:]),
			Size:     int64(len(bundle)),
		}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func response(code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), ContentLength: int64(len(body))}
}

func testRelease(t *testing.T, version string, manifest, bundle []byte) []byte {
	t.Helper()
	raw, err := json.Marshal(githubRelease{
		TagName: "v" + version,
		HTMLURL: "https://github.com/owner/repo/releases/tag/v" + version,
		Assets: []githubAsset{
			{ID: 1, Name: "update-manifest.json", URL: "https://api.github.com/repos/owner/repo/releases/assets/1", Size: int64(len(manifest))},
			{ID: 2, Name: "deeix-chat-linux-" + runtime.GOARCH + ".tar.gz", URL: "https://api.github.com/repos/owner/repo/releases/assets/2", Size: int64(len(bundle))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestExtractBundleRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bundle.tar.gz")
	if err := os.WriteFile(archive, bundleBytes(t, map[string]string{"../outside": "bad"}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractBundle(archive, filepath.Join(dir, "stage")); err == nil {
		t.Fatal("accepted traversal entry")
	}
}

func TestCheckInstallAndActivateRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release activation uses Linux symlinks")
	}
	bundle := bundleBytes(t, map[string]string{
		"VERSION":                 "0.3.9\n",
		"deeix-chat":              "binary",
		"frontend/out/index.html": "index",
	})
	manifest := testManifest(t, "0.3.9", bundle)
	release := testRelease(t, "0.3.9", manifest, bundle)
	runtimeDir := t.TempDir()
	stateFile := filepath.Join(t.TempDir(), "journal.json")
	u := &Updater{
		cfg: Config{Repository: "owner/repo", RuntimeDir: runtimeDir, StateFile: stateFile, CurrentVersion: "0.3.8", DownloadTimeout: time.Minute, Restart: func() {}},
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.Path {
			case "/repos/owner/repo/releases/latest":
				return response(http.StatusOK, release), nil
			case "/repos/owner/repo/releases/assets/1":
				return response(http.StatusOK, manifest), nil
			case "/repos/owner/repo/releases/assets/2":
				return response(http.StatusOK, bundle), nil
			default:
				return response(http.StatusNotFound, nil), nil
			}
		})},
		start: func(fn func()) { fn() },
	}
	if err := os.MkdirAll(filepath.Join(runtimeDir, "releases"), 0o755); err != nil {
		t.Fatal(err)
	}
	status, err := u.Check(context.Background())
	if err != nil || !status.UpdateAvailable {
		t.Fatalf("check status=%#v error=%v", status, err)
	}
	candidate := status.Candidate
	job, err := u.Install(context.Background(), InstallRequest{
		Version: candidate.Version, ManifestDigest: candidate.ManifestDigest,
		Confirmation:   "install " + candidate.Version + " " + candidate.ManifestDigest,
		IdempotencyKey: "1234567890abcdef", ActorUserID: 1, ActorUsername: "root", RequestID: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err = u.Job(context.Background(), job.ID)
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("job=%#v error=%v", job, err)
	}
	current, err := filepath.EvalSymlinks(filepath.Join(runtimeDir, "current"))
	if err != nil || current != filepath.Join(runtimeDir, "releases", "0.3.9") {
		t.Fatalf("current=%q error=%v", current, err)
	}
}

func TestReleaseAssetRejectsDuplicatesAndUnexpectedURLs(t *testing.T) {
	valid := githubAsset{ID: 1, Name: "update-manifest.json", URL: "https://api.github.com/repos/owner/repo/releases/assets/1", Size: 100}
	if _, ok := releaseAsset("owner/repo", []githubAsset{valid}, valid.Name, maxManifestBytes); !ok {
		t.Fatal("rejected valid GitHub release asset")
	}
	duplicate := valid
	duplicate.ID = 2
	duplicate.URL = "https://api.github.com/repos/owner/repo/releases/assets/2"
	if _, ok := releaseAsset("owner/repo", []githubAsset{valid, duplicate}, valid.Name, maxManifestBytes); ok {
		t.Fatal("accepted duplicate release asset")
	}
	invalidURL := valid
	invalidURL.URL = "https://example.com/asset"
	if _, ok := releaseAsset("owner/repo", []githubAsset{invalidURL}, valid.Name, maxManifestBytes); ok {
		t.Fatal("accepted unexpected release asset URL")
	}
}

func TestManifestRejectsUnexpectedAssetURL(t *testing.T) {
	bundle := []byte("bundle")
	raw := testManifest(t, "0.3.9", bundle)
	var decoded manifest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Bundles[0].URL = "https://example.com/bundle.tar.gz"
	if _, err := validateManifest("owner/repo", decoded, raw); err == nil {
		t.Fatal("accepted unexpected asset URL")
	}
}

func TestCheckRejectsCandidateRefreshDuringInstall(t *testing.T) {
	u := &Updater{journal: journal{Jobs: []storedJob{{Job: Job{ID: "active", Status: "pulling"}}}}}
	if _, err := u.Check(context.Background()); err != ErrConflict {
		t.Fatalf("Check() error = %v, want conflict", err)
	}
}

func TestCanInstallVersionAllowsSameVersionOnlyFromImageRelease(t *testing.T) {
	imageTarget := "releases/image-0.3.9-" + strings.Repeat("a", 64)
	if !installableVersion("0.3.9", "0.4.0", "") || installableVersion("0.3.9", "0.3.8", imageTarget) || installableVersion("0.3.9", "0.3.9", "") {
		t.Fatal("semantic version ordering is incorrect")
	}
	if !installableVersion("0.3.9", "0.3.9", imageTarget) {
		t.Fatal("same-version stable bundle was rejected for an image release")
	}
	if installableVersion("0.3.9", "0.3.9", "releases/0.3.9") || installableVersion("0.3.9", "0.3.9", "releases/image-0.3.9-not-a-digest") {
		t.Fatal("same-version stable bundle was accepted twice")
	}
}
