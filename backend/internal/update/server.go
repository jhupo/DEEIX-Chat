package update

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var jobIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func Serve(u *Updater, socket string) error {
	if err := prepareSocket(socket); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0750); err != nil {
		return err
	}
	l, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	if err = os.Chmod(socket, 0660); err != nil {
		_ = l.Close()
		return err
	}
	return (&http.Server{Handler: newHandler(u), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}).Serve(l)
}

func newHandler(u *Updater) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodGet) || !emptyBody(r) {
			http.Error(w, "invalid request", 400)
			return
		}
		writeJSON(w, u.Status(r.Context()))
	})
	mux.HandleFunc("/v1/check", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodPost) || !emptyBody(r) {
			http.Error(w, "invalid request", 400)
			return
		}
		s, e := u.Check(r.Context())
		if e != nil {
			http.Error(w, "check failed", updaterErrorStatus(e))
			return
		}
		writeJSON(w, s)
	})
	mux.HandleFunc("/v1/install", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodPost) {
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			http.Error(w, "invalid request", 400)
			return
		}
		defer r.Body.Close()
		var in InstallRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "invalid request", 400)
			return
		}
		j, e := u.Install(r.Context(), in)
		if e != nil {
			http.Error(w, "install failed", updaterErrorStatus(e))
			return
		}
		writeJSON(w, j)
	})
	mux.HandleFunc("/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodGet) || !emptyBody(r) {
			http.Error(w, "invalid request", 400)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
		if id == "" || strings.Contains(id, "/") || !jobIDPattern.MatchString(id) {
			http.Error(w, "not found", 404)
			return
		}
		j, e := u.Job(id)
		if e != nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, j)
	})
	return mux
}
func updaterErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrUpstream):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	w.Header().Set("Allow", want)
	w.WriteHeader(http.StatusMethodNotAllowed)
	return false
}
func emptyBody(r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	var b [1]byte
	n, err := r.Body.Read(b[:])
	return n == 0 && err == io.EOF
}
func prepareSocket(socket string) error {
	info, err := os.Lstat(socket)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return errors.New("unsafe socket path")
	}
	conn, err := net.DialTimeout("unix", socket, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return errors.New("updater socket already active")
	}
	return os.Remove(socket)
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
