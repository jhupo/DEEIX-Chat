package agentgateway

import (
	"net/http/httptest"
	"strings"
	"testing"

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
