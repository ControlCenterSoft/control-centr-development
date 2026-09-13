package ui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"control-center/internal/incidents"
)

func TestIncidentOperationalReportProviderDoesNotExposeIncidentPrivateFields(t *testing.T) {
	incident := reportTestIncident(incidents.StatusAcknowledged, incidents.SeverityWarning)
	incident.Title = "private incident title"
	incident.Signals[0].Summary = "provider-private diagnostic summary"
	incident.Signals[0].Source = "private-provider"
	incident.Acknowledgement.ActorID = "user:private-operator"

	reader := &incidentReportReaderStub{pages: []incidents.ListPage{{Items: []incidents.Incident{incident}}}}
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
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"private incident title",
		"provider-private diagnostic summary",
		"private-provider",
		"user:private-operator",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("operational report leaked private incident field %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, incident.AffectedResources[0].ID) {
		t.Fatalf("operational report lost required resource identity: %s", serialized)
	}
	if report.MutationAuthorized {
		t.Fatal("MutationAuthorized = true, want false")
	}
}
