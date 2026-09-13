package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-center/internal/corecontracts"
	"control-center/internal/incidents"
)

type incidentWebServiceStub struct {
	incident  incidents.Incident
	page      incidents.ListPage
	getErr    error
	listErr   error
	getCalls  int
	listCalls int
	lastQuery incidents.ListQuery
	lastActor string
}

func (s *incidentWebServiceStub) Get(_ context.Context, actor, _ string) (incidents.Incident, error) {
	s.getCalls++
	s.lastActor = actor
	return s.incident, s.getErr
}

func (s *incidentWebServiceStub) List(_ context.Context, actor string, query incidents.ListQuery) (incidents.ListPage, error) {
	s.listCalls++
	s.lastActor = actor
	s.lastQuery = query
	return s.page, s.listErr
}

func (s *incidentWebServiceStub) Acknowledge(context.Context, string, string, incidents.AcknowledgeCommand) (incidents.Incident, error) {
	return incidents.Incident{}, nil
}

func (s *incidentWebServiceStub) Resolve(context.Context, string, string, incidents.ResolveCommand) (incidents.Incident, error) {
	return incidents.Incident{}, nil
}

func (s *incidentWebServiceStub) UpdateEvidence(context.Context, string, string, incidents.EvidenceUpdateCommand) (incidents.Incident, error) {
	return incidents.Incident{}, nil
}

func TestIncidentWebListUsesServerActorExactFiltersAndNoStore(t *testing.T) {
	incident := validIncidentWebFixture()
	service := &incidentWebServiceStub{page: incidents.ListPage{Items: []incidents.Incident{incident}}}
	handler := NewWithBrowser(service, fixedActor("user:operator"))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui?status=open&severity=warning&scope_id=site:primary&resource_kind=node&resource_id=node-1&limit=25", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "text/html; charset=utf-8" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers=%v", response.Header())
	}
	if service.listCalls != 1 || service.lastActor != "user:operator" {
		t.Fatalf("list calls=%d actor=%q", service.listCalls, service.lastActor)
	}
	if len(service.lastQuery.Statuses) != 1 || service.lastQuery.Statuses[0] != incidents.StatusOpen ||
		len(service.lastQuery.Severities) != 1 || service.lastQuery.Severities[0] != incidents.SeverityWarning ||
		service.lastQuery.ScopeID != "site:primary" || service.lastQuery.ResourceKind != "node" ||
		service.lastQuery.ResourceID != "node-1" || service.lastQuery.Limit != 25 {
		t.Fatalf("unexpected query: %#v", service.lastQuery)
	}
	body := response.Body.String()
	for _, expected := range []string{"Потеря свежести сетевой проверки", "warning", "open", "site:primary", "incident:inc-1"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body missing %q: %s", expected, body)
		}
	}
}

func TestIncidentWebDetailRendersBoundedEvidenceWithoutMutationControls(t *testing.T) {
	incident := validIncidentWebFixture()
	service := &incidentWebServiceStub{incident: incident}
	handler := NewWithBrowser(service, fixedActor("user:operator"))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui/incident:inc-1", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"node-1", "network-verification-stale", "core-network",
		"runbook:network-verification", "audit:evt-1", "read-only",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("detail body missing %q: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"/acknowledge", "/resolve", "production_mutation_allowed"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("detail body exposed mutation control %q: %s", forbidden, body)
		}
	}
	if service.getCalls != 1 || service.lastActor != "user:operator" {
		t.Fatalf("get calls=%d actor=%q", service.getCalls, service.lastActor)
	}
}

func TestIncidentWebRejectsInvalidQueryBeforeService(t *testing.T) {
	service := &incidentWebServiceStub{}
	handler := NewWithBrowser(service, fixedActor("user:operator"))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui?search=secret", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.listCalls != 0 {
		t.Fatalf("invalid query reached service %d times", service.listCalls)
	}
}

func TestIncidentWebFailsClosedOnAccessDeniedAndInvalidStoredEvidence(t *testing.T) {
	service := &incidentWebServiceStub{listErr: incidents.ErrOperatorAccessDenied}
	handler := NewWithBrowser(service, fixedActor("user:viewer"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui?scope_id=site:primary", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", response.Code, response.Body.String())
	}

	broken := validIncidentWebFixture()
	broken.Title = ""
	service = &incidentWebServiceStub{page: incidents.ListPage{Items: []incidents.Incident{broken}}}
	handler = NewWithBrowser(service, fixedActor("user:operator"))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "incident:inc-1") {
		t.Fatalf("invalid evidence status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestIncidentWebRejectsInconsistentPaginationEvidence(t *testing.T) {
	incident := validIncidentWebFixture()
	service := &incidentWebServiceStub{page: incidents.ListPage{
		Items:   []incidents.Incident{incident},
		HasMore: true,
	}}
	handler := NewWithBrowser(service, fixedActor("user:operator"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui?limit=10", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing cursor status=%d body=%s", response.Code, response.Body.String())
	}

	wrong := &incidents.ListCursor{StartedAt: incident.StartedAt.Add(-time.Hour), ObjectID: "incident:other"}
	service = &incidentWebServiceStub{page: incidents.ListPage{
		Items:   []incidents.Incident{incident},
		HasMore: true,
		Next:    wrong,
	}}
	handler = NewWithBrowser(service, fixedActor("user:operator"))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui?limit=10", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("mismatched cursor status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestIncidentWebPaginationPreservesBoundedExactFilters(t *testing.T) {
	incident := validIncidentWebFixture()
	next := &incidents.ListCursor{StartedAt: incident.StartedAt, ObjectID: incident.ObjectID}
	service := &incidentWebServiceStub{page: incidents.ListPage{Items: []incidents.Incident{incident}, HasMore: true, Next: next}}
	handler := NewWithBrowser(service, fixedActor("user:operator"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/ui?status=open&severity=warning&scope_id=site:primary&limit=10", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{"before_started_at=", "before_object_id=incident%3Ainc-1", "status=open", "severity=warning", "scope_id=site%3Aprimary", "limit=10"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("pagination missing %q: %s", expected, body)
		}
	}
}

func TestIncidentAPIRemainsJSONWhenBrowserSurfaceIsEnabled(t *testing.T) {
	service := &incidentWebServiceStub{page: incidents.ListPage{Items: []incidents.Incident{}}}
	handler := NewWithBrowser(service, fixedActor("user:operator"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents?limit=5", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func validIncidentWebFixture() incidents.Incident {
	started := time.Date(2026, 9, 13, 0, 10, 0, 0, time.UTC)
	observed := started.Add(2 * time.Minute)
	evidence := incidents.EvidenceRef{
		Kind: "audit-event", ID: "audit:evt-1", SHA256: strings.Repeat("a", 64),
		Collected: observed, Redaction: incidents.RedactionApplied,
	}
	return incidents.Incident{
		ObjectMetadata: corecontracts.ObjectMetadata{
			ObjectID: "incident:inc-1", ScopeID: "site:primary", OwnerScope: "installation:default",
			Generation: 1, ResourceVersion: "rv-1", CreatedAt: started, UpdatedAt: observed,
		},
		Severity: incidents.SeverityWarning, Status: incidents.StatusOpen,
		Title: "Потеря свежести сетевой проверки", StartedAt: started, LastObservedAt: observed,
		AffectedResources: []incidents.ResourceRef{{Kind: "node", ID: "node-1", ScopeID: "site:primary"}},
		Signals: []incidents.Signal{{
			ID: "signal-1", Kind: "network-verification-stale", Source: "core-network",
			ObservedAt: observed, Summary: "Последняя connectivity verification вышла за freshness budget",
			Evidence: []incidents.EvidenceRef{evidence},
		}},
		Timeline: []incidents.TimelineEntry{
			{Kind: incidents.TimelineOpened, At: started},
			{Kind: incidents.TimelineSignalObserved, At: observed, Summary: "Зафиксирован сигнал потери freshness", Evidence: []incidents.EvidenceRef{evidence}},
		},
		Runbook:  &incidents.RunbookRef{ID: "runbook:network-verification", Revision: "v1"},
		Evidence: []incidents.EvidenceRef{evidence},
	}
}
