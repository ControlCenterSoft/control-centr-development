package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionSecurityWorkspaceHandlerReturnsSelfOnlyNoStoreProjection(t *testing.T) {
	fixture := newHTTPFixture(t)
	first := fixture.login(t, "viewer")
	_ = fixture.login(t, "viewer")

	handler := fixture.server.Authenticate(fixture.server.RequirePasswordCurrent(http.HandlerFunc(fixture.server.sessionSecurityWorkspace)))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ui/session-security", nil)
	request.RemoteAddr = "192.0.2.10:4242"
	request.AddCookie(first)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)

	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	if result.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", result.Header().Get("Cache-Control"))
	}
	body := result.Body.String()
	for _, required := range []string{
		`"schema":"ui.session-security-workspace/v1"`,
		`"self_only":true`,
		`"mutation_authorized":false`,
		`"current":true`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("body missing %s: %s", required, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "token") || strings.Contains(strings.ToLower(body), "password") {
		t.Fatalf("credential-like material leaked in workspace response: %s", body)
	}
}

func TestSessionSecurityWorkspaceHandlerRequiresAuthenticationAndCurrentPassword(t *testing.T) {
	fixture := newHTTPFixture(t)
	handler := fixture.server.Authenticate(fixture.server.RequirePasswordCurrent(http.HandlerFunc(fixture.server.sessionSecurityWorkspace)))

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/ui/session-security", nil)
	unauthenticatedResult := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedResult, unauthenticated)
	if unauthenticatedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticatedResult.Code, unauthenticatedResult.Body.String())
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{\"username\":\"admin\",\"password\":\"admin\"}"))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(loginResult, loginRequest)
	if loginResult.Code != http.StatusOK || len(loginResult.Result().Cookies()) != 1 {
		t.Fatalf("bootstrap login status=%d body=%s", loginResult.Code, loginResult.Body.String())
	}

	firstLogin := httptest.NewRequest(http.MethodGet, "/api/v1/ui/session-security", nil)
	firstLogin.AddCookie(loginResult.Result().Cookies()[0])
	firstLoginResult := httptest.NewRecorder()
	handler.ServeHTTP(firstLoginResult, firstLogin)
	if firstLoginResult.Code != http.StatusForbidden || !strings.Contains(firstLoginResult.Body.String(), "password_change_required") {
		t.Fatalf("first-login status=%d body=%s", firstLoginResult.Code, firstLoginResult.Body.String())
	}
}
