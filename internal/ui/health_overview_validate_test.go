package ui

import (
	"testing"
	"time"
)

func TestValidateHealthOverviewAcceptsCanonicalBuilderOutput(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded:       true,
		Now:          now,
		StaleAfter:   5 * time.Minute,
		ExpiredAfter: 15 * time.Minute,
		Signals: []HealthSignal{
			{
				ID: "db-primary", ResourceKind: "database", ResourceID: "db-a",
				CheckName: "readiness", State: HealthStateHealthy,
				ObservedAt: now.Add(-time.Minute), EvidenceRefs: []string{"evidence:db-a"},
			},
			{
				ID: "node-storage", ResourceKind: "node", ResourceID: "node-a",
				CheckName: "storage", State: HealthStateDegraded,
				ObservedAt: now.Add(-2 * time.Minute), RunbookRef: "runbook:storage",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	if err := ValidateHealthOverview(view); err != nil {
		t.Fatalf("ValidateHealthOverview() error = %v", err)
	}
}

func TestValidateHealthOverviewRejectsTamperedEffectiveState(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded: true, Now: now, StaleAfter: time.Minute, ExpiredAfter: 5 * time.Minute,
		Signals: []HealthSignal{{
			ID: "node-a", ResourceKind: "node", ResourceID: "node-a",
			CheckName: "ready", State: HealthStateHealthy, ObservedAt: now.Add(-2 * time.Minute),
		}},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	view.Signals[0].EffectiveState = HealthStateHealthy
	if err := ValidateHealthOverview(view); err == nil {
		t.Fatal("ValidateHealthOverview() error = nil, want tampered state rejection")
	}
}

func TestValidateHealthOverviewRejectsNonCanonicalSignalOrder(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	view, err := BuildHealthOverview(HealthOverviewInput{
		Loaded: true, Now: now, StaleAfter: 5 * time.Minute, ExpiredAfter: 15 * time.Minute,
		Signals: []HealthSignal{
			{ID: "healthy", ResourceKind: "node", ResourceID: "node-a", CheckName: "ready", State: HealthStateHealthy, ObservedAt: now},
			{ID: "failed", ResourceKind: "node", ResourceID: "node-b", CheckName: "ready", State: HealthStateUnhealthy, ObservedAt: now},
		},
	})
	if err != nil {
		t.Fatalf("BuildHealthOverview() error = %v", err)
	}
	view.Signals[0], view.Signals[1] = view.Signals[1], view.Signals[0]
	if err := ValidateHealthOverview(view); err == nil {
		t.Fatal("ValidateHealthOverview() error = nil, want non-canonical ordering rejection")
	}
}

func TestValidateHealthOverviewRejectsContradictoryUnavailableEvidence(t *testing.T) {
	view := HealthOverview{
		Schema:             HealthOverviewSchemaV1,
		DataState:          HealthDataUnavailable,
		OverallState:       HealthStateHealthy,
		Resources:          []ResourceHealthView{},
		Signals:            []HealthSignalView{},
		MutationAuthorized: false,
	}
	if err := ValidateHealthOverview(view); err == nil {
		t.Fatal("ValidateHealthOverview() error = nil, want contradictory unavailable rejection")
	}
}

func TestValidateHealthOverviewRejectsMutationAuthority(t *testing.T) {
	view := HealthOverview{
		Schema:             HealthOverviewSchemaV1,
		DataState:          HealthDataUnavailable,
		OverallState:       HealthStateUnknown,
		Resources:          []ResourceHealthView{},
		Signals:            []HealthSignalView{},
		MutationAuthorized: true,
	}
	if err := ValidateHealthOverview(view); err == nil {
		t.Fatal("ValidateHealthOverview() error = nil, want mutation authority rejection")
	}
}
