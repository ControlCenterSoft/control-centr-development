package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"control-center/internal/identity/audit"
)

func TestAuditEventsBoundedMetadataSearchIsPrivacyPreserving(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	now := time.Now().UTC().Truncate(time.Second)

	if err := fixture.log.Append(context.Background(), audit.Event{
		OccurredAt: now,
		Action: "config.reconcile",
		Outcome: "success",
		CorrelationID: "reconcile-12345",
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.log.Append(context.Background(), audit.Event{
		OccurredAt: now.Add(time.Second),
		Action: "unrelated.action",
		Outcome: "success",
		CorrelationID: "other-correlation",
		Details: map[string]any{"note": "reconcile-12345"},
	}); err != nil {
		t.Fatal(err)
	}

	from := url.QueryEscape(now.Add(-time.Minute).Format(time.RFC3339Nano))
	to := url.QueryEscape(now.Add(time.Minute).Format(time.RFC3339Nano))
	result := fixture.get(t, cookie, "/api/v1/audit/events?search=RECONCILE-12345&from="+from+"&to="+to)
	if result.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", result.Code, result.Body.String())
	}
	var response auditEventsResponse
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Events) != 1 || response.Events[0].CorrelationID != "reconcile-12345" {
		t.Fatalf("unexpected search response: %#v", response.Events)
	}

	var searchEvidence *audit.Event
	for _, event := range fixture.log.Records() {
		if event.Action == "audit.events_list" && event.Outcome == "success" && event.ActorID == "auditor-1" {
			copy := event
			searchEvidence = &copy
		}
	}
	if searchEvidence == nil {
		t.Fatal("metadata search did not produce audit read evidence")
	}
	if searchEvidence.Details["search_applied"] != true {
		t.Fatalf("search evidence does not record bounded search usage: %#v", searchEvidence.Details)
	}
	if strings.Contains(strings.ToLower(fmt.Sprint(searchEvidence.Details)), "reconcile-12345") {
		t.Fatalf("search evidence retained the raw search term: %#v", searchEvidence.Details)
	}
}

func TestAuditEventsRejectsUnboundedOrAmbiguousMetadataSearch(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	from := "2026-09-01T00:00:00Z"
	to := "2026-09-02T00:00:00Z"

	for _, target := range []string{
		"/api/v1/audit/events?search=login",
		"/api/v1/audit/events?search=ab&from=" + from + "&to=" + to,
		"/api/v1/audit/events?search=login&from=invalid&to=" + to,
		"/api/v1/audit/events?search=login&from=" + to + "&to=" + from,
		"/api/v1/audit/events?search=login&from=2026-09-01T00:00:00Z&to=2026-10-03T00:00:01Z",
		"/api/v1/audit/events?search=login&search=logout&from=" + from + "&to=" + to,
	} {
		result := fixture.get(t, cookie, target)
		if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "invalid_audit_query") {
			t.Fatalf("%s status=%d body=%s", target, result.Code, result.Body.String())
		}
	}
}
