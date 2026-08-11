package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginRequestAcceptsTurnstileToken(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"email":"user@example.test","password":"secret","turnstileToken":"turnstile-token"}`))
	context.Request.Header.Set("Content-Type", "application/json")

	var request LoginRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		t.Fatalf("ShouldBindJSON() error = %v", err)
	}
	if request.TurnstileToken != "turnstile-token" {
		t.Fatalf("TurnstileToken = %q", request.TurnstileToken)
	}
}

func TestOptionalRequestFieldsRetainJSONOmitEmpty(t *testing.T) {
	tests := []struct {
		typeOf    reflect.Type
		fieldName string
	}{
		{reflect.TypeOf(PatchMeRequest{}), "AvatarURL"},
		{reflect.TypeOf(PatchMeRequest{}), "DisplayName"},
		{reflect.TypeOf(PatchMeRequest{}), "Timezone"},
		{reflect.TypeOf(PatchMeRequest{}), "Locale"},
		{reflect.TypeOf(PatchMeRequest{}), "ProfilePreferences"},
		{reflect.TypeOf(PatchMeRequest{}), "AppearancePreferences"},
		{reflect.TypeOf(UpdateCurrentSessionLocationRequest{}), "AccuracyMeters"},
		{reflect.TypeOf(UpdateCurrentSessionLocationRequest{}), "Timezone"},
	}

	for _, test := range tests {
		field, ok := test.typeOf.FieldByName(test.fieldName)
		if !ok {
			t.Fatalf("field %s.%s not found", test.typeOf.Name(), test.fieldName)
		}
		if !strings.Contains(field.Tag.Get("json"), ",omitempty") {
			t.Errorf("field %s.%s JSON tag = %q, want omitempty", test.typeOf.Name(), test.fieldName, field.Tag.Get("json"))
		}
	}
}

func TestLoginResponseOmitsSuccessFieldsForTwoFactorChallenge(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	challenge, err := json.Marshal(LoginResponse{TwoFactorRequired: true, TwoFactorChallengeToken: "challenge-token"})
	if err != nil {
		t.Fatalf("marshal challenge response: %v", err)
	}
	var challengeFields map[string]any
	if err = json.Unmarshal(challenge, &challengeFields); err != nil {
		t.Fatalf("unmarshal challenge response: %v", err)
	}
	for _, key := range []string{"user", "accessToken", "sessionID", "expiresAt", "refreshExpiresAt"} {
		if _, ok := challengeFields[key]; ok {
			t.Fatalf("challenge response contains %q: %s", key, challenge)
		}
	}

	success, err := json.Marshal(LoginResponse{AccessToken: "access-token", SessionID: "session-id", ExpiresAt: &now, RefreshExpiresAt: &now})
	if err != nil {
		t.Fatalf("marshal success response: %v", err)
	}
	var successFields map[string]any
	if err = json.Unmarshal(success, &successFields); err != nil {
		t.Fatalf("unmarshal success response: %v", err)
	}
	for _, key := range []string{"accessToken", "sessionID", "expiresAt", "refreshExpiresAt"} {
		if _, ok := successFields[key]; !ok {
			t.Fatalf("success response omits %q: %s", key, success)
		}
	}
}

func TestIdentityAuthorityRoutesExcludeLocalCapabilities(t *testing.T) {
	router := gin.New()
	module := NewModule(&Handler{})
	module.RegisterPublicRoutes(router.Group("/api"))
	module.RegisterProtectedRoutes(router.Group("/api"))
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /api/auth/password/reset/start",
		"POST /api/auth/password/reset/complete",
		"GET /api/auth/providers/:slug/start",
		"POST /api/auth/providers/:slug/callback",
		"POST /api/me/2fa/setup/start",
		"POST /api/me/identities/providers/:slug/callback",
	} {
		if registered[route] {
			t.Fatalf("legacy identity route remains registered: %s", route)
		}
	}
	for _, route := range []string{
		"GET /api/auth/login-options",
		"POST /api/auth/register/email/start",
		"POST /api/auth/register/email/complete",
		"POST /api/auth/login",
		"POST /api/auth/login/2fa",
		"POST /api/auth/refresh",
		"GET /api/me",
		"PATCH /api/me",
		"PUT /api/me/password",
		"GET /api/auth/sessions",
		"PUT /api/auth/sessions/current/location",
		"POST /api/auth/sessions/:session_id/logout",
		"POST /api/auth/logout",
		"POST /api/auth/logout-all",
	} {
		if !registered[route] {
			t.Fatalf("current auth route is not registered: %s", route)
		}
	}
}
