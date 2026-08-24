package agentgateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func TestStreamBrowserEventsPushesHubChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &Handler{hub: newBridgeHub()}
	router := gin.New()
	router.GET("/events", func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, uint(7))
		handler.StreamBrowserEvents(c)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "application/x-ndjson") {
		t.Fatalf("event stream content type = %q", contentType)
	}

	reader := bufio.NewReader(response.Body)
	if line, readErr := reader.ReadString('\n'); readErr != nil || line != "{\"type\":\"ready\"}\n" {
		t.Fatalf("ready event = %q, %v", line, readErr)
	}
	handler.hub.notifyUser(7)
	if line, readErr := reader.ReadString('\n'); readErr != nil || line != "{\"type\":\"change\"}\n" {
		t.Fatalf("change event = %q, %v", line, readErr)
	}
}

func TestRuntimeProfileProjectionPreservesApprovalReviewerCapability(t *testing.T) {
	items := toRuntimeProfileDocs([]appagent.RuntimeProfileView{{
		ProfileID: "codex-default", DeviceID: "agd_device", Provider: "codex", Status: "ready",
		Manifest: json.RawMessage(`{"provider":"codex","threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"approvalsReviewer":["user","auto_review"],"sandboxPolicy":["workspace-write"]}}`),
	}})
	if len(items) != 1 || len(items[0].Manifest.ThreadSettings.ApprovalsReviewer) != 2 ||
		items[0].Manifest.ThreadSettings.ApprovalsReviewer[1] != "auto_review" {
		t.Fatalf("runtime profile approval reviewers = %#v", items)
	}
}

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
