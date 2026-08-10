package admin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/update"
)

func TestMapUpdateError(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{errors.New("internal"), http.StatusInternalServerError},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{update.ErrInvalidRequest, http.StatusBadRequest},
		{os.ErrNotExist, http.StatusNotFound},
		{update.ErrConflict, http.StatusConflict},
		{update.ErrUpstream, http.StatusBadGateway},
	}
	for _, tc := range cases {
		got, _ := mapUpdateError(tc.err)
		if got != tc.status {
			t.Fatalf("%v: %d", tc.err, got)
		}
	}
}
