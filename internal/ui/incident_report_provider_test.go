package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"control-center/internal/incidents"
)

type incidentReportReaderStub struct {
	pages   []incidents.ListPage
	failAt  int
	err     error
	queries []incidents.ListQuery
}

func (s *incidentReportReaderStub) Get(context.Context, string) (incidents.Incident, error) {
	return incidents.Incident{}, incidents.ErrNotFound
}

func (s *incidentReportReaderStub) List(_ context.Context, query incidents.ListQuery) (incidents.ListPage, error) {
	index := len(s.queries)
	s.queries = append(s.queries, query)
	if s.err != nil && index == s.failAt {
		return incidents.ListPage{}, s.err
	}
	if index >= len(s.pages) {
		return incidents.ListPage{}, errors.New("unexpected incident report page read")
	}
	return s.pages[index], nil
}

func TestIncidentOperationalReportProviderLoadsCompletePaginatedSource(t *testing.T) {
	now := time.Date(2026, 9, 13, 5, 0, 0, 0, time.UTC)
	warning := reportTestIncident(incidents.StatusOpen, incidents.SeverityWarning)
	critical := reportTestIncident(incidents.StatusOpen, incidents.SeverityCritical)
	critical.ObjectID = "incident:report-2"
	critical.ResourceVersion = "rv-report-critical"
	critical.AffectedResources[0].ID = "node-2"

	reader := &incidentReportReaderStub{pages: []incidents.ListPage{
		{
			Items:   []incidents.Incident{warning},
			HasMore: true,
			Next:    &incidents.ListCursor{StartedAt: warning.StartedAt, ObjectID: warning.ObjectID},
		},
		{Items: []incidents.Incident{critical}},
	}}
	provider, err := NewIncidentOperationalReportProvider(reader, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewIncidentOperationalReportProvider() error = %v", err)
	}

	report, err := provider.OperationalReport(context.Background())
	if err != nil {
		t.Fatalf("OperationalReport() error = %v", err)
	}
	if report.DataState != ReportDataLoaded || report.OverallState != ReportHealthUnhealthy {
		t.Fatalf("report state = %#v", report)
	}
	if len(report.Evidence) != 2 || report.MutationAuthorized {
		t.Fatalf("report evidence = %#v mutation=%v", report.Evidence, report.MutationAuthorized)
	}
	if len(reader.queries) != 2 || reader.queries[1].Before == nil {
		t.Fatalf("queries = %#v", reader.queries)
	}
	if reader.queries[1].Before.ObjectID != warning.ObjectID || !reader.queries[1].Before.StartedAt.Equal(warning.StartedAt) {
		t.Fatalf("second page cursor = %#v", reader.queries[1].Before)
	}
}

func TestIncidentOperationalReportProviderEmptyLoadedSourceRemainsUnknown(t *testing.T) {
	reader := &incidentReportReaderStub{pages: []incidents.ListPage{{}}}
	provider, err := NewIncidentOperationalReportProvider(reader, func() time.Time {
		return time.Date(2026, 9, 13, 5, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := provider.OperationalReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.DataState != ReportDataLoaded || report.OverallState != ReportHealthUnknown || len(report.Evidence) != 0 {
		t.Fatalf("empty report invented state: %#v", report)
	}
}

func TestIncidentOperationalReportProviderFailsClosedOnReaderError(t *testing.T) {
	backendErr := errors.New("incident repository unavailable")
	reader := &incidentReportReaderStub{failAt: 0, err: backendErr}
	provider, err := NewIncidentOperationalReportProvider(reader, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OperationalReport(context.Background()); !errors.Is(err, backendErr) {
		t.Fatalf("OperationalReport() error = %v, want wrapped backend error", err)
	}
}

func TestIncidentOperationalReportProviderRejectsInconsistentPagination(t *testing.T) {
	incident := reportTestIncident(incidents.StatusOpen, incidents.SeverityWarning)
	provider, err := NewIncidentOperationalReportProvider(&incidentReportReaderStub{pages: []incidents.ListPage{{
		Items:   []incidents.Incident{incident},
		HasMore: true,
	}}}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OperationalReport(context.Background()); err == nil || !strings.Contains(err.Error(), "pagination evidence") {
		t.Fatalf("OperationalReport() error = %v, want fail-closed pagination rejection", err)
	}
}

func TestIncidentOperationalReportProviderRejectsNonAdvancingCursor(t *testing.T) {
	incident := reportTestIncident(incidents.StatusOpen, incidents.SeverityWarning)
	cursor := &incidents.ListCursor{StartedAt: incident.StartedAt, ObjectID: incident.ObjectID}
	reader := &incidentReportReaderStub{pages: []incidents.ListPage{
		{Items: []incidents.Incident{incident}, HasMore: true, Next: cursor},
		{Items: []incidents.Incident{incident}, HasMore: true, Next: cursor},
	}}
	provider, err := NewIncidentOperationalReportProvider(reader, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OperationalReport(context.Background()); err == nil {
		t.Fatal("OperationalReport() error = nil, want repeated-page rejection")
	}
}

func TestIncidentOperationalReportProviderRejectsEvidenceOverflow(t *testing.T) {
	items := make([]incidents.Incident, 0, 16)
	for incidentIndex := 0; incidentIndex < 16; incidentIndex++ {
		incident := reportTestIncident(incidents.StatusOpen, incidents.SeverityWarning)
		incident.ObjectID = fmt.Sprintf("incident:report-%02d", incidentIndex)
		incident.ResourceVersion = fmt.Sprintf("rv-report-%02d", incidentIndex)
		incident.AffectedResources = make([]incidents.ResourceRef, 0, 64)
		for resourceIndex := 0; resourceIndex < 64; resourceIndex++ {
			incident.AffectedResources = append(incident.AffectedResources, incidents.ResourceRef{
				Kind:    "node",
				ID:      fmt.Sprintf("node-%02d-%02d", incidentIndex, resourceIndex),
				ScopeID: "site:primary",
			})
		}
		items = append(items, incident)
	}
	provider, err := NewIncidentOperationalReportProvider(&incidentReportReaderStub{
		pages: []incidents.ListPage{{Items: items}},
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OperationalReport(context.Background()); err == nil || !strings.Contains(err.Error(), "evidence exceeds") {
		t.Fatalf("OperationalReport() error = %v, want bounded evidence rejection", err)
	}
}

func TestNewIncidentOperationalReportProviderRequiresDependencies(t *testing.T) {
	if _, err := NewIncidentOperationalReportProvider(nil, time.Now); err == nil {
		t.Fatal("NewIncidentOperationalReportProvider(nil, ...) error = nil")
	}
	reader := &incidentReportReaderStub{}
	if _, err := NewIncidentOperationalReportProvider(reader, nil); err == nil {
		t.Fatal("NewIncidentOperationalReportProvider(..., nil) error = nil")
	}
}
