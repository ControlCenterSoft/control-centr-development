package ui

import (
	"context"
	"testing"
	"time"

	"control-center/internal/incidents"
)

func TestIncidentOperationalReportProviderQueriesOnlyActiveStatuses(t *testing.T) {
	reader := &incidentReportReaderStub{pages: []incidents.ListPage{{}}}
	provider, err := NewIncidentOperationalReportProvider(reader, func() time.Time {
		return time.Date(2026, 9, 13, 5, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OperationalReport(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reader.queries) != 1 {
		t.Fatalf("queries = %#v", reader.queries)
	}
	statuses := reader.queries[0].Statuses
	if len(statuses) != 2 || statuses[0] != incidents.StatusAcknowledged || statuses[1] != incidents.StatusOpen {
		t.Fatalf("active statuses = %#v", statuses)
	}
}

func TestIncidentOperationalReportProviderRejectsResolvedItemFromFilteredReader(t *testing.T) {
	resolved := reportTestIncident(incidents.StatusResolved, incidents.SeverityWarning)
	reader := &incidentReportReaderStub{pages: []incidents.ListPage{{Items: []incidents.Incident{resolved}}}}
	provider, err := NewIncidentOperationalReportProvider(reader, func() time.Time {
		return time.Date(2026, 9, 13, 5, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OperationalReport(context.Background()); err == nil {
		t.Fatal("OperationalReport() error = nil, want active-status filter violation")
	}
}
