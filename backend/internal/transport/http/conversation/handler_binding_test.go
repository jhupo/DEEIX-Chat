package conversation

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindConversationJSONIsStrictAndValidates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for name, body := range map[string]string{
		"missing execution": `{"title":"test"}`,
		"unknown field":     `{"execution":{"type":"cloud"},"extra":true}`,
		"multiple values":   `{"execution":{"type":"cloud"}} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", strings.NewReader(body))
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			var input CreateConversationRequest
			if err := bindConversationJSON(context, &input, 4096); err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}

	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"execution":{"type":"cloud"}}`))
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	var input CreateConversationRequest
	if err := bindConversationJSON(context, &input, 4096); err != nil {
		t.Fatalf("valid request was rejected: %v", err)
	}
}
