package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionSecurityBrowserEscapesSessionMetadataAndKeepsSecurityHeaders(t *testing.T) {
	fixture := newSessionSecurityHTTPFixture(t)
	loginBody, err := json.Marshal(map[string]string{
		"username": "viewer",
		"password": "a secure test password",
	})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("User-Agent", `<script>alert("session")</script>`)
	loginResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(loginResult, login)
	if loginResult.Code != http.StatusOK || len(loginResult.Result().Cookies()) != 1 {
		t.Fatalf("login status=%d body=%s", loginResult.Code, loginResult.Body.String())
	}
	cookie := loginResult.Result().Cookies()[0]

	request := httptest.NewRequest(http.MethodGet, "/web/security/sessions", nil)
	request.AddCookie(cookie)
	result := httptest.NewRecorder()
	fixture.server.ServeHTTP(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("workspace status=%d body=%s", result.Code, result.Body.String())
	}
	body := result.Body.String()
	if strings.Contains(body, `<script>alert("session")</script>`) {
		t.Fatalf("session metadata was rendered without HTML escaping: %s", body)
	}
	if !strings.Contains(body, `&lt;script&gt;alert`) {
		t.Fatalf("escaped user-agent evidence missing from self-only workspace: %s", body)
	}
	if strings.Contains(body, cookie.Value) || strings.Contains(body, "a secure test password") {
		t.Fatal("browser workspace disclosed session token or password material")
	}
	if result.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", result.Header().Get("Cache-Control"))
	}
	if result.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("X-Frame-Options=%q", result.Header().Get("X-Frame-Options"))
	}
	if result.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q", result.Header().Get("X-Content-Type-Options"))
	}
	if !strings.Contains(result.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("Content-Security-Policy=%q", result.Header().Get("Content-Security-Policy"))
	}
}
