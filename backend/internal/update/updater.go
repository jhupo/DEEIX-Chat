package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	xproxy "golang.org/x/net/proxy"
)

const (
	maxManifestBytes = 64 << 10
	maxReleaseBytes  = 1 << 20
	maxArchiveBytes  = 512 << 20
	maxExtractBytes  = 1024 << 20
	maxArchiveFiles  = 100000
	maxActorText     = 64
	maxRequestID     = 128
)

var (
	versionPattern            = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	tagPattern                = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	digestPattern             = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	repositoryPattern         = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	idempotencyPattern        = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,128}$`)
	commitPattern             = regexp.MustCompile(`^[0-9a-f]{40}$`)
	platformPattern           = regexp.MustCompile(`^linux/(amd64|arm64)$`)
	imageReleaseTargetPattern = regexp.MustCompile(`^releases/image-([0-9]+\.){2}[0-9]+-[0-9a-f]{64}$`)
)

type Config struct {
	Repository      string
	RuntimeDir      string
	StateFile       string
	CurrentVersion  string
	ProxyURL        string
	DownloadTimeout time.Duration
	Restart         func()
}

type manifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Repository    string           `json:"repository"`
	Tag           string           `json:"tag"`
	Version       string           `json:"version"`
	Commit        string           `json:"commit"`
	PublishedAt   string           `json:"publishedAt"`
	ReleaseURL    string           `json:"releaseURL"`
	Bundles       []manifestBundle `json:"bundles"`
}

type manifestBundle struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	HTMLURL    string        `json:"html_url"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type journal struct {
	Candidate *Candidate  `json:"candidate,omitempty"`
	Jobs      []storedJob `json:"jobs"`
}

type storedJob struct {
	Job
	Key           string `json:"key"`
	Input         string `json:"input"`
	ActorUserID   uint   `json:"actorUserId"`
	ActorUsername string `json:"actorUsername"`
	RequestID     string `json:"requestId"`
}

type Updater struct {
	cfg     Config
	mu      sync.Mutex
	journal journal
	http    *http.Client
	start   func(func())
}

var (
	ErrInvalidRequest = errors.New("invalid update request")
	ErrConflict       = errors.New("update conflict")
	ErrUpstream       = errors.New("update upstream failure")
	ErrInternal       = errors.New("update internal failure")
)

func NewUpdater(cfg Config) (*Updater, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(cfg.RuntimeDir, "releases"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.StateFile), 0o700); err != nil {
		return nil, err
	}
	transport, err := updateTransport(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}
	u := &Updater{
		cfg:   cfg,
		http:  &http.Client{Transport: transport, CheckRedirect: allowGitHubRedirect},
		start: func(fn func()) { go fn() },
	}
	if err := u.load(); err != nil {
		return nil, err
	}
	return u, nil
}

func validateConfig(cfg Config) error {
	if !repositoryPattern.MatchString(cfg.Repository) || !strictVersion(cfg.CurrentVersion) || !filepath.IsAbs(cfg.RuntimeDir) || !filepath.IsAbs(cfg.StateFile) || cfg.DownloadTimeout < 30*time.Second || cfg.DownloadTimeout > 2*time.Hour || cfg.Restart == nil {
		return errors.New("invalid updater configuration")
	}
	return nil
}

func updateTransport(rawProxy string) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if rawProxy == "" {
		return transport, nil
	}
	proxyURL, err := url.Parse(rawProxy)
	if err != nil {
		return nil, errors.New("invalid update proxy")
	}
	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		dialer, err := xproxy.FromURL(proxyURL, &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
		if err != nil {
			return nil, errors.New("invalid update proxy")
		}
		contextDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, errors.New("update proxy lacks context support")
		}
		transport.DialContext = contextDialer.DialContext
	default:
		return nil, errors.New("invalid update proxy")
	}
	return transport, nil
}

func allowGitHubRedirect(req *http.Request, via []*http.Request) error {
	host := strings.ToLower(req.URL.Hostname())
	if req.URL.Scheme != "https" || !(host == "github.com" || strings.HasSuffix(host, ".github.com") || strings.HasSuffix(host, ".githubusercontent.com")) {
		return errors.New("unexpected release redirect")
	}
	if len(via) > 4 {
		return errors.New("too many release redirects")
	}
	return nil
}

func (u *Updater) load() error {
	if err := rejectSymlink(u.cfg.StateFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	b, err := os.ReadFile(u.cfg.StateFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || json.Unmarshal(b, &u.journal) != nil {
		return errors.New("invalid update journal")
	}
	changed := false
	for i := range u.journal.Jobs {
		if !terminal(u.journal.Jobs[i].Status) {
			u.journal.Jobs[i].Status = "outcome_unknown"
			u.journal.Jobs[i].Error = "application restarted during update"
			u.journal.Jobs[i].UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	if changed {
		return u.save()
	}
	return nil
}

func (u *Updater) Status(context.Context) (Status, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.statusLocked(), nil
}

func (u *Updater) statusLocked() Status {
	var job *Job
	for i := len(u.journal.Jobs) - 1; i >= 0; i-- {
		stored := u.journal.Jobs[i]
		if !terminal(stored.Status) || (u.journal.Candidate != nil && stored.Version == u.journal.Candidate.Version) {
			copy := stored.Job
			job = &copy
			break
		}
	}
	return Status{
		InstalledVersion: u.cfg.CurrentVersion,
		Candidate:        u.journal.Candidate,
		UpdateAvailable:  u.journal.Candidate != nil && u.canInstallVersion(u.journal.Candidate.Version),
		Job:              job,
	}
}

func (u *Updater) canInstallVersion(version string) bool {
	target, _ := os.Readlink(filepath.Join(u.cfg.RuntimeDir, "current"))
	return installableVersion(u.cfg.CurrentVersion, version, target)
}

func installableVersion(currentVersion, candidateVersion, currentTarget string) bool {
	switch compareVersions(candidateVersion, currentVersion) {
	case 1:
		return true
	case -1:
		return false
	}
	return imageReleaseTargetPattern.MatchString(currentTarget) && strings.HasPrefix(currentTarget, "releases/image-"+candidateVersion+"-")
}

func (u *Updater) Check(ctx context.Context) (Status, error) {
	u.mu.Lock()
	for _, stored := range u.journal.Jobs {
		if !terminal(stored.Status) {
			u.mu.Unlock()
			return Status{}, ErrConflict
		}
	}
	u.mu.Unlock()
	candidate, err := u.fetchManifest(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, stored := range u.journal.Jobs {
		if !terminal(stored.Status) {
			return Status{}, ErrConflict
		}
	}
	u.journal.Candidate = candidate
	if err := u.save(); err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrInternal, err)
	}
	return u.statusLocked(), nil
}

func (u *Updater) fetchManifest(ctx context.Context) (*Candidate, error) {
	releaseURL := "https://api.github.com/repos/" + u.cfg.Repository + "/releases/latest"
	req, err := githubRequest(ctx, releaseURL, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	resp, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("release metadata unavailable")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseBytes+1))
	if err != nil || len(b) > maxReleaseBytes {
		return nil, errors.New("invalid release metadata")
	}
	var release githubRelease
	if err := json.Unmarshal(b, &release); err != nil || release.Draft || release.Prerelease || release.TagName == "" || release.HTMLURL != "https://github.com/"+u.cfg.Repository+"/releases/tag/"+release.TagName {
		return nil, errors.New("invalid release metadata")
	}
	manifestAsset, ok := releaseAsset(u.cfg.Repository, release.Assets, "update-manifest.json", maxManifestBytes)
	if !ok {
		return nil, errors.New("release manifest unavailable")
	}
	b, err = u.fetchAsset(ctx, manifestAsset, maxManifestBytes)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := decodeStrictJSON(b, &m); err != nil {
		return nil, errors.New("invalid release manifest")
	}
	candidate, err := validateManifest(u.cfg.Repository, m, b)
	if err != nil || candidate.Tag != release.TagName || candidate.ReleaseURL != release.HTMLURL {
		return nil, errors.New("invalid release manifest")
	}
	bundleName := filepath.Base(candidate.BundleURL)
	bundleAsset, ok := releaseAsset(u.cfg.Repository, release.Assets, bundleName, candidate.BundleSize)
	if !ok || bundleAsset.Size != candidate.BundleSize {
		return nil, errors.New("release bundle unavailable")
	}
	candidate.BundleURL = bundleAsset.URL
	return candidate, nil
}

func githubRequest(ctx context.Context, rawURL, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "DEEIX-Chat-Updater")
	return req, nil
}

func decodeStrictJSON(raw []byte, out any) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}

func releaseAsset(repository string, assets []githubAsset, name string, maxSize int64) (githubAsset, bool) {
	var found githubAsset
	for _, asset := range assets {
		if asset.Name != name {
			continue
		}
		expectedURL := "https://api.github.com/repos/" + repository + "/releases/assets/" + strconv.FormatInt(asset.ID, 10)
		if found.ID != 0 || asset.ID <= 0 || asset.Size <= 0 || asset.Size > maxSize || asset.URL != expectedURL {
			return githubAsset{}, false
		}
		found = asset
	}
	return found, found.ID != 0
}

func (u *Updater) fetchAsset(ctx context.Context, asset githubAsset, maxSize int64) ([]byte, error) {
	req, err := githubRequest(ctx, asset.URL, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	resp, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxSize {
		return nil, errors.New("release asset unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil || int64(len(raw)) > maxSize || int64(len(raw)) != asset.Size {
		return nil, errors.New("invalid release asset")
	}
	return raw, nil
}

func validateManifest(repo string, m manifest, raw []byte) (*Candidate, error) {
	if m.SchemaVersion != 2 || m.Repository != repo || len(m.Repository) > 160 || !tagPattern.MatchString(m.Tag) || !strictVersion(m.Version) || m.Version != strings.TrimPrefix(m.Tag, "v") || !commitPattern.MatchString(m.Commit) || m.ReleaseURL != "https://github.com/"+repo+"/releases/tag/"+m.Tag || len(m.Bundles) == 0 || len(m.Bundles) > 4 {
		return nil, errors.New("invalid release manifest")
	}
	published, err := time.Parse(time.RFC3339, m.PublishedAt)
	if err != nil || published.IsZero() || !strings.HasSuffix(m.PublishedAt, "Z") || published.After(time.Now().UTC().Add(10*time.Minute)) {
		return nil, errors.New("invalid release manifest")
	}
	wanted := "linux/" + runtime.GOARCH
	seen := make(map[string]bool, len(m.Bundles))
	var selected *manifestBundle
	for i := range m.Bundles {
		bundle := &m.Bundles[i]
		expectedURL := "https://github.com/" + repo + "/releases/download/" + m.Tag + "/deeix-chat-linux-" + strings.TrimPrefix(bundle.Platform, "linux/") + ".tar.gz"
		if !platformPattern.MatchString(bundle.Platform) || seen[bundle.Platform] || bundle.URL != expectedURL || !digestPattern.MatchString(bundle.SHA256) || bundle.Size <= 0 || bundle.Size > maxArchiveBytes {
			return nil, errors.New("invalid release manifest")
		}
		seen[bundle.Platform] = true
		if bundle.Platform == wanted {
			selected = bundle
		}
	}
	if selected == nil {
		return nil, errors.New("platform unavailable")
	}
	sum := sha256.Sum256(raw)
	return &Candidate{
		Version: m.Version, Tag: m.Tag, ReleaseURL: m.ReleaseURL,
		ManifestDigest: "sha256:" + hex.EncodeToString(sum[:]),
		BundleURL:      selected.URL, BundleDigest: selected.SHA256, BundleSize: selected.Size,
		Commit: m.Commit, PublishedAt: published,
	}, nil
}

func (u *Updater) Install(ctx context.Context, in InstallRequest) (Job, error) {
	if !idempotencyPattern.MatchString(in.IdempotencyKey) || !strictVersion(in.Version) || !digestPattern.MatchString(in.ManifestDigest) || in.Confirmation != "install "+in.Version+" "+in.ManifestDigest || len(in.ActorUsername) > maxActorText || len(in.RequestID) > maxRequestID || strings.ContainsAny(in.ActorUsername+in.RequestID, "\r\n\t\x00") {
		return Job{}, ErrInvalidRequest
	}
	input := fmt.Sprintf("%s|%s|%d", in.Version, in.ManifestDigest, in.ActorUserID)
	u.mu.Lock()
	for _, old := range u.journal.Jobs {
		if old.Key == in.IdempotencyKey {
			u.mu.Unlock()
			if old.Input != input {
				return Job{}, ErrConflict
			}
			return old.Job, nil
		}
	}
	for _, old := range u.journal.Jobs {
		if !terminal(old.Status) {
			u.mu.Unlock()
			return Job{}, ErrConflict
		}
	}
	u.mu.Unlock()

	candidate, err := u.fetchManifest(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	u.mu.Lock()
	for _, old := range u.journal.Jobs {
		if old.Key == in.IdempotencyKey {
			u.mu.Unlock()
			if old.Input != input {
				return Job{}, ErrConflict
			}
			return old.Job, nil
		}
	}
	if u.journal.Candidate == nil || *candidate != *u.journal.Candidate || candidate.Version != in.Version || candidate.ManifestDigest != in.ManifestDigest || !u.canInstallVersion(candidate.Version) {
		u.mu.Unlock()
		return Job{}, ErrConflict
	}
	for _, old := range u.journal.Jobs {
		if !terminal(old.Status) {
			u.mu.Unlock()
			return Job{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	stored := storedJob{
		Job: Job{ID: fmt.Sprintf("upd-%d", now.UnixNano()), Version: in.Version, Status: "queued", CreatedAt: now, UpdatedAt: now},
		Key: in.IdempotencyKey, Input: input, ActorUserID: in.ActorUserID, ActorUsername: in.ActorUsername, RequestID: in.RequestID,
	}
	u.journal.Candidate = candidate
	u.journal.Jobs = append(u.journal.Jobs, stored)
	if len(u.journal.Jobs) > 32 {
		u.journal.Jobs = u.journal.Jobs[len(u.journal.Jobs)-32:]
	}
	if err := u.save(); err != nil {
		u.mu.Unlock()
		return Job{}, fmt.Errorf("%w: %v", ErrInternal, err)
	}
	u.mu.Unlock()
	u.start(func() { u.apply(stored.ID, candidate) })
	return stored.Job, nil
}

func (u *Updater) Job(_ context.Context, id string) (Job, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, stored := range u.journal.Jobs {
		if stored.ID == id {
			return stored.Job, nil
		}
	}
	return Job{}, os.ErrNotExist
}

func (u *Updater) Restart(context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.journal.Candidate == nil || len(u.journal.Jobs) == 0 {
		return ErrConflict
	}
	latest := u.journal.Jobs[len(u.journal.Jobs)-1]
	if latest.Status != "succeeded" || latest.Version != u.journal.Candidate.Version {
		return ErrConflict
	}
	wanted, err := filepath.EvalSymlinks(filepath.Join(u.cfg.RuntimeDir, "current"))
	if err != nil || filepath.Clean(wanted) != filepath.Join(u.cfg.RuntimeDir, "releases", latest.Version) {
		return ErrConflict
	}
	u.start(func() {
		time.Sleep(500 * time.Millisecond)
		u.cfg.Restart()
	})
	return nil
}

func (u *Updater) apply(id string, candidate *Candidate) {
	if err := u.transition(id, "pulling", ""); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), u.cfg.DownloadTimeout)
	defer cancel()
	archivePath, err := u.download(ctx, id, candidate)
	if err != nil {
		_ = u.transition(id, "failed", "release download or checksum verification failed")
		return
	}
	defer os.Remove(archivePath)
	if err := u.transition(id, "applying", ""); err != nil {
		return
	}
	if err := u.installArchive(archivePath, id, candidate.Version); err != nil {
		_ = u.transition(id, "failed", "release extraction or activation failed")
		return
	}
	_ = u.transition(id, "succeeded", "")
}

func (u *Updater) download(ctx context.Context, id string, candidate *Candidate) (string, error) {
	req, err := githubRequest(ctx, candidate.BundleURL, "application/octet-stream")
	if err != nil {
		return "", err
	}
	resp, err := u.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > candidate.BundleSize {
		return "", errors.New("release bundle unavailable")
	}
	path := filepath.Join(u.cfg.RuntimeDir, ".download-"+id)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, candidate.BundleSize+1))
	if err != nil || written != candidate.BundleSize || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != candidate.BundleDigest {
		return "", errors.New("invalid release bundle")
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func (u *Updater) installArchive(archivePath, id, version string) error {
	stage := filepath.Join(u.cfg.RuntimeDir, ".staging-"+id)
	if err := os.Mkdir(stage, 0o755); err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := extractBundle(archivePath, stage); err != nil {
		return err
	}
	if err := validateRelease(stage, version); err != nil {
		return err
	}
	target := filepath.Join(u.cfg.RuntimeDir, "releases", version)
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(stage, target); err != nil {
			return err
		}
		keep = true
	} else if err != nil || validateRelease(target, version) != nil {
		return errors.New("existing release is invalid")
	}
	return switchCurrent(u.cfg.RuntimeDir, version)
}

func extractBundle(archivePath, stage string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var total int64
	entries := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || header.Name == "" || strings.Contains(header.Name, "\\") {
			return errors.New("invalid release archive")
		}
		entries++
		if entries > maxArchiveFiles {
			return errors.New("too many release archive entries")
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("invalid release archive path")
		}
		destination := filepath.Join(stage, clean)
		rel, err := filepath.Rel(stage, destination)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("invalid release archive path")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || total+header.Size > maxExtractBytes {
				return errors.New("release archive too large")
			}
			total += header.Size
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(out, io.LimitReader(reader, header.Size+1))
			closeErr := out.Close()
			if copyErr != nil || closeErr != nil || written != header.Size {
				return errors.New("invalid release archive file")
			}
		default:
			return errors.New("unsupported release archive entry")
		}
	}
	return nil
}

func validateRelease(root, version string) error {
	versionFile := filepath.Join(root, "VERSION")
	binary := filepath.Join(root, "deeix-chat")
	index := filepath.Join(root, "frontend", "out", "index.html")
	for _, path := range []string{versionFile, binary, index} {
		if err := regularFile(path); err != nil {
			return errors.New("release is incomplete")
		}
	}
	b, err := os.ReadFile(versionFile)
	if err != nil || strings.TrimSpace(string(b)) != version {
		return errors.New("release version mismatch")
	}
	return os.Chmod(binary, 0o755)
}

func switchCurrent(runtimeDir, version string) error {
	current := filepath.Join(runtimeDir, "current")
	if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink == 0 {
		return errors.New("current release is not a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := filepath.Join(runtimeDir, ".current-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.Symlink(filepath.Join("releases", version), temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, current); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return syncParent(runtimeDir)
}

func (u *Updater) transition(id, status, message string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.journal.Jobs {
		if u.journal.Jobs[i].ID == id {
			u.journal.Jobs[i].Status = status
			u.journal.Jobs[i].Error = message
			u.journal.Jobs[i].UpdatedAt = time.Now().UTC()
			return u.save()
		}
	}
	return os.ErrNotExist
}

func (u *Updater) save() error {
	b, err := json.Marshal(u.journal)
	if err != nil {
		return err
	}
	return atomicWrite(u.cfg.StateFile, b, 0o600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := rejectSymlink(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := path + ".tmp"
	if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temporary)
		}
	}()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	ok = true
	return syncParent(filepath.Dir(path))
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlink path rejected")
	}
	return nil
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("non-regular file")
	}
	return nil
}

func syncParent(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func terminal(status string) bool {
	return status == "succeeded" || status == "failed" || status == "outcome_unknown"
}

func strictVersion(version string) bool {
	match := versionPattern.FindStringSubmatch(version)
	if match == nil || len(version) > 32 {
		return false
	}
	for _, part := range match[1:] {
		if len(part) > 1 && part[0] == '0' {
			return false
		}
		value, err := strconv.ParseUint(part, 10, 31)
		if err != nil || value > 1<<31-1 {
			return false
		}
	}
	return true
}

func compareVersions(a, b string) int {
	if !strictVersion(a) || !strictVersion(b) {
		return 0
	}
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := range left {
		leftPart, _ := strconv.ParseUint(left[i], 10, 31)
		rightPart, _ := strconv.ParseUint(right[i], 10, 31)
		if leftPart > rightPart {
			return 1
		}
		if leftPart < rightPart {
			return -1
		}
	}
	return 0
}
