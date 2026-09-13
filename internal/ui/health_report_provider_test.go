package ui

import (
	"context"
	"errors"
	"testing"
	"time"
)

type healthSignalSourceFunc func(context.Context) (HealthSignalSnapshot, error)

func (f healthSignalSourceFunc) HealthSignals(ctx context.Context) (HealthSignalSnapshot, error) {
	return f(ctx)
}

func newHealthReportProviderForTest(t *testing.T, now time.Time, source HealthSignalSource) *HealthOperationalReportProvider {
	t.Helper()
	provider, err := NewHealthOperationalReportProvider(source, func() time.Time { return now }, time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestHealthOperationalReportProviderFailsClosedOnSourceError(t *testing.T) {
	now := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{
			Loaded: true,
			Signals: []HealthSignal{{
				ID: "partial", ResourceKind: "node", ResourceID: "node-1", CheckName: "ready",
				State: HealthStateHealthy, ObservedAt: now,
			}},
		}, errors.New("provider transport detail must not become report evidence")
	}))

	report, err := provider.OperationalReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.DataState != ReportDataUnavailable || report.OverallState != ReportHealthUnknown {
		t.Fatalf("report state = %s/%s, want unavailable/unknown", report.DataState, report.OverallState)
	}
	if len(report.Evidence) != 0 {
		t.Fatalf("source failure leaked partial evidence: %#v", report.Evidence)
	}
	if report.MutationAuthorized {
		t.Fatal("source failure unexpectedly authorized mutation")
	}
}

func TestHealthOperationalReportProviderPreservesFreshnessFailClosed(t *testing.T) {
	now := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: true, Signals: []HealthSignal{
			{
				ID: "current", ResourceKind: "node", ResourceID: "node-current", CheckName: "ready",
				State: HealthStateHealthy, ObservedAt: now.Add(-30 * time.Second), EvidenceRefs: []string{"evidence:current"},
			},
			{
				ID: "stale", ResourceKind: "node", ResourceID: "node-stale", CheckName: "ready",
				State: HealthStateHealthy, ObservedAt: now.Add(-2 * time.Minute), EvidenceRefs: []string{"evidence:stale"},
			},
			{
				ID: "expired", ResourceKind: "node", ResourceID: "node-expired", CheckName: "ready",
				State: HealthStateHealthy, ObservedAt: now.Add(-6 * time.Minute), EvidenceRefs: []string{"evidence:expired"},
			},
		}}, nil
	}))

	report, err := provider.OperationalReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.DataState != ReportDataLoaded {
		t.Fatalf("DataState = %s, want loaded", report.DataState)
	}
	if report.OverallState != ReportHealthDegraded {
		t.Fatalf("OverallState = %s, want degraded", report.OverallState)
	}

	states := map[string]ReportHealthState{}
	for _, evidence := range report.Evidence {
		states[evidence.ResourceID] = evidence.EffectiveState
	}
	if states["node-current"] != ReportHealthHealthy {
		t.Fatalf("current state = %s, want healthy", states["node-current"])
	}
	if states["node-stale"] != ReportHealthDegraded {
		t.Fatalf("stale state = %s, want degraded", states["node-stale"])
	}
	if states["node-expired"] != ReportHealthUnknown {
		t.Fatalf("expired state = %s, want unknown", states["node-expired"])
	}
}

func TestHealthOperationalReportProviderLoadedEmptyNeverMeansHealthy(t *testing.T) {
	now := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: true}, nil
	}))

	report, err := provider.OperationalReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.DataState != ReportDataLoaded || report.OverallState != ReportHealthUnknown {
		t.Fatalf("empty report state = %s/%s, want loaded/unknown", report.DataState, report.OverallState)
	}
	if len(report.Evidence) != 0 {
		t.Fatalf("empty source produced evidence: %#v", report.Evidence)
	}
}

func TestHealthOperationalReportProviderRejectsPartialUnavailableSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: false, Signals: []HealthSignal{{
			ID: "partial", ResourceKind: "node", ResourceID: "node-1", CheckName: "ready",
			State: HealthStateHealthy, ObservedAt: now,
		}}}, nil
	}))

	if _, err := provider.OperationalReport(context.Background()); err == nil {
		t.Fatal("partial unavailable snapshot accepted, want fail-closed error")
	}
}

func TestHealthOperationalReportProviderRejectsFutureEvidence(t *testing.T) {
	now := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: true, Signals: []HealthSignal{{
			ID: "future", ResourceKind: "node", ResourceID: "node-1", CheckName: "ready",
			State: HealthStateHealthy, ObservedAt: now.Add(time.Second),
		}}}, nil
	}))

	if _, err := provider.OperationalReport(context.Background()); err == nil {
		t.Fatal("future evidence accepted, want fail-closed error")
	}
}

func TestHealthOperationalReportProviderHonorsCancelledContext(t *testing.T) {
	now := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	called := false
	provider := newHealthReportProviderForTest(t, now, healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		called = true
		return HealthSignalSnapshot{Loaded: true}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := provider.OperationalReport(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("source invoked after context cancellation")
	}
}

func TestNewHealthOperationalReportProviderValidatesBoundary(t *testing.T) {
	source := healthSignalSourceFunc(func(context.Context) (HealthSignalSnapshot, error) {
		return HealthSignalSnapshot{Loaded: true}, nil
	})
	clock := func() time.Time { return time.Now().UTC() }

	if _, err := NewHealthOperationalReportProvider(nil, clock, time.Minute, 2*time.Minute); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := NewHealthOperationalReportProvider(source, nil, time.Minute, 2*time.Minute); err == nil {
		t.Fatal("nil clock accepted")
	}
	if _, err := NewHealthOperationalReportProvider(source, clock, 2*time.Minute, time.Minute); err == nil {
		t.Fatal("invalid freshness budgets accepted")
	}
}
