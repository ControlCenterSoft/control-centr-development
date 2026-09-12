package audit

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNormalizeQueryRequiresBoundedMetadataSearch(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)

	normalized, err := NormalizeQuery(Query{Search: "  CORRELATION-123  ", From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Search != "CORRELATION-123" {
		t.Fatalf("search=%q", normalized.Search)
	}
	if normalized.From.Location() != time.UTC || normalized.To.Location() != time.UTC {
		t.Fatalf("time bounds were not canonicalized to UTC: from=%s to=%s", normalized.From, normalized.To)
	}

	for name, query := range map[string]Query{
		"missing bounds": {Search: "login"},
		"short search":  {Search: "ab", From: from, To: to},
		"long search":   {Search: strings.Repeat("x", MaxSearchRunes+1), From: from, To: to},
		"reversed":      {Search: "login", From: to, To: from},
		"wide window":   {Search: "login", From: from, To: from.Add(MaxSearchWindow + time.Second)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeQuery(query); err == nil {
				t.Fatalf("NormalizeQuery accepted invalid search query: %#v", query)
			}
		})
	}
}

func TestMemoryLogSearchesMetadataOnlyWithinExplicitWindow(t *testing.T) {
	log := NewMemoryLog()
	base := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	if err := log.Append(context.Background(), Event{
		OccurredAt: base,
		Action: "system.check",
		Outcome: "success",
		CorrelationID: "deploy-ABC-123",
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), Event{
		OccurredAt: base.Add(time.Minute),
		Action: "system.check",
		Outcome: "success",
		CorrelationID: "other-correlation",
		Details: map[string]any{"note": "deploy-ABC-123"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), Event{
		OccurredAt: base.Add(48 * time.Hour),
		Action: "deploy-abc-123.outside-window",
		Outcome: "success",
	}); err != nil {
		t.Fatal(err)
	}

	page, err := log.Read(context.Background(), Query{
		Search: "DEPLOY-abc-123",
		From: base.Add(-time.Minute),
		To: base.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || len(page.Entries) != 1 {
		t.Fatalf("metadata search returned has_more=%v entries=%d: %#v", page.HasMore, len(page.Entries), page.Entries)
	}
	if page.Entries[0].Event.CorrelationID != "deploy-ABC-123" {
		t.Fatalf("unexpected metadata search match: %#v", page.Entries[0].Event)
	}

	detailsOnly, err := log.Read(context.Background(), Query{
		Search: "note",
		From: base.Add(-time.Minute),
		To: base.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detailsOnly.Entries) != 0 {
		t.Fatalf("audit search inspected structured details: %#v", detailsOnly.Entries)
	}
}
