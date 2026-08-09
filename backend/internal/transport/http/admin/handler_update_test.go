package admin

import (
	"context"
	"errors"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/update"
	"net/http"
	"testing"
)

func TestMapUpdateError(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{{errors.New("network"), 503}, {context.DeadlineExceeded, 504}, {&update.HTTPError{Status: http.StatusBadRequest}, 400}, {&update.HTTPError{Status: http.StatusNotFound}, 404}, {&update.HTTPError{Status: http.StatusConflict}, 409}, {&update.HTTPError{Status: http.StatusInternalServerError}, 502}, {&update.HTTPError{Status: http.StatusBadGateway}, 502}}
	for _, tc := range cases {
		got, _ := mapUpdateError(tc.err)
		if got != tc.status {
			t.Fatalf("%v: %d", tc.err, got)
		}
	}
}
