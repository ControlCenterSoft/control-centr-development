package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	productui "control-center/internal/ui"
)

type fixedHealthOverviewProvider struct {
	view productui.HealthOverview
}

func (p fixedHealthOverviewProvider) HealthOverview(context.Context) (productui.HealthOverview, error) {
	return p.view, nil
}

func healthAuthProvider(t *testing.T) productui.HealthOverviewProvider {
	t.Helper()
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	view, err := productui.BuildHealthOverview(productui.HealthOverviewInput{
		Loaded: true, Now: now, StaleAfter: 5 * time.Minute, ExpiredAfter: 15 * time.Minute,
		Signals: []productui.HealthSignal{{
			ID: "node-1-ready", ResourceKind: "node", ResourceID: "node-1",
			CheckName: "readiness", State: productui.HealthStateHealthy, ObservedAt: now.Add(-time.Minute),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixedHealthOverviewProvider{view: view}
}

func TestHealthOverviewEndpointRequiresGlobalResourcesRead(t *testing.T) {
	fixture := newResourceAuthFixtureWithProductOptions(t, withHealthOverviewProvider(healthAuthProvider(t)))
	tests := []struct {
		name       string
		username   string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "unbound", username: "unbound", wantStatus: http.StatusForbidden},
		{name: "site-scoped-viewer", username: "siteviewer", wantStatus: http.StatusForbidden},
		{name: "viewer", username: "viewer", wantStatus: http.StatusOK},
		{name: "auditor", username: "auditor", wantStatus: http.StatusOK},
		{name: "operator", username: "operator", wantStatus: http.StatusOK},
		{name: "administrator", username: "admin", wantStatus: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/ui/health", nil)
			if test.username != "" {
				request.AddCookie(fixture.login(t, test.username))
			}
			result := httptest.NewRecorder()
			fixture.handler.ServeHTTP(result, request)
			if result.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", result.Code, test.wantStatus, result.Body.String())
			}
			if result.Code == http.StatusOK && result.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control=%q want=no-store", result.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestHealthBrowserRequiresGlobalResourcesRead(t *testing.T) {
	fixture := newResourceAuthFixtureWithProductOptions(t, withHealthOverviewProvider(healthAuthProvider(t)))
	tests := []struct {
		name       string
		username   string
		wantStatus int
		wantBody   string
	}{
		{name: "anonymous-redirect", wantStatus: http.StatusSeeOther},
		{name: "unbound", username: "unbound", wantStatus: http.StatusForbidden},
		{name: "site-scoped-viewer", username: "siteviewer", wantStatus: http.StatusForbidden},
		{name: "viewer", username: "viewer", wantStatus: http.StatusOK, wantBody: "Состояние инфраструктуры"},
		{name: "auditor", username: "auditor", wantStatus: http.StatusOK, wantBody: "Исправно (healthy)"},
		{name: "operator", username: "operator", wantStatus: http.StatusOK, wantBody: "node-1"},
		{name: "administrator", username: "admin", wantStatus: http.StatusOK, wantBody: "readiness"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/health", nil)
			if test.username != "" {
				request.AddCookie(fixture.login(t, test.username))
			}
			result := httptest.NewRecorder()
			fixture.handler.ServeHTTP(result, request)
			if result.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", result.Code, test.wantStatus, result.Body.String())
			}
			if test.wantBody != "" && !strings.Contains(result.Body.String(), test.wantBody) {
				t.Fatalf("body missing %q: %s", test.wantBody, result.Body.String())
			}
			if result.Code == http.StatusOK {
				for header, want := range map[string]string{
					"Cache-Control": "no-store",
					"X-Content-Type-Options": "nosniff",
					"X-Frame-Options": "DENY",
					"Referrer-Policy": "no-referrer",
				} {
					if got := result.Header().Get(header); got != want {
						t.Fatalf("%s=%q want=%q", header, got, want)
					}
				}
				if !strings.Contains(result.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
					t.Fatalf("CSP=%q missing frame-ancestors", result.Header().Get("Content-Security-Policy"))
				}
			}
		})
	}
}

func TestHealthOverviewRoutesRemainAbsentWithoutAuthoritativeProvider(t *testing.T) {
	fixture := newResourceAuthFixture(t)
	for _, path := range []string{"/api/v1/ui/health", "/health"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(fixture.login(t, "viewer"))
		result := httptest.NewRecorder()
		fixture.handler.ServeHTTP(result, request)
		if result.Code != http.StatusNotFound {
			t.Fatalf("GET %s status=%d want=404 body=%s", path, result.Code, result.Body.String())
		}
	}
}
