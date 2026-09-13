package intent

import (
	"encoding/json"
	"testing"
	"time"

	"control-center/internal/corecontracts"
)

func intentMetadata(id string) corecontracts.ObjectMetadata {
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	return corecontracts.ObjectMetadata{
		ObjectID:        id,
		ScopeID:         "site-a",
		OwnerScope:      "site-a",
		Generation:      1,
		ResourceVersion: "rv-1",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func validIntentRevision() IntentRevision {
	return IntentRevision{
		ObjectMetadata: intentMetadata("intent-revision:office-a:1"),
		IntentID:       "intent:office-a",
		Ordinal:        1,
		State:          StateConfirmed,
		Requirements: []Requirement{
			{
				ID:       "availability.node-failure",
				Class:    RequirementHardConstraint,
				Domain:   "availability",
				Key:      "survive-node-failure",
				Operator: "eq",
				Value:    json.RawMessage(`true`),
			},
			{
				ID:       "capacity.horizon",
				Class:    RequirementOptimizationObjective,
				Domain:   "capacity",
				Key:      "horizon-days",
				Operator: "maximize",
				Value:    json.RawMessage(`365`),
			},
		},
		ProviderPreferences: []ProviderPreference{
			{ProviderID: "proxmox-ve", Mode: ProviderPreferred},
		},
		ReusePolicy: ReusePolicy{
			Mode:              ReusePreferred,
			PreserveWorkloads: true,
			AllowMigration:    true,
			AllowReinstall:    false,
		},
	}
}

func TestIntentRevisionValidate(t *testing.T) {
	revision := validIntentRevision()
	if err := revision.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestHardRequirementNeedsStructuredValue(t *testing.T) {
	revision := validIntentRevision()
	revision.Requirements[0].Value = json.RawMessage(`not-json`)
	if err := revision.Validate(); err == nil {
		t.Fatal("invalid normalized requirement value must be rejected")
	}
}

func TestProviderPreferenceIsUnique(t *testing.T) {
	revision := validIntentRevision()
	revision.ProviderPreferences = append(revision.ProviderPreferences, ProviderPreference{
		ProviderID: "proxmox-ve",
		Mode:       ProviderRequired,
	})
	if err := revision.Validate(); err == nil {
		t.Fatal("duplicate provider preference must be rejected")
	}
}

func TestReuseRequiredRejectsReinstall(t *testing.T) {
	revision := validIntentRevision()
	revision.ReusePolicy.Mode = ReuseRequired
	revision.ReusePolicy.AllowReinstall = true
	if err := revision.Validate(); err == nil {
		t.Fatal("reuse_required must reject reinstall")
	}
}
