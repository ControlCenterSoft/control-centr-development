package httpapi

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

type healthOverviewProviderFunc func(context.Context) (productui.HealthOverview, error)

func (f healthOverviewProviderFunc) HealthOverview(ctx context.Context) (productui.HealthOverview, error) {
	return f(ctx)
}

func TestHealthOverviewHandlerReturnsValidatedLoadedView(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	view, err := productui.BuildHealthOverview(productui.HealthOverviewInput{
		Loaded: true, Now: now, StaleAfter: 5 * time.Minute, ExpiredAfter: 15 * time.Minute,
		Signals: []productui.HealthSignal{{
			ID: "node-a-ready", ResourceKind: "node", ResourceID: "node-a",
			CheckName: "ready", State: productui.HealthStateHealthy, ObservedAt: now.Add(-time.Minute),
		}},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}

	handler := HealthOverviewHandler(healthOverviewProviderFunc(func(context.Context) (productui.HealthOverview, error) {
		return view, nil
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ui/health", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if body := response.Body.String(); body == "" || body == "{}\n" {
		t.Fatalf("unexpected empty Health response: %q", body)
	}
}

func TestHealthOverviewHandlerSourceFailureIsExplicitUnavailable(t *testing.T) {
	handler := HealthOverviewHandler(healthOverviewProviderFunc(func(context.Context) (productui.HealthOverview, error) {
		return productui.HealthOverview{}, errors.New("source unavailable")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ui/health", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	body := response.Body.String()
	for _, expected := range []string{`"data_state":"unavailable"`, `"overall_state":"unknown"`, `"mutation_authorized":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("unavailable body missing %q: %s", expected, body)
		}
	}
}

func TestHealthOverviewHandlerRejectsTamperedProviderView(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	view, err := productui.BuildHealthOverview(productui.HealthOverviewInput{
		Loaded: true, Now: now, StaleAfter: time.Minute, ExpiredAfter: 5 * time.Minute,
		Signals: []productui.HealthSignal{{
			ID: "node-a-ready", ResourceKind: "node", ResourceID: "node-a",
			CheckName: "ready", State: productui.HealthStateHealthy, ObservedAt: now.Add(-2 * time.Minute),
		}},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	view.Signals[0].EffectiveState = productui.HealthStateHealthy

	handler := HealthOverviewHandler(healthOverviewProviderFunc(func(context.Context) (productui.HealthOverview, error) {
		return view, nil
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ui/health", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{`"data_state":"unavailable"`, `"overall_state":"unknown"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("tampered view fail-closed body missing %q: %s", expected, body)
		}
	}
}

func TestHealthOverviewHandlerNilProviderFailsClosed(t *testing.T) {
	response := httptest.NewRecorder()
	HealthOverviewHandler(nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ui/health", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthOverviewHandlerIsGetOnly(t *testing.T) {
	response := httptest.NewRecorder()
	HealthOverviewHandler(nil).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/ui/health", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
}
