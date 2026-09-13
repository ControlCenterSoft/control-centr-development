package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"control-center/internal/identity/audit"
)

func TestAuditWebIsPermissionBoundPrivacyMinimizedAndAudited(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	if err := fixture.log.Append(context.Background(), audit.Event{
		Action: "security.web-test", Outcome: "failed", ActorID: "actor-1", SubjectID: "node-1",
		CorrelationID: "corr-1", SourceIP: "203.0.113.88",
		Details: map[string]any{"api_token": "top-secret", "message": "private-detail"},
	}); err != nil {
		t.Fatal(err)
	}

	result := fixture.get(t, cookie, "/web/audit?action=security.web-test&limit=25")
	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	if result.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("content-type=%q", result.Header().Get("Content-Type"))
	}
	if result.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%q", result.Header().Get("Cache-Control"))
	}
	body := result.Body.String()
	for _, expected := range []string{"security.web-test", "failed", "actor-1", "node-1", "corr-1", "/api/v1/audit/events/export?"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("audit web body missing %q: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"203.0.113.88", "top-secret", "private-detail"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("audit web body leaked %q: %s", forbidden, body)
		}
	}

	foundEvidence := false
	for _, event := range fixture.log.Records() {
		if event.Action == "audit.events_web" && event.Outcome == "success" && event.ActorID == "auditor-1" {
			foundEvidence = true
		}
	}
	if !foundEvidence {
		t.Fatal("successful Audit web read did not create audit evidence")
	}
}

func TestAuditWebRequiresPermissionAndCurrentPassword(t *testing.T) {
	fixture := newAuditEventsFixture(t)

	viewer := fixture.login(t, "viewer-audit", "a secure test password")
	viewerResult := fixture.get(t, viewer, "/web/audit")
	if viewerResult.Code != http.StatusForbidden || !strings.Contains(viewerResult.Body.String(), "Forbidden") {
		t.Fatalf("viewer status=%d body=%s", viewerResult.Code, viewerResult.Body.String())
	}

	admin := fixture.login(t, "admin-audit", "admin")
	adminResult := fixture.get(t, admin, "/web/audit")
	if adminResult.Code != http.StatusSeeOther || adminResult.Header().Get("Location") != "/password/change" {
		t.Fatalf("bootstrap admin status=%d location=%q body=%s", adminResult.Code, adminResult.Header().Get("Location"), adminResult.Body.String())
	}
}

func TestAuditWebRejectsAmbiguousQuery(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	result := fixture.get(t, cookie, "/web/audit?limit=25&limit=50")
	if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "Некорректные фильтры Audit") {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
}

func TestAuditWebDoesNotRenderEventsWhenReadEvidenceCannotBeRecorded(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	if err := fixture.log.Append(context.Background(), audit.Event{
		Action: "sensitive.web.audit", Outcome: "success", SubjectID: "must-not-render",
	}); err != nil {
		t.Fatal(err)
	}
	fixture.server.audit = failingAuditLogger{}

	result := fixture.get(t, cookie, "/web/audit?action=sensitive.web.audit")
	if result.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	if strings.Contains(result.Body.String(), "must-not-render") || strings.Contains(result.Body.String(), "sensitive.web.audit") {
		t.Fatalf("Audit web disclosed data after evidence failure: %s", result.Body.String())
	}
}

func TestAuditWebPaginationKeepsExactFiltersAndOffersBoundedExport(t *testing.T) {
	fixture := newAuditEventsFixture(t)
	cookie := fixture.login(t, "auditor", "a secure test password")
	for index := 0; index < 26; index++ {
		if err := fixture.log.Append(context.Background(), audit.Event{
			Action: "security.page-test", Outcome: "success", ActorID: "actor-page", SubjectID: "node-page",
		}); err != nil {
			t.Fatal(err)
		}
	}

	result := fixture.get(t, cookie, "/web/audit?action=security.page-test&outcome=success&actor_id=actor-page&subject_id=node-page&limit=25")
	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	body := result.Body.String()
	if !strings.Contains(body, "Следующая страница") || !strings.Contains(body, "cursor=") {
		t.Fatalf("pagination missing: %s", body)
	}
	for _, expected := range []string{"action=security.page-test", "outcome=success", "actor_id=actor-page", "subject_id=node-page", "limit=25"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("filtered navigation missing %q: %s", expected, body)
		}
	}
}
