package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	productui "control-center/internal/ui"
)

type fixedOperationalReportProvider struct {
	report productui.OperationalReport
}

func (p fixedOperationalReportProvider) OperationalReport(context.Context) (productui.OperationalReport, error) {
	return p.report, nil
}

func reportsAuthProvider(t *testing.T) productui.OperationalReportProvider {
	t.Helper()
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	report, err := productui.BuildOperationalReport(now, productui.ReportDataLoaded, []productui.ReportEvidenceInput{
		{
			ID:            "health-node-1-api",
			Kind:          "health",
			ResourceKind:  "node",
			ResourceID:    "node-1",
			ObservedState: productui.ReportHealthHealthy,
			Freshness:     productui.ReportFreshnessCurrent,
			ObservedAt:    now.Add(-time.Minute),
			ReasonCode:    "health.healthy",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixedOperationalReportProvider{report: report}
}

func TestResourceReportsPreserveGlobalResourcesReadBoundary(t *testing.T) {
	provider := reportsAuthProvider(t)
	fixture := newResourceAuthFixtureWithProductOptions(t, withResourceReportsProvider(provider))

	for _, path := range []string{
		"/api/v1/ui/reports/resources",
		"/api/v1/ui/reports/resources/evidence?resource_kind=node&resource_id=node-1",
	} {
		t.Run(path, func(t *testing.T) {
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
					request := httptest.NewRequest(http.MethodGet, path, nil)
					if test.username != "" {
						request.AddCookie(fixture.login(t, test.username))
					}
					result := httptest.NewRecorder()
					fixture.handler.ServeHTTP(result, request)
					if result.Code != test.wantStatus {
						t.Fatalf("status=%d want=%d body=%s", result.Code, test.wantStatus, result.Body.String())
					}
				})
			}
		})
	}
}

func TestAuditReportsDoNotInheritResourcesRead(t *testing.T) {
	provider := reportsAuthProvider(t)
	fixture := newResourceAuthFixtureWithProductOptions(t, withAuditReportsProvider(provider))

	for _, path := range []string{
		"/api/v1/ui/reports/audit",
		"/api/v1/ui/reports/audit/evidence?resource_kind=node&resource_id=node-1",
	} {
		t.Run(path, func(t *testing.T) {
			tests := []struct {
				name       string
				username   string
				wantStatus int
			}{
				{name: "anonymous", wantStatus: http.StatusUnauthorized},
				{name: "unbound", username: "unbound", wantStatus: http.StatusForbidden},
				{name: "site-scoped-viewer", username: "siteviewer", wantStatus: http.StatusForbidden},
				{name: "viewer", username: "viewer", wantStatus: http.StatusForbidden},
				{name: "operator", username: "operator", wantStatus: http.StatusForbidden},
				{name: "auditor", username: "auditor", wantStatus: http.StatusOK},
				{name: "administrator", username: "admin", wantStatus: http.StatusOK},
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					request := httptest.NewRequest(http.MethodGet, path, nil)
					if test.username != "" {
						request.AddCookie(fixture.login(t, test.username))
					}
					result := httptest.NewRecorder()
					fixture.handler.ServeHTTP(result, request)
					if result.Code != test.wantStatus {
						t.Fatalf("status=%d want=%d body=%s", result.Code, test.wantStatus, result.Body.String())
					}
				})
			}
		})
	}
}

func TestReportsRoutesRemainAbsentWithoutAuthoritativeProvider(t *testing.T) {
	fixture := newResourceAuthFixture(t)
	for _, path := range []string{
		"/api/v1/ui/reports/resources",
		"/api/v1/ui/reports/audit",
	} {
		result := httptest.NewRecorder()
		fixture.handler.ServeHTTP(result, httptest.NewRequest(http.MethodGet, path, nil))
		if result.Code != http.StatusNotFound {
			t.Fatalf("GET %s status=%d want=404 body=%s", path, result.Code, result.Body.String())
		}
	}
}
