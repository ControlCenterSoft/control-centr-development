package ui

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
)

// ValidateHealthOverview rejects malformed, contradictory or non-canonical
// Health projections at a provider/transport boundary. It does not recalculate
// freshness from wall-clock time; the authoritative provider must build the
// view through BuildHealthOverview with its trusted clock and freshness policy.
// This validator rechecks the security-relevant projection invariants, exact
// ordering and derived resource/overall state before the view is exposed.
func ValidateHealthOverview(view HealthOverview) error {
	if view.Schema != HealthOverviewSchemaV1 {
		return fmt.Errorf("unsupported health overview schema %q", view.Schema)
	}
	if view.MutationAuthorized {
		return errors.New("health overview unexpectedly carries mutation authority")
	}

	provided := normalizeHealthOverviewRepresentation(view)
	switch provided.DataState {
	case HealthDataUnavailable:
		expected := HealthOverview{
			Schema:             HealthOverviewSchemaV1,
			DataState:          HealthDataUnavailable,
			OverallState:       HealthStateUnknown,
			Resources:          []ResourceHealthView{},
			Signals:            []HealthSignalView{},
			MutationAuthorized: false,
		}
		if !reflect.DeepEqual(provided, expected) {
			return errors.New("unavailable health overview contains contradictory evidence")
		}
		return nil
	case HealthDataLoaded:
	default:
		return fmt.Errorf("unsupported health data state %q", provided.DataState)
	}

	canonicalSignals := make([]HealthSignalView, 0, len(provided.Signals))
	seen := make(map[string]struct{}, len(provided.Signals))
	for _, signal := range provided.Signals {
		normalized, err := validateHealthSignalView(signal)
		if err != nil {
			return err
		}
		if _, exists := seen[normalized.ID]; exists {
			return fmt.Errorf("duplicate health signal id %q", normalized.ID)
		}
		seen[normalized.ID] = struct{}{}
		canonicalSignals = append(canonicalSignals, normalized)
	}
	sortHealthSignalViewsCanonical(canonicalSignals)
	if !reflect.DeepEqual(provided.Signals, canonicalSignals) {
		return errors.New("health overview signals are not canonical")
	}

	canonicalResources := aggregateResourceHealth(canonicalSignals)
	canonicalResources = normalizeResourceHealthViews(canonicalResources)
	if !reflect.DeepEqual(provided.Resources, canonicalResources) {
		return errors.New("health overview resources do not match signal evidence")
	}

	expectedOverall := HealthStateUnknown
	if len(canonicalResources) > 0 {
		expectedOverall = canonicalResources[0].State
		for _, resource := range canonicalResources[1:] {
			if healthStateRank(resource.State) > healthStateRank(expectedOverall) {
				expectedOverall = resource.State
			}
		}
	}
	if provided.OverallState != expectedOverall {
		return errors.New("health overview overall state does not match resource evidence")
	}
	return nil
}

func validateHealthSignalView(signal HealthSignalView) (HealthSignalView, error) {
	if err := validateHealthBoundedText("signal id", signal.ID, maxSignalIDLength); err != nil {
		return HealthSignalView{}, err
	}
	if err := validateHealthBoundedText("resource kind", signal.ResourceKind, maxResourceKindLength); err != nil {
		return HealthSignalView{}, fmt.Errorf("signal %q: %w", signal.ID, err)
	}
	if err := validateHealthBoundedText("resource id", signal.ResourceID, maxResourceIDLength); err != nil {
		return HealthSignalView{}, fmt.Errorf("signal %q: %w", signal.ID, err)
	}
	if err := validateHealthBoundedText("check name", signal.CheckName, maxCheckNameLength); err != nil {
		return HealthSignalView{}, fmt.Errorf("signal %q: %w", signal.ID, err)
	}
	if signal.RunbookRef != "" {
		if err := validateHealthBoundedText("runbook ref", signal.RunbookRef, maxRunbookRefLength); err != nil {
			return HealthSignalView{}, fmt.Errorf("signal %q: %w", signal.ID, err)
		}
	}
	if signal.ObservedAt.IsZero() {
		return HealthSignalView{}, fmt.Errorf("signal %q observed_at is required", signal.ID)
	}
	if healthStateRank(signal.ObservedState) < 0 {
		return HealthSignalView{}, fmt.Errorf("signal %q has unsupported observed state %q", signal.ID, signal.ObservedState)
	}
	if healthStateRank(signal.EffectiveState) < 0 {
		return HealthSignalView{}, fmt.Errorf("signal %q has unsupported effective state %q", signal.ID, signal.EffectiveState)
	}
	if healthFreshnessRank(signal.Freshness) < 0 {
		return HealthSignalView{}, fmt.Errorf("signal %q has unsupported freshness %q", signal.ID, signal.Freshness)
	}
	if expected := effectiveHealthState(signal.ObservedState, signal.Freshness); signal.EffectiveState != expected {
		return HealthSignalView{}, fmt.Errorf("signal %q has inconsistent effective state", signal.ID)
	}

	refs, err := normalizeHealthEvidenceRefs(signal.EvidenceRefs)
	if err != nil {
		return HealthSignalView{}, fmt.Errorf("signal %q: %w", signal.ID, err)
	}
	signal.ObservedAt = signal.ObservedAt.UTC()
	signal.EvidenceRefs = refs
	return signal, nil
}

func sortHealthSignalViewsCanonical(signals []HealthSignalView) {
	sort.Slice(signals, func(i, j int) bool {
		if healthStateRank(signals[i].EffectiveState) != healthStateRank(signals[j].EffectiveState) {
			return healthStateRank(signals[i].EffectiveState) > healthStateRank(signals[j].EffectiveState)
		}
		if healthFreshnessRank(signals[i].Freshness) != healthFreshnessRank(signals[j].Freshness) {
			return healthFreshnessRank(signals[i].Freshness) > healthFreshnessRank(signals[j].Freshness)
		}
		if signals[i].ResourceKind != signals[j].ResourceKind {
			return signals[i].ResourceKind < signals[j].ResourceKind
		}
		if signals[i].ResourceID != signals[j].ResourceID {
			return signals[i].ResourceID < signals[j].ResourceID
		}
		if signals[i].CheckName != signals[j].CheckName {
			return signals[i].CheckName < signals[j].CheckName
		}
		return signals[i].ID < signals[j].ID
	})
}

func normalizeHealthOverviewRepresentation(view HealthOverview) HealthOverview {
	view.Signals = append([]HealthSignalView(nil), view.Signals...)
	if view.Signals == nil {
		view.Signals = []HealthSignalView{}
	}
	for index := range view.Signals {
		if len(view.Signals[index].EvidenceRefs) == 0 {
			view.Signals[index].EvidenceRefs = []string{}
		}
	}
	view.Resources = normalizeResourceHealthViews(view.Resources)
	return view
}

func normalizeResourceHealthViews(resources []ResourceHealthView) []ResourceHealthView {
	resources = append([]ResourceHealthView(nil), resources...)
	if resources == nil {
		resources = []ResourceHealthView{}
	}
	for index := range resources {
		resources[index].SignalIDs = append([]string(nil), resources[index].SignalIDs...)
		if resources[index].SignalIDs == nil {
			resources[index].SignalIDs = []string{}
		}
	}
	return resources
}
