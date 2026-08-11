package sub2api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

func TestLoginAndVerifyCurrentUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(writer http.ResponseWriter, request *http.Request) {
		var input map[string]string
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatalf("decode login request: %v", err)
		}
		if input["email"] != "admin@example.test" || input["password"] != "secret" || input["turnstile_token"] != "turnstile-token" {
			t.Fatalf("unexpected login input: %#v", input)
		}
		writeEnvelope(writer, map[string]any{
			"access_token":  "access",
			"refresh_token": "refresh",
			"expires_in":    3600,
			"token_type":    "Bearer",
			"user":          map[string]any{"id": 7, "email": "admin@example.test", "username": "admin", "role": "admin", "status": "active"},
		})
	})
	mux.HandleFunc("/api/v1/auth/me", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("unexpected Authorization header %q", request.Header.Get("Authorization"))
		}
		writeEnvelope(writer, map[string]any{"id": 7, "email": "admin@example.test", "username": "admin", "role": "admin", "status": "active"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	login, err := client.Login(context.Background(), " admin@example.test ", "secret", " turnstile-token ")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.AccessToken != "access" || login.RefreshToken != "refresh" {
		t.Fatalf("unexpected token pair: %#v", login.TokenPair)
	}
	user, err := client.Me(context.Background(), login.AccessToken)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if user.ID != 7 || user.Role != "admin" {
		t.Fatalf("unexpected user: %#v", user)
	}
}

func TestRejectsCrossOriginRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", "https://example.com")
		writer.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err = client.Settings(context.Background()); err == nil {
		t.Fatal("expected cross-origin redirect to fail")
	}
}

func TestChangePasswordUsesCurrentUserRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/user/password" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var input map[string]string
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if input["old_password"] != "old-secret" || input["new_password"] != "new-secret" || len(input) != 2 {
			t.Fatalf("input = %#v", input)
		}
		writeEnvelope(writer, map[string]any{"message": "changed"})
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, sharedsecurity.NewStrictOutboundPolicy(false))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err = client.ChangePassword(context.Background(), "access", "old-secret", "new-secret"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
}

func writeEnvelope(writer http.ResponseWriter, data any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "message": "success", "data": data})
}
