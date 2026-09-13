package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	productui "control-center/internal/ui"
)

func TestSessionSecurityWorkspaceAdapterUsesAuthenticatedSelfOnlyContext(t *testing.T) {
	fixture := newHTTPFixture(t)
	first := fixture.login(t, "viewer")
	_ = fixture.login(t, "viewer")

	handler := fixture.server.Authenticate(fixture.server.RequirePasswordCurrent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("authenticated principal missing")
		}
		workspace, err := fixture.server.buildSessionSecurityWorkspace(r.Context(), principal, remoteIP(r), time.Now().UTC())
		if err != nil {
			t.Fatalf("buildSessionSecurityWorkspace() error = %v", err)
		}
		if workspace.Schema != productui.SessionSecurityWorkspaceSchemaV1 || workspace.DataState != productui.SessionSecurityDataLoaded {
			t.Fatalf("workspace identity = %#v", workspace)
		}
		if !workspace.SelfOnly || workspace.MutationAuthorized || workspace.Identity == nil || workspace.Identity.ID != "viewer-1" {
			t.Fatalf("workspace authority = %#v", workspace)
		}
		if len(workspace.Sessions) != 2 || !workspace.Sessions[0].Current {
			t.Fatalf("workspace sessions = %#v", workspace.Sessions)
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	request := httptest.NewRequest(http.MethodGet, "/security/sessions", nil)
	request.AddCookie(first)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	if result.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
}

func TestSessionSecurityWorkspaceAdapterCannotBypassFirstLogin(t *testing.T) {
	fixture := newHTTPFixture(t)
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{\"username\":\"admin\",\"password\":\"admin\"}"))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResult := httptest.NewRecorder()
	fixture.server.ServeHTTP(loginResult, loginRequest)
	if loginResult.Code != http.StatusOK {
		t.Fatalf("admin login status=%d body=%s", loginResult.Code, loginResult.Body.String())
	}
	cookies := loginResult.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("admin login cookies=%d", len(cookies))
	}

	called := false
	handler := fixture.server.Authenticate(fixture.server.RequirePasswordCurrent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})))
	request := httptest.NewRequest(http.MethodGet, "/security/sessions", nil)
	request.AddCookie(cookies[0])
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	if called || result.Code != http.StatusForbidden {
		t.Fatalf("first-login bypass: called=%v status=%d body=%s", called, result.Code, result.Body.String())
	}
}
