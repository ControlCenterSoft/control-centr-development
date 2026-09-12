package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/identity/audit"
)

func TestPostgresAuditReadBoundedMetadataSearch(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log, err := NewAuditLog(db)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	marker := "metadata-" + suffix
	now := time.Now().UTC().Truncate(time.Second)
	if err := log.Append(ctx, audit.Event{
		OccurredAt: now,
		Action: "integration.audit-search",
		Outcome: "success",
		SubjectID: "search-subject-" + suffix,
		CorrelationID: marker,
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ctx, audit.Event{
		OccurredAt: now.Add(time.Second),
		Action: "integration.audit-search",
		Outcome: "success",
		SubjectID: "details-only-subject-" + suffix,
		CorrelationID: "other-" + suffix,
		Details: map[string]any{"note": marker},
	}); err != nil {
		t.Fatal(err)
	}

	page, err := log.Read(ctx, audit.Query{
		Search: marker,
		From: now.Add(-time.Minute),
		To: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || len(page.Entries) != 1 {
		t.Fatalf("search has_more=%v entries=%d: %#v", page.HasMore, len(page.Entries), page.Entries)
	}
	if page.Entries[0].Event.CorrelationID != marker {
		t.Fatalf("unexpected metadata match: %#v", page.Entries[0].Event)
	}
}
