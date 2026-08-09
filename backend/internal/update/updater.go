package update

import (
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
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxManifestBytes = 64 << 10
	maxActorText     = 64
	maxRequestID     = 128
)

var (
	versionPattern     = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	tagPattern         = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	digestPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	repositoryPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,128}$`)
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	platformPattern    = regexp.MustCompile(`^linux/(amd64|arm64)$`)
	containerIDPattern = regexp.MustCompile(`^[0-9a-f]{12,64}$`)
)

type HostConfig struct {
	Repository, SocketPath, StateFile, DeploymentDir, ComposeFile, EnvFile, AppBaseURL string
	ReadyTimeout                                                                       time.Duration
}

type manifest struct {
	SchemaVersion   int      `json:"schemaVersion"`
	Repository      string   `json:"repository"`
	Tag             string   `json:"tag"`
	Version         string   `json:"version"`
	Commit          string   `json:"commit"`
	PublishedAt     string   `json:"publishedAt"`
	ImageRepository string   `json:"imageRepository"`
	ImageDigest     string   `json:"imageDigest"`
	ReleaseURL      string   `json:"releaseURL"`
	Platforms       []string `json:"platforms"`
}

type journal struct {
	InstalledVersion string      `json:"installedVersion"`
	InstalledDigest  string      `json:"installedDigest"`
	Candidate        *Candidate  `json:"candidate,omitempty"`
	Jobs             []storedJob `json:"jobs"`
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
	cfg     HostConfig
	mu      sync.Mutex
	journal journal
	http    *http.Client
	run     func(context.Context, string, []string, []string) error
	output  func(context.Context, string, []string, []string) (string, error)
	start   func(func())
	ready   func(context.Context, string) error
	locked  bool
}

func NewUpdater(cfg HostConfig) (*Updater, error) {
	if err := validateHostConfig(&cfg); err != nil {
		return nil, err
	}
	u := &Updater{cfg: cfg, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: allowGitHubRedirect}}
	u.run = func(ctx context.Context, name string, args []string, env []string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = cfg.DeploymentDir
		cmd.Env = commandEnv(env)
		return cmd.Run()
	}
	u.output = func(ctx context.Context, name string, args []string, env []string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = cfg.DeploymentDir
		cmd.Env = commandEnv(env)
		out, err := cmd.Output()
		return string(out), err
	}
	u.start = func(fn func()) { go fn() }
	u.ready = u.waitReady
	if err := u.load(); err != nil {
		return nil, err
	}
	return u, nil
}

func commandEnv(overrides []string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		key, _, found := strings.Cut(override, "=")
		if found && key != "" {
			keys[key] = struct{}{}
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if _, overridden := keys[key]; !overridden {
			env = append(env, value)
		}
	}
	return append(env, overrides...)
}

func validateHostConfig(cfg *HostConfig) error {
	if !repositoryPattern.MatchString(cfg.Repository) || len(cfg.Repository) > 160 || !filepath.IsAbs(cfg.SocketPath) || !filepath.IsAbs(cfg.StateFile) || !filepath.IsAbs(cfg.DeploymentDir) || !filepath.IsAbs(cfg.ComposeFile) || !filepath.IsAbs(cfg.EnvFile) {
		return errors.New("invalid updater configuration")
	}
	if cfg.ReadyTimeout <= 0 || cfg.ReadyTimeout > 30*time.Minute {
		return errors.New("invalid updater timeout")
	}
	deployment, err := filepath.EvalSymlinks(cfg.DeploymentDir)
	if err != nil {
		return errors.New("invalid deployment directory")
	}
	if !filepath.IsAbs(deployment) {
		return errors.New("invalid deployment directory")
	}
	cfg.DeploymentDir = deployment
	for _, p := range []*string{&cfg.ComposeFile, &cfg.EnvFile} {
		if !inside(cfg.DeploymentDir, *p) || regularFile(*p) != nil {
			return errors.New("invalid deployment file")
		}
		resolved, err := filepath.EvalSymlinks(*p)
		if err != nil || !inside(cfg.DeploymentDir, resolved) {
			return errors.New("invalid deployment file")
		}
		*p = resolved
	}
	if err := validateAppURL(cfg.AppBaseURL); err != nil {
		return err
	}
	for _, p := range []string{cfg.SocketPath, cfg.StateFile} {
		if err := safeParent(p); err != nil {
			return err
		}
	}
	return nil
}
func inside(root, value string) bool {
	rel, err := filepath.Rel(root, value)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
func regularFile(p string) error {
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("non-regular file")
	}
	return nil
}
func safeParent(p string) error {
	parent := filepath.Dir(p)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(parent) {
		return errors.New("unsafe parent")
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("unsafe parent")
	}
	return nil
}
func validateAppURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || !isLoopback(u.Hostname()) {
		return errors.New("invalid app base url")
	}
	return nil
}
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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

func (u *Updater) lockPath() string { return u.cfg.StateFile + ".lock" }
func (u *Updater) load() error {
	if err := noSymlink(u.cfg.StateFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	b, err := os.ReadFile(u.cfg.StateFile)
	if errors.Is(err, os.ErrNotExist) {
		if _, e := os.Lstat(u.lockPath()); e == nil {
			return errors.New("stale update lock requires reconciliation")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &u.journal); err != nil {
		return errors.New("invalid update journal")
	}
	active := make([]int, 0, 1)
	for i := range u.journal.Jobs {
		if !terminal(u.journal.Jobs[i].Status) {
			active = append(active, i)
		}
	}
	if len(active) > 1 {
		return errors.New("multiple active update jobs require reconciliation")
	}
	if len(active) == 1 {
		if err := noSymlink(u.lockPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		lockBytes, lockErr := os.ReadFile(u.lockPath())
		if lockErr == nil && strings.TrimSpace(string(lockBytes)) != u.journal.Jobs[active[0]].ID {
			return errors.New("update lock does not match active job")
		}
		if lockErr != nil && !errors.Is(lockErr, os.ErrNotExist) {
			return lockErr
		}
		u.journal.Jobs[active[0]].Status = "outcome_unknown"
		u.journal.Jobs[active[0]].Error = "daemon restarted during update"
		u.journal.Jobs[active[0]].UpdatedAt = time.Now().UTC()
		if err := u.save(); err != nil {
			return err
		}
		if lockErr == nil {
			if err := removeLock(u.lockPath()); err != nil {
				return err
			}
		}
		return nil
	}
	if err := noSymlink(u.lockPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lockBytes, err := os.ReadFile(u.lockPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lockID := strings.TrimSpace(string(lockBytes))
	for _, j := range u.journal.Jobs {
		if j.ID == lockID && terminal(j.Status) {
			return removeLock(u.lockPath())
		}
	}
	return errors.New("stale update lock requires reconciliation")
}

func (u *Updater) Status(ctx context.Context) Status {
	u.mu.Lock()
	missing := u.journal.InstalledVersion == ""
	status := u.statusLocked()
	u.mu.Unlock()
	if !missing {
		return status
	}
	if version, err := u.discover(ctx); err == nil {
		u.mu.Lock()
		if u.journal.InstalledVersion == "" {
			u.journal.InstalledVersion = version
			_ = u.save()
		}
		status = u.statusLocked()
		u.mu.Unlock()
	}
	return status
}
func (u *Updater) statusLocked() Status {
	var job *Job
	for i := len(u.journal.Jobs) - 1; i >= 0; i-- {
		if !terminal(u.journal.Jobs[i].Status) {
			j := u.journal.Jobs[i].Job
			job = &j
			break
		}
	}
	if job == nil && u.journal.Candidate != nil {
		for i := len(u.journal.Jobs) - 1; i >= 0; i-- {
			if terminal(u.journal.Jobs[i].Status) && u.journal.Jobs[i].Version == u.journal.Candidate.Version {
				j := u.journal.Jobs[i].Job
				job = &j
				break
			}
		}
	}
	return Status{InstalledVersion: u.journal.InstalledVersion, InstalledDigest: u.journal.InstalledDigest, Candidate: u.journal.Candidate, UpdateAvailable: u.journal.Candidate != nil && compareVersions(u.journal.Candidate.Version, u.journal.InstalledVersion) > 0, Job: job}
}

func (u *Updater) Check(ctx context.Context) (Status, error) {
	current, err := u.discover(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	candidate, err := u.fetch(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.journal.InstalledVersion = current
	u.journal.Candidate = candidate
	if err := u.save(); err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrInternal, err)
	}
	return u.statusLocked(), nil
}
func (u *Updater) discover(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(u.cfg.AppBaseURL, "/")+"/api/v1/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := u.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("app version unavailable")
	}
	var data struct {
		Version string `json:"version"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	if err = dec.Decode(&data); err != nil || dec.Decode(&struct{}{}) != io.EOF || !strictVersion(data.Version) {
		return "", errors.New("invalid app version")
	}
	return data.Version, nil
}
func (u *Updater) fetch(ctx context.Context) (*Candidate, error) {
	rawURL := "https://github.com/" + u.cfg.Repository + "/releases/latest/download/update-manifest.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("release manifest unavailable")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
	if err != nil || len(b) > maxManifestBytes {
		return nil, errors.New("invalid release manifest")
	}
	var m manifest
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&m); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("invalid release manifest")
	}
	return validateManifest(u.cfg.Repository, m, b)
}
func validateManifest(repo string, m manifest, b []byte) (*Candidate, error) {
	if len(m.Repository) > 160 || len(m.Tag) > 32 || len(m.Version) > 32 || len(m.Commit) != 40 || len(m.PublishedAt) > 40 || len(m.ImageRepository) > 180 || len(m.ImageDigest) != 71 || len(m.ReleaseURL) > 512 || m.SchemaVersion != 1 || m.Repository != repo || m.ImageRepository != "ghcr.io/"+strings.ToLower(repo) || !tagPattern.MatchString(m.Tag) || !strictVersion(m.Version) || m.Version != strings.TrimPrefix(m.Tag, "v") || !commitPattern.MatchString(m.Commit) || !digestPattern.MatchString(m.ImageDigest) || m.ReleaseURL != "https://github.com/"+repo+"/releases/tag/"+m.Tag || len(m.Platforms) == 0 || len(m.Platforms) > 4 {
		return nil, errors.New("invalid release manifest")
	}
	published, err := time.Parse(time.RFC3339, m.PublishedAt)
	if err != nil || published.IsZero() || !strings.HasSuffix(m.PublishedAt, "Z") || published.After(time.Now().UTC().Add(10*time.Minute)) {
		return nil, errors.New("invalid release manifest")
	}
	wanted := "linux/" + runtime.GOARCH
	seen := map[string]bool{}
	found := false
	for _, p := range m.Platforms {
		if !platformPattern.MatchString(p) || seen[p] {
			return nil, errors.New("invalid release manifest")
		}
		seen[p] = true
		if p == wanted {
			found = true
		}
	}
	if !found {
		return nil, errors.New("platform unavailable")
	}
	sum := sha256.Sum256(b)
	return &Candidate{Version: m.Version, Tag: m.Tag, ReleaseURL: m.ReleaseURL, ManifestDigest: "sha256:" + hex.EncodeToString(sum[:]), ImageRef: m.ImageRepository + "@" + m.ImageDigest, Commit: m.Commit, PublishedAt: published}, nil
}

func (u *Updater) Install(ctx context.Context, in InstallRequest) (Job, error) {
	if !idempotencyPattern.MatchString(in.IdempotencyKey) || !strictVersion(in.Version) || !digestPattern.MatchString(in.ManifestDigest) || in.Confirmation != "install "+in.Version+" "+in.ManifestDigest || len(in.ActorUsername) > maxActorText || len(in.RequestID) > maxRequestID || strings.ContainsAny(in.ActorUsername+in.RequestID, "\r\n\t\x00") {
		return Job{}, ErrInvalidRequest
	}
	input := fmt.Sprintf("%s|%s|%d", in.Version, in.ManifestDigest, in.ActorUserID)
	u.mu.Lock()
	for _, old := range u.journal.Jobs {
		if old.Key == in.IdempotencyKey {
			if old.Input != input {
				u.mu.Unlock()
				return Job{}, ErrConflict
			}
			u.mu.Unlock()
			return old.Job, nil
		}
	}
	u.mu.Unlock()
	current, err := u.discover(ctx)
	if err != nil || !strictVersion(current) {
		return Job{}, fmt.Errorf("%w: current version unavailable", ErrUpstream)
	}
	candidate, err := u.fetch(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	u.mu.Lock()
	for _, old := range u.journal.Jobs {
		if old.Key == in.IdempotencyKey {
			if old.Input != input {
				u.mu.Unlock()
				return Job{}, ErrConflict
			}
			u.mu.Unlock()
			return old.Job, nil
		}
	}
	if u.journal.Candidate == nil || candidate.Version != u.journal.Candidate.Version || candidate.ManifestDigest != u.journal.Candidate.ManifestDigest || candidate.ImageRef != u.journal.Candidate.ImageRef || candidate.Version != in.Version || candidate.ManifestDigest != in.ManifestDigest || compareVersions(candidate.Version, current) <= 0 {
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
	jobID := fmt.Sprintf("upd-%d", now.UnixNano())
	if err := u.acquireLock(jobID); err != nil {
		u.mu.Unlock()
		if errors.Is(err, os.ErrExist) {
			return Job{}, ErrConflict
		}
		return Job{}, fmt.Errorf("%w: %v", ErrInternal, err)
	}
	j := storedJob{Job: Job{ID: jobID, Version: in.Version, Status: "queued", CreatedAt: now, UpdatedAt: now}, Key: in.IdempotencyKey, Input: input, ActorUserID: in.ActorUserID, ActorUsername: in.ActorUsername, RequestID: in.RequestID}
	u.journal.InstalledVersion = current
	u.journal.Candidate = candidate
	u.journal.Jobs = append(u.journal.Jobs, j)
	if len(u.journal.Jobs) > 32 {
		u.journal.Jobs = u.journal.Jobs[len(u.journal.Jobs)-32:]
	}
	if err := u.save(); err != nil {
		_ = u.releaseLock()
		u.mu.Unlock()
		return Job{}, fmt.Errorf("%w: %v", ErrInternal, err)
	}
	u.mu.Unlock()
	u.start(func() { u.apply(j.ID, candidate) })
	return j.Job, nil
}

var (
	ErrInvalidRequest = errors.New("invalid update request")
	ErrConflict       = errors.New("update conflict")
	ErrUpstream       = errors.New("update upstream failure")
	ErrInternal       = errors.New("update internal failure")
)

func (u *Updater) Job(id string) (Job, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, j := range u.journal.Jobs {
		if j.ID == id {
			return j.Job, nil
		}
	}
	return Job{}, os.ErrNotExist
}
func (u *Updater) apply(id string, c *Candidate) {
	release := false
	defer func() {
		if release {
			u.mu.Lock()
			_ = u.releaseLock()
			u.mu.Unlock()
		}
	}()
	if err := u.transition(id, "pulling", ""); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), u.cfg.ReadyTimeout)
	defer cancel()
	if err := u.command(ctx, []string{"DEEIX_CHAT_IMAGE=" + c.ImageRef}, "pull", "app"); err != nil {
		release = u.transition(id, "failed", "image pull failed") == nil
		return
	}
	if err := u.transition(id, "applying", ""); err != nil {
		return
	}
	if err := replaceImage(u.cfg.EnvFile, c.ImageRef); err != nil {
		release = u.transition(id, "failed", "deployment environment update failed") == nil
		return
	}
	if err := u.command(ctx, []string{"DEEIX_CHAT_IMAGE=" + c.ImageRef}, "up", "-d", "--no-deps", "app"); err != nil {
		release = u.transition(id, "outcome_unknown", "candidate start failed") == nil
		return
	}
	if err := u.transition(id, "verifying", ""); err != nil {
		return
	}
	if err := u.ready(ctx, c.Version); err != nil {
		release = u.transition(id, "outcome_unknown", "candidate verification failed") == nil
		return
	}
	if err := u.verifyRunningImage(ctx, c.ImageRef); err != nil {
		release = u.transition(id, "outcome_unknown", "candidate image verification failed") == nil
		return
	}
	u.mu.Lock()
	u.journal.InstalledVersion = c.Version
	u.journal.InstalledDigest = strings.TrimPrefix(c.ImageRef, c.ImageRef[:strings.LastIndex(c.ImageRef, "@")+1])
	u.setLocked(id, "succeeded", "")
	release = u.save() == nil
	u.mu.Unlock()
}
func (u *Updater) command(ctx context.Context, env []string, args ...string) error {
	argv := append([]string{"compose", "-f", u.cfg.ComposeFile, "--env-file", u.cfg.EnvFile}, args...)
	return u.run(ctx, "docker", argv, env)
}
func (u *Updater) commandOutput(ctx context.Context, env []string, args ...string) (string, error) {
	argv := append([]string{"compose", "-f", u.cfg.ComposeFile, "--env-file", u.cfg.EnvFile}, args...)
	return u.output(ctx, "docker", argv, env)
}
func (u *Updater) verifyRunningImage(ctx context.Context, expected string) error {
	id, err := u.commandOutput(ctx, nil, "ps", "-q", "app")
	if err != nil || !containerIDPattern.MatchString(strings.TrimSpace(id)) {
		return errors.New("app container unavailable")
	}
	image, err := u.output(ctx, "docker", []string{"inspect", "--format={{.Config.Image}}", strings.TrimSpace(id)}, nil)
	if err != nil || strings.TrimSpace(image) != expected {
		return errors.New("app image mismatch")
	}
	return nil
}
func (u *Updater) waitReady(ctx context.Context, version string) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r, e := u.http.Get(strings.TrimRight(u.cfg.AppBaseURL, "/") + "/readyz")
		if e == nil && r.StatusCode == http.StatusOK {
			r.Body.Close()
			if got, e := u.discover(ctx); e == nil && got == version {
				return nil
			}
		} else if r != nil {
			r.Body.Close()
		}
		time.Sleep(time.Second)
	}
}
func (u *Updater) transition(id, status, message string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.setLocked(id, status, message)
	return u.save()
}
func (u *Updater) setLocked(id, status, message string) {
	for i := range u.journal.Jobs {
		if u.journal.Jobs[i].ID == id {
			u.journal.Jobs[i].Status = status
			u.journal.Jobs[i].Error = message
			u.journal.Jobs[i].UpdatedAt = time.Now().UTC()
		}
	}
}
func (u *Updater) acquireLock(jobID string) error {
	f, err := os.OpenFile(u.lockPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(jobID + "\n"); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(u.lockPath())
		return err
	}
	u.locked = true
	if err := syncParent(filepath.Dir(u.lockPath())); err != nil {
		u.locked = false
		_ = os.Remove(u.lockPath())
		return err
	}
	return nil
}
func (u *Updater) releaseLock() error {
	if !u.locked {
		return nil
	}
	u.locked = false
	return removeLock(u.lockPath())
}
func removeLock(p string) error {
	if err := noSymlink(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncParent(filepath.Dir(p))
}
func (u *Updater) save() error { return atomicWrite(u.cfg.StateFile, mustJSON(u.journal), 0600) }
func mustJSON(v any) []byte    { b, _ := json.Marshal(v); return b }
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := noSymlink(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp := path + ".tmp"
	if err := noSymlink(tmp); err == nil {
		return errors.New("temporary file exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return syncParent(filepath.Dir(path))
}
func syncParent(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func noSymlink(p string) error {
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlink path rejected")
	}
	return nil
}
func replaceImage(path, image string) error {
	if err := regularFile(path); err != nil {
		return err
	}
	info, _ := os.Stat(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "DEEIX_CHAT_IMAGE=") {
			lines[i] = "DEEIX_CHAT_IMAGE=" + image
			found = true
		}
	}
	if !found {
		lines = append(lines, "DEEIX_CHAT_IMAGE="+image)
	}
	return atomicWrite(path, []byte(strings.Join(lines, "\n")), info.Mode().Perm())
}
func terminal(s string) bool { return s == "succeeded" || s == "failed" || s == "outcome_unknown" }
func strictVersion(v string) bool {
	m := versionPattern.FindStringSubmatch(v)
	if m == nil || len(v) > 32 {
		return false
	}
	for _, p := range m[1:] {
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		n, e := strconv.ParseUint(p, 10, 31)
		if e != nil || n > 1<<31-1 {
			return false
		}
	}
	return true
}
func compareVersions(a, b string) int {
	if !strictVersion(a) || !strictVersion(b) {
		return 0
	}
	aa := strings.Split(a, ".")
	bb := strings.Split(b, ".")
	for i := range aa {
		ai, _ := strconv.ParseUint(aa[i], 10, 31)
		bi, _ := strconv.ParseUint(bb[i], 10, 31)
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return 0
}
