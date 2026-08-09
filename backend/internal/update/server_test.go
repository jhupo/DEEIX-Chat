package update

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdaterErrorStatus(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{ErrInvalidRequest, http.StatusBadRequest},
		{fmt.Errorf("wrapped: %w", ErrConflict), http.StatusConflict},
		{ErrUpstream, http.StatusBadGateway},
		{ErrInternal, http.StatusInternalServerError},
		{errors.New("other"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := updaterErrorStatus(tc.err); got != tc.want {
			t.Fatalf("%v: got %d want %d", tc.err, got, tc.want)
		}
	}
}

func TestServerStrictRequestTable(t *testing.T) {
	h := newHandler(&Updater{journal: journal{Jobs: []storedJob{{Job: Job{ID: "known", Status: "succeeded"}}}}})
	cases := []struct {
		name, method, path, contentType, body string
		want                                  int
	}{
		{"wrong method", http.MethodPost, "/v1/jobs/known", "", "", 405},
		{"chunked get body", http.MethodGet, "/v1/jobs/known", "", "x", 400},
		{"bad media", http.MethodPost, "/v1/install", "text/plain", "{}", 400},
		{"unknown json", http.MethodPost, "/v1/install", "application/json", "{\"unknown\":true}", 400},
		{"trailing json", http.MethodPost, "/v1/install", "application/json", "{}{}", 400},
		{"extra segment", http.MethodGet, "/v1/jobs/known/extra", "", "", 404},
		{"invalid id", http.MethodGet, "/v1/jobs/a/b", "", "", 404},
		{"missing job", http.MethodGet, "/v1/jobs/missing", "", "", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
		})
	}
}
