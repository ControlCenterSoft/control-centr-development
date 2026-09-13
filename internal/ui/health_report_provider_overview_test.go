package ui

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthOperationalReportProviderExposesSameCanonicalOverview(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 30, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: true, Signals: []HealthSignal{
			{
				ID: "node-current", ResourceKind: "node", ResourceID: "node-current", CheckName: "ready",
				State: HealthStateHealthy, ObservedAt: now.Add(-30 * time.Second),
			},
			{
				ID: "node-stale", ResourceKind: "node", ResourceID: "node-stale", CheckName: "ready",
				State: HealthStateHealthy, ObservedAt: now.Add(-2 * time.Minute),
			},
		}}, nil
	}))

	overview, err := provider.HealthOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHealthOverview(overview); err != nil {
		t.Fatalf("ValidateHealthOverview() error = %v", err)
	}
	if overview.DataState != HealthDataLoaded || overview.OverallState != HealthStateDegraded {
		t.Fatalf("overview state = %s/%s, want loaded/degraded", overview.DataState, overview.OverallState)
	}

	report, err := provider.OperationalReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.DataState != ReportDataLoaded || report.OverallState != ReportHealthDegraded {
		t.Fatalf("report state = %s/%s, want loaded/degraded", report.DataState, report.OverallState)
	}
}

func TestHealthOperationalReportProviderHealthOverviewFailsClosedOnSourceError(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 30, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: true, Signals: []HealthSignal{{
			ID: "partial", ResourceKind: "node", ResourceID: "node-a", CheckName: "ready",
			State: HealthStateHealthy, ObservedAt: now,
		}}}, errors.New("private provider failure")
	}))

	overview, err := provider.HealthOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overview.DataState != HealthDataUnavailable || overview.OverallState != HealthStateUnknown {
		t.Fatalf("overview state = %s/%s, want unavailable/unknown", overview.DataState, overview.OverallState)
	}
	if len(overview.Signals) != 0 || len(overview.Resources) != 0 {
		t.Fatalf("source failure leaked partial health evidence: %#v", overview)
	}
	if overview.MutationAuthorized {
		t.Fatal("source failure unexpectedly authorized mutation")
	}
}
