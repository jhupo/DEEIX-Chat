package agentgateway

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/agentgateway"
	"github.com/gin-gonic/gin"
)

func TestBindStrictJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "valid", body: `{"name":"workstation"}`},
		{name: "unknown field", body: `{"name":"workstation","admin":true}`, wantErr: true},
		{name: "trailing value", body: `{"name":"workstation"} {}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			var request renameDeviceRequest
			err := bindStrictJSON(context, &request, smallJSONBodyLimit)
			if (err != nil) != test.wantErr {
				t.Fatalf("bindStrictJSON() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestWriteErrorExplainsRuntimeKeyMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	writeError(context, errors.Join(errors.New("verify runtime proof"), appagent.ErrRuntimeAuth), "fallback")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("writeError() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "local Codex API key does not belong to this DEEIX account or has been disabled") {
		t.Fatalf("writeError() body = %s", body)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"errorCode":"agent.runtime_key_invalid"`) {
		t.Fatalf("writeError() body = %s", body)
	}
}
