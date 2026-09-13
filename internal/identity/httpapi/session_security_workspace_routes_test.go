package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newSessionSecurityHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	fixture := newHTTPFixture(t)
	fixture.server.registerSessionSecurityWorkspaceRoutes()
	return fixture
}

func loginWithPassword(t *testing.T, server *Server, username, password string) *http.Cookie {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	server.ServeHTTP(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", result.Code, result.Body.String())
	}
	cookies := result.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func setSameOriginFormHeaders(request *http.Request) {
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://"+request.Host)
}

func TestSessionSecurityWorkspaceAPIIsSelfOnlyAndCredentialFree(t *testing.T) {
	fixture := newSessionSecurityHTTPFixture(t)

	anonymous := httptest.NewRecorder()
	fixture.server.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/v1/identity/session-security", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d body=%s", anonymous.Code, anonymous.Body.String())
	}

	cookie := fixture.login(t, "viewer")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/identity/session-security?actor_id=admin-1", nil)
	request.AddCookie(cookie)
	result := httptest.NewRecorder()
	fixture.server.ServeHTTP(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("workspace status=%d body=%s", result.Code, result.Body.String())
	}
	body := result.Body.String()
	if !strings.Contains(body, `"self_only":true`) || !strings.Contains(body, `"username":"viewer"`) {
		t.Fatalf("workspace lost self-only identity binding: %s", body)
	}
	for _, forbidden := range []string{"admin-1", cookie.Value, "password_hash", "token_digest", "a secure test password"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("workspace disclosed forbidden value %q: %s", forbidden, body)
		}
	}
	if result.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", result.Header().Get("Cache-Control"))
	}
}

func TestSessionSecurityBrowserRequiresCompletedFirstLogin(t *testing.T) {
	fixture := newSessionSecurityHTTPFixture(t)
	adminCookie := loginWithPassword(t, fixture.server, "admin", "admin")

	browserRequest := httptest.NewRequest(http.MethodGet, "/web/security/sessions", nil)
	browserRequest.AddCookie(adminCookie)
	browserResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(browserResult, browserRequest)
	if browserResult.Code != http.StatusSeeOther || browserResult.Header().Get("Location") != "/password/change" {
		t.Fatalf("browser first-login result=%d location=%q", browserResult.Code, browserResult.Header().Get("Location"))
	}

	apiRequest := httptest.NewRequest(http.MethodGet, "/api/v1/identity/session-security", nil)
	apiRequest.AddCookie(adminCookie)
	apiResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(apiResult, apiRequest)
	if apiResult.Code != http.StatusForbidden || !strings.Contains(apiResult.Body.String(), "password_change_required") {
		t.Fatalf("api first-login status=%d body=%s", apiResult.Code, apiResult.Body.String())
	}
}

func TestSessionSecurityBrowserRejectsCrossOriginOrQuerySelectedMutation(t *testing.T) {
	fixture := newSessionSecurityHTTPFixture(t)
	cookie := fixture.login(t, "viewer")
	form := url.Values{"session_id": {strings.Repeat("a", 32)}}

	crossOrigin := httptest.NewRequest(http.MethodPost, "/web/security/sessions/revoke", strings.NewReader(form.Encode()))
	crossOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	crossOrigin.Header.Set("Origin", "https://attacker.invalid")
	crossOrigin.AddCookie(cookie)
	crossOriginResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(crossOriginResult, crossOrigin)
	if crossOriginResult.Code != http.StatusForbidden {
		t.Fatalf("cross-origin revoke status=%d", crossOriginResult.Code)
	}

	querySelected := httptest.NewRequest(http.MethodPost, "/web/security/sessions/revoke?session_id="+strings.Repeat("b", 32), strings.NewReader(form.Encode()))
	setSameOriginFormHeaders(querySelected)
	querySelected.AddCookie(cookie)
	querySelectedResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(querySelectedResult, querySelected)
	if querySelectedResult.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("query-selected revoke status=%d", querySelectedResult.Code)
	}
}

func TestSessionSecurityBrowserRevokesOnlyOwnedSessionAndPreservesCurrent(t *testing.T) {
	fixture := newSessionSecurityHTTPFixture(t)
	older := fixture.login(t, "viewer")
	current := fixture.login(t, "viewer")

	inventoryRequest := httptest.NewRequest(http.MethodGet, "/api/v1/identity/session-security", nil)
	inventoryRequest.AddCookie(current)
	inventoryResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(inventoryResult, inventoryRequest)
	if inventoryResult.Code != http.StatusOK {
		t.Fatalf("inventory status=%d body=%s", inventoryResult.Code, inventoryResult.Body.String())
	}
	var response struct {
		Workspace struct {
			Sessions []struct {
				ID      string `json:"id"`
				Current bool   `json:"current"`
			} `json:"sessions"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(inventoryResult.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var olderID string
	for _, session := range response.Workspace.Sessions {
		if !session.Current {
			olderID = session.ID
		}
	}
	if olderID == "" {
		t.Fatal("expected an older non-current session")
	}

	form := url.Values{"session_id": {olderID}}
	revokeRequest := httptest.NewRequest(http.MethodPost, "/web/security/sessions/revoke", strings.NewReader(form.Encode()))
	setSameOriginFormHeaders(revokeRequest)
	revokeRequest.AddCookie(current)
	revokeResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(revokeResult, revokeRequest)
	if revokeResult.Code != http.StatusSeeOther || revokeResult.Header().Get("Location") != "/web/security/sessions" {
		t.Fatalf("revoke status=%d location=%q body=%s", revokeResult.Code, revokeResult.Header().Get("Location"), revokeResult.Body.String())
	}

	currentCheck := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	currentCheck.AddCookie(current)
	currentResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(currentResult, currentCheck)
	if currentResult.Code != http.StatusOK {
		t.Fatalf("current session was incorrectly revoked: status=%d", currentResult.Code)
	}
	olderCheck := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	olderCheck.AddCookie(older)
	olderResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(olderResult, olderCheck)
	if olderResult.Code != http.StatusUnauthorized {
		t.Fatalf("older session remained active: status=%d", olderResult.Code)
	}
}

func TestSessionSecurityCurrentRevokeClearsCookieAndRequiresLogin(t *testing.T) {
	fixture := newSessionSecurityHTTPFixture(t)
	cookie := fixture.login(t, "viewer")

	inventoryRequest := httptest.NewRequest(http.MethodGet, "/api/v1/identity/session-security", nil)
	inventoryRequest.AddCookie(cookie)
	inventoryResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(inventoryResult, inventoryRequest)
	var response struct {
		Workspace struct {
			Sessions []struct {
				ID      string `json:"id"`
				Current bool   `json:"current"`
			} `json:"sessions"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(inventoryResult.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var currentID string
	for _, session := range response.Workspace.Sessions {
		if session.Current {
			currentID = session.ID
		}
	}
	if currentID == "" {
		t.Fatal("current session id missing")
	}

	form := url.Values{"session_id": {currentID}}
	revokeRequest := httptest.NewRequest(http.MethodPost, "/web/security/sessions/revoke", strings.NewReader(form.Encode()))
	setSameOriginFormHeaders(revokeRequest)
	revokeRequest.AddCookie(cookie)
	revokeResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(revokeResult, revokeRequest)
	if revokeResult.Code != http.StatusSeeOther || revokeResult.Header().Get("Location") != "/login" {
		t.Fatalf("current revoke status=%d location=%q", revokeResult.Code, revokeResult.Header().Get("Location"))
	}
	cleared := false
	for _, responseCookie := range revokeResult.Result().Cookies() {
		if responseCookie.Name == fixture.server.cookieName && responseCookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("current-session revoke did not clear browser session cookie")
	}

	after := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	after.AddCookie(cookie)
	afterResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(afterResult, after)
	if afterResult.Code != http.StatusUnauthorized {
		t.Fatalf("revoked current session still authenticates: status=%d", afterResult.Code)
	}
}
