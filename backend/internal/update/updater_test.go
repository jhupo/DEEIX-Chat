package update

import (
	"bytes"
	"context"
	"errors"
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

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testManifest(version string) []byte {
	return []byte(`{"schemaVersion":1,"repository":"owner/repo","tag":"v` + version + `","version":"` + version + `","commit":"0123456789abcdef0123456789abcdef01234567","publishedAt":"` + time.Now().UTC().Add(-time.Minute).Format(time.RFC3339) + `","imageRepository":"ghcr.io/owner/repo","imageDigest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","releaseURL":"https://github.com/owner/repo/releases/tag/v` + version + `","platforms":["linux/` + runtime.GOARCH + `"]}`)
}
func response(code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
}
func testUpdater(t *testing.T, manifests ...[]byte) (*Updater, string, *int, *[]string) {
	t.Helper()
	d, _ := filepath.Abs(t.TempDir())
	compose := filepath.Join(d, "compose.yaml")
	env := filepath.Join(d, ".env")
	state := filepath.Join(d, "state", "journal.json")
	socket := filepath.Join(d, "run", "updater.sock")
	_ = os.MkdirAll(filepath.Dir(state), 0700)
	_ = os.MkdirAll(filepath.Dir(socket), 0700)
	_ = os.WriteFile(compose, []byte("services: {}\n"), 0600)
	_ = os.WriteFile(env, []byte("DEEIX_CHAT_IMAGE=old\n"), 0600)
	u := &Updater{cfg: HostConfig{Repository: "owner/repo", SocketPath: socket, StateFile: state, DeploymentDir: d, ComposeFile: compose, EnvFile: env, AppBaseURL: "http://127.0.0.1:8080", PullTimeout: time.Second, ReadyTimeout: time.Second}}
	i := 0
	calls := []string{}
	u.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/version":
			return response(200, []byte(`{"version":"0.3.4"}`)), nil
		case "/readyz":
			return response(200, nil), nil
		default:
			if strings.Contains(r.URL.Host, "github.com") {
				n := i
				if n >= len(manifests) {
					n = len(manifests) - 1
				}
				i++
				return response(200, manifests[n]), nil
			}
			return response(404, nil), nil
		}
	})}
	u.start = func(fn func()) { fn() }
	u.ready = func(context.Context, string) error { return nil }
	u.run = func(_ context.Context, _ string, args []string, envs []string) error {
		calls = append(calls, strings.Join(append(args, envs...), "|"))
		return nil
	}
	u.output = func(_ context.Context, _ string, args []string, _ []string) (string, error) {
		if len(args) > 0 && args[0] == "inspect" {
			return "ghcr.io/owner/repo@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n", nil
		}
		return "0123456789abcdef\n", nil
	}
	return u, env, &i, &calls
}
func installReq(c *Candidate) InstallRequest {
	return InstallRequest{Version: c.Version, ManifestDigest: c.ManifestDigest, Confirmation: "install " + c.Version + " " + c.ManifestDigest, IdempotencyKey: "1234567890abcdef", ActorUserID: 1, ActorUsername: "root", RequestID: "request-1"}
}

func TestWaitReadyUsesConfiguredPortForReadinessAndVersion(t *testing.T) {
	u, _, _, _ := testUpdater(t)
	u.cfg.AppBaseURL = "http://127.0.0.1:50001"
	var targets []string
	u.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		targets = append(targets, r.URL.Host+r.URL.Path)
		switch r.URL.Path {
		case "/readyz":
			return response(http.StatusOK, nil), nil
		case "/api/v1/version":
			return response(http.StatusOK, []byte(`{"version":"0.3.4"}`)), nil
		default:
			return response(http.StatusNotFound, nil), nil
		}
	})

	if err := u.waitReady(context.Background(), "0.3.4"); err != nil {
		t.Fatalf("waitReady() error = %v", err)
	}
	want := []string{"127.0.0.1:50001/readyz", "127.0.0.1:50001/api/v1/version"}
	if strings.Join(targets, ",") != strings.Join(want, ",") {
		t.Fatalf("readiness/version targets = %#v, want %#v", targets, want)
	}
}

func TestStatusRefreshesVersionAfterExternalDeployment(t *testing.T) {
	u, _, _, _ := testUpdater(t, testManifest("0.3.5"))
	u.journal.InstalledVersion = "0.3.4"
	u.journal.InstalledDigest = "sha256:old"
	u.journal.Candidate = &Candidate{Version: "0.3.5"}
	u.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/version" {
			return response(http.StatusOK, []byte(`{"version":"0.3.6"}`)), nil
		}
		return response(http.StatusNotFound, nil), nil
	})

	status := u.Status(context.Background())
	if status.InstalledVersion != "0.3.6" || status.InstalledDigest != "" || status.UpdateAvailable {
		t.Fatalf("status = %#v", status)
	}
}

func TestFreshJournalCheckAndInstallHigherVersion(t *testing.T) {
	u, env, _, calls := testUpdater(t, testManifest("0.3.5"))
	s, e := u.Check(context.Background())
	if e != nil || s.InstalledVersion != "0.3.4" || !s.UpdateAvailable {
		t.Fatalf("check=%#v err=%v", s, e)
	}
	inspected := false
	u.output = func(_ context.Context, _ string, args []string, _ []string) (string, error) {
		if args[0] == "inspect" {
			inspected = true
			return s.Candidate.ImageRef + "\n", nil
		}
		return "0123456789abcdef\n", nil
	}
	j, e := u.Install(context.Background(), installReq(s.Candidate))
	if e != nil {
		t.Fatal(e)
	}
	got, _ := u.Job(j.ID)
	if got.Status != "succeeded" || u.journal.InstalledDigest != "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("job=%#v journal=%#v", got, u.journal)
	}
	b, _ := os.ReadFile(env)
	if !strings.Contains(string(b), s.Candidate.ImageRef) || len(*calls) != 2 || !strings.Contains((*calls)[0], "pull|app|DEEIX_CHAT_IMAGE=") || !strings.Contains((*calls)[1], "up|-d|--no-deps|app|DEEIX_CHAT_IMAGE=") || !inspected {
		t.Fatalf("env/calls %s %#v", b, *calls)
	}
	if _, e = os.Stat(u.lockPath()); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("lock retained")
	}
}

func TestApplyUsesSeparatePullAndReadinessTimeouts(t *testing.T) {
	m := testManifest("0.3.5")
	u, _, _, _ := testUpdater(t, m, m)
	u.cfg.PullTimeout = 30 * time.Minute
	u.cfg.ReadyTimeout = 5 * time.Minute
	var pullRemaining, startRemaining, readyRemaining time.Duration
	u.run = func(ctx context.Context, _ string, args []string, _ []string) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("command context has no deadline")
		}
		if args[len(args)-2] == "pull" {
			pullRemaining = time.Until(deadline)
		} else {
			startRemaining = time.Until(deadline)
		}
		return nil
	}
	u.ready = func(ctx context.Context, _ string) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("ready context has no deadline")
		}
		readyRemaining = time.Until(deadline)
		return nil
	}

	status, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = u.Install(context.Background(), installReq(status.Candidate)); err != nil {
		t.Fatal(err)
	}
	if pullRemaining < 29*time.Minute || startRemaining < 4*time.Minute || startRemaining > 5*time.Minute || readyRemaining < 4*time.Minute || readyRemaining > 5*time.Minute {
		t.Fatalf("timeouts: pull=%s start=%s ready=%s", pullRemaining, startRemaining, readyRemaining)
	}
}
func TestInstallRefetchMismatchRejected(t *testing.T) {
	u, env, _, calls := testUpdater(t, testManifest("0.3.5"), testManifest("0.3.6"))
	s, _ := u.Check(context.Background())
	if _, e := u.Install(context.Background(), installReq(s.Candidate)); !errors.Is(e, ErrConflict) {
		t.Fatalf("err=%v", e)
	}
	b, _ := os.ReadFile(env)
	if string(b) != "DEEIX_CHAT_IMAGE=old\n" || len(*calls) != 0 {
		t.Fatal("mutated")
	}
}
func TestPostStartFailureOutcomeUnknown(t *testing.T) {
	u, env, _, _ := testUpdater(t, testManifest("0.3.5"))
	u.run = func(_ context.Context, _ string, args []string, _ []string) error {
		if args[len(args)-1] == "app" && args[len(args)-2] == "--no-deps" {
			return errors.New("up failed")
		}
		return nil
	}
	s, _ := u.Check(context.Background())
	j, e := u.Install(context.Background(), installReq(s.Candidate))
	if e != nil {
		t.Fatal(e)
	}
	got, _ := u.Job(j.ID)
	b, _ := os.ReadFile(env)
	if got.Status != "outcome_unknown" || !strings.Contains(string(b), s.Candidate.ImageRef) {
		t.Fatalf("%#v %s", got, b)
	}
}
func TestIdempotencyReplayAndConflict(t *testing.T) {
	u, _, _, calls := testUpdater(t, testManifest("0.3.5"))
	s, _ := u.Check(context.Background())
	r := installReq(s.Candidate)
	a, e := u.Install(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	b, e := u.Install(context.Background(), r)
	if e != nil || a.ID != b.ID || len(*calls) != 2 {
		t.Fatal("replay")
	}
	r.RequestID = "other"
	b, e = u.Install(context.Background(), r)
	if e != nil || a.ID != b.ID {
		t.Fatal("request trace should replay")
	}
	r.ActorUserID = 2
	if _, e = u.Install(context.Background(), r); !errors.Is(e, ErrConflict) {
		t.Fatal("changed actor should conflict")
	}
	r.ActorUserID = 1
	r.Version = "0.3.6"
	r.Confirmation = "install " + r.Version + " " + r.ManifestDigest
	if _, e = u.Install(context.Background(), r); !errors.Is(e, ErrConflict) {
		t.Fatal("changed payload should conflict")
	}
}
func TestInstallInvalidRequestCategory(t *testing.T) {
	u, _, _, _ := testUpdater(t, testManifest("0.3.5"))
	if _, err := u.Install(context.Background(), InstallRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err=%v", err)
	}
}
func TestConcurrentLockRejected(t *testing.T) {
	u, _, _, calls := testUpdater(t, testManifest("0.3.5"))
	s, _ := u.Check(context.Background())
	if e := os.WriteFile(u.lockPath(), []byte("other\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := u.Install(context.Background(), installReq(s.Candidate)); !errors.Is(e, ErrConflict) || len(*calls) != 0 {
		t.Fatal("lock")
	}
}
func TestRestartRecoveryAndLockBinding(t *testing.T) {
	for _, name := range []string{"matching", "missing", "mismatch", "orphan", "multiple", "terminal-lock", "terminal-mismatch"} {
		t.Run(name, func(t *testing.T) {
			u, _, _, _ := testUpdater(t, testManifest("0.3.5"))
			now := time.Now()
			if name != "orphan" {
				status := "pulling"
				if strings.HasPrefix(name, "terminal") {
					status = "succeeded"
				}
				u.journal.Jobs = []storedJob{{Job: Job{ID: "a", Status: status, CreatedAt: now}}}
				if name == "multiple" {
					u.journal.Jobs = append(u.journal.Jobs, storedJob{Job: Job{ID: "b", Status: "queued"}})
				}
				_ = u.save()
			}
			if name != "missing" {
				v := "a\n"
				if name == "mismatch" || name == "orphan" || name == "terminal-mismatch" {
					v = "x\n"
				}
				_ = os.WriteFile(u.lockPath(), []byte(v), 0600)
			}
			u2 := &Updater{cfg: u.cfg}
			err := u2.load()
			if name == "matching" || name == "missing" {
				if err != nil || u2.journal.Jobs[0].Status != "outcome_unknown" {
					t.Fatal(err)
				}
			} else if name == "terminal-lock" {
				if err != nil {
					t.Fatal(err)
				}
				if _, statErr := os.Stat(u.lockPath()); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatal("terminal lock retained")
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
func TestStatusHidesHistoricalTerminalJob(t *testing.T) {
	u, _, _, _ := testUpdater(t, testManifest("0.3.5"), testManifest("0.3.5"), testManifest("0.3.6"))
	first, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = u.Install(context.Background(), installReq(first.Candidate)); err != nil {
		t.Fatal(err)
	}
	status, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.Job != nil || status.Candidate == nil || status.Candidate.Version != "0.3.6" {
		t.Fatalf("historical job hid candidate: %#v", status)
	}
	request := installReq(status.Candidate)
	request.IdempotencyKey = "abcdef1234567890"
	if _, err = u.Install(context.Background(), request); err != nil {
		t.Fatalf("new candidate not installable: %v", err)
	}
}
func TestImageMismatchLeavesOutcomeUnknown(t *testing.T) {
	u, _, _, _ := testUpdater(t, testManifest("0.3.5"))
	s, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u.output = func(_ context.Context, _ string, args []string, _ []string) (string, error) {
		if args[0] == "inspect" {
			return "ghcr.io/owner/repo@sha256:different\n", nil
		}
		return "0123456789abcdef\n", nil
	}
	job, err := u.Install(context.Background(), installReq(s.Candidate))
	if err != nil {
		t.Fatal(err)
	}
	got, err := u.Job(job.ID)
	if err != nil || got.Status != "outcome_unknown" {
		t.Fatalf("job=%#v err=%v", got, err)
	}
}
func TestManifestStrictNegatives(t *testing.T) {
	base := manifest{SchemaVersion: 1, Repository: "owner/repo", Tag: "v0.3.5", Version: "0.3.5", Commit: "0123456789abcdef0123456789abcdef01234567", PublishedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), ImageRepository: "ghcr.io/owner/repo", ImageDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ReleaseURL: "https://github.com/owner/repo/releases/tag/v0.3.5", Platforms: []string{"linux/" + runtime.GOARCH}}
	for _, mut := range []func(*manifest){func(m *manifest) { m.Repository = "bad" }, func(m *manifest) { m.Version = "01.3.5" }, func(m *manifest) { m.PublishedAt = time.Now().UTC().Add(11 * time.Minute).Format(time.RFC3339) }, func(m *manifest) { m.Platforms = []string{"linux/" + runtime.GOARCH, "linux/" + runtime.GOARCH} }, func(m *manifest) { m.ImageDigest = "bad" }} {
		m := base
		mut(&m)
		if _, e := validateManifest("owner/repo", m, []byte("{}")); e == nil {
			t.Fatal("accepted")
		}
	}
}
func TestTransitionPersistenceFailureStopsBeforeCommand(t *testing.T) {
	u, _, _, calls := testUpdater(t, testManifest("0.3.5"))
	u.journal.Jobs = []storedJob{{Job: Job{ID: "a", Status: "queued"}}}
	_ = os.WriteFile(u.cfg.StateFile+".tmp", []byte("x"), 0600)
	u.apply("a", &Candidate{ImageRef: "x", Version: "0.3.5"})
	if len(*calls) != 0 {
		t.Fatal("command ran")
	}
}
func TestSymlinkPathsRejected(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "t")
	_ = os.WriteFile(target, nil, 0600)
	link := filepath.Join(d, "link")
	if e := os.Symlink(target, link); e != nil {
		t.Skip(e)
	}
	if e := regularFile(link); e == nil {
		t.Fatal("symlink")
	}
	if e := atomicWrite(target, []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	_ = os.Symlink(target, target+".tmp")
	if e := atomicWrite(target, []byte("x"), 0600); e == nil {
		t.Fatal("tmp symlink")
	}
}
func TestPullUsesCandidateImageAndAppOnly(t *testing.T) {
	u := &Updater{cfg: HostConfig{ComposeFile: "/deploy/compose.yaml", EnvFile: "/deploy/.env"}}
	var gotName string
	var gotArgs, gotEnv []string
	u.run = func(_ context.Context, name string, args []string, env []string) error {
		gotName, gotArgs, gotEnv = name, args, env
		return nil
	}
	image := "ghcr.io/owner/repo@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := u.command(context.Background(), []string{"DEEIX_CHAT_IMAGE=" + image}, "pull", "app"); err != nil {
		t.Fatal(err)
	}
	if gotName != "docker" || strings.Join(gotArgs, " ") != "compose -f /deploy/compose.yaml --env-file /deploy/.env pull app" || len(gotEnv) != 1 || gotEnv[0] != "DEEIX_CHAT_IMAGE="+image {
		t.Fatalf("name=%q args=%q env=%q", gotName, gotArgs, gotEnv)
	}
}
func TestPullFailureLeavesEnvUnchanged(t *testing.T) {
	d := t.TempDir()
	env := filepath.Join(d, ".env")
	before := []byte("DEEIX_CHAT_IMAGE=old\nOTHER=value\n")
	if err := os.WriteFile(env, before, 0600); err != nil {
		t.Fatal(err)
	}
	u := &Updater{cfg: HostConfig{EnvFile: env}, run: func(context.Context, string, []string, []string) error { return errors.New("pull failed") }}
	if err := u.command(context.Background(), []string{"DEEIX_CHAT_IMAGE=candidate"}, "pull", "app"); err == nil {
		t.Fatal("expected pull failure")
	}
	after, err := os.ReadFile(env)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("env changed: %q %v", after, err)
	}
}
func TestCommandEnvOverridesAmbientImage(t *testing.T) {
	t.Setenv("DEEIX_CHAT_IMAGE", "wrong")
	env := commandEnv([]string{"DEEIX_CHAT_IMAGE=candidate"})
	count := 0
	for _, value := range env {
		if strings.HasPrefix(value, "DEEIX_CHAT_IMAGE=") {
			count++
			if value != "DEEIX_CHAT_IMAGE=candidate" {
				t.Fatalf("unexpected image environment: %q", value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("image environment count=%d", count)
	}
}
