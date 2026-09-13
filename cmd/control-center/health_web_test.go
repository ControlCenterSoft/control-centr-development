package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	productui "control-center/internal/ui"
)

type failingHealthOverviewProvider struct{}

func (failingHealthOverviewProvider) HealthOverview(context.Context) (productui.HealthOverview, error) {
	return productui.HealthOverview{}, errors.New("provider-private transport failure")
}

func TestHealthBrowserProviderFailureIsExplicitUnavailable(t *testing.T) {
	fixture := newResourceAuthFixtureWithProductOptions(t, withHealthOverviewProvider(failingHealthOverviewProvider{}))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.AddCookie(fixture.login(t, "viewer"))
	result := httptest.NewRecorder()
	fixture.handler.ServeHTTP(result, request)

	if result.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", result.Code, http.StatusServiceUnavailable, result.Body.String())
	}
	body := result.Body.String()
	for _, expected := range []string{"Недоступны (unavailable)", "Неизвестно (unknown)", "Подтверждённых объектов Health сейчас нет."} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, "provider-private transport failure") {
		t.Fatalf("provider-private error leaked to browser: %s", body)
	}
}

func TestHealthBrowserRejectsTamperedProviderView(t *testing.T) {
	provider := fixedHealthOverviewProvider{view: productui.HealthOverview{
		Schema: productui.HealthOverviewSchemaV1,
		DataState: productui.HealthDataLoaded,
		OverallState: productui.HealthStateHealthy,
		Resources: []productui.ResourceHealthView{},
		Signals: []productui.HealthSignalView{},
		MutationAuthorized: true,
	}}
	fixture := newResourceAuthFixtureWithProductOptions(t, withHealthOverviewProvider(provider))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.AddCookie(fixture.login(t, "viewer"))
	result := httptest.NewRecorder()
	fixture.handler.ServeHTTP(result, request)

	if result.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", result.Code, http.StatusServiceUnavailable, result.Body.String())
	}
	if !strings.Contains(result.Body.String(), "Неизвестно (unknown)") {
		t.Fatalf("tampered view did not fail closed: %s", result.Body.String())
	}
}

func TestHealthBrowserEscapesProviderControlledText(t *testing.T) {
	now := time.Date(2026, 9, 13, 5, 0, 0, 0, time.UTC)
	view, err := productui.BuildHealthOverview(productui.HealthOverviewInput{
		Loaded: true,
		Now: now,
		StaleAfter: 5 * time.Minute,
		ExpiredAfter: 15 * time.Minute,
		Signals: []productui.HealthSignal{{
			ID: "node-1-check",
			ResourceKind: "node",
			ResourceID: "node-1",
			CheckName: "<script>alert(1)</script>",
			State: productui.HealthStateDegraded,
			ObservedAt: now.Add(-time.Minute),
			RunbookRef: "runbook:<unsafe>",
			EvidenceRefs: []string{"evidence:<unsafe>"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := newResourceAuthFixtureWithProductOptions(t, withHealthOverviewProvider(fixedHealthOverviewProvider{view: view}))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.AddCookie(fixture.login(t, "viewer"))
	result := httptest.NewRecorder()
	fixture.handler.ServeHTTP(result, request)

	if result.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", result.Code, result.Body.String())
	}
	body := result.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("unescaped provider text rendered: %s", body)
	}
	for _, expected := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "runbook:&lt;unsafe&gt;", "evidence:&lt;unsafe&gt;"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("escaped body missing %q: %s", expected, body)
		}
	}
}
