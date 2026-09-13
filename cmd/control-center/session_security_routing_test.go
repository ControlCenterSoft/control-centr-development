package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSplitHandlerRoutesSessionSecurityWorkspaceToIdentity(t *testing.T) {
	identityCalls := 0
	coreCalls := 0
	handler := splitHandler{
		identity: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identityCalls++
			w.WriteHeader(http.StatusNoContent)
		}),
		core: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			coreCalls++
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	for _, path := range []string{
		"/api/v1/identity/session-security",
		"/web/security/sessions",
		"/web/security/sessions/revoke",
		"/web/security/sessions/revoke-all",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		if result.Code != http.StatusNoContent {
			t.Fatalf("path=%s status=%d want=%d", path, result.Code, http.StatusNoContent)
		}
	}
	if identityCalls != 4 || coreCalls != 0 {
		t.Fatalf("identityCalls=%d coreCalls=%d", identityCalls, coreCalls)
	}
}
