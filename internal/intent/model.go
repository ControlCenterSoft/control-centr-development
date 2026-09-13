// Package intent defines normalized infrastructure requirements without
// performing synthesis or infrastructure mutations.
package intent

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"control-center/internal/corecontracts"
)

var (
	ErrInvalidIntent         = errors.New("invalid infrastructure intent")
	ErrInvalidIntentRevision = errors.New("invalid intent revision")
	intentIDPattern          = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?$`)
)

type RevisionState string

const (
	StateDraft              RevisionState = "draft"
	StateNormalized         RevisionState = "normalized"
	StateNeedsClarification RevisionState = "needs_clarification"
	StateConfirmed          RevisionState = "confirmed"
	StateFeasibilityChecked RevisionState = "feasibility_checked"
	StateSatisfied          RevisionState = "satisfied"
	StatePartiallySatisfied RevisionState = "partially_satisfied"
	StateUnsatisfiable      RevisionState = "unsatisfiable"
	StateStale              RevisionState = "stale"
	StateSuperseded         RevisionState = "superseded"
)

func (s RevisionState) Valid() bool {
	switch s {
	case StateDraft, StateNormalized, StateNeedsClarification, StateConfirmed,
		StateFeasibilityChecked, StateSatisfied, StatePartiallySatisfied,
		StateUnsatisfiable, StateStale, StateSuperseded:
		return true
	default:
		return false
	}
}

type RequirementClass string

const (
	RequirementHardConstraint        RequirementClass = "hard_constraint"
	RequirementSoftPreference        RequirementClass = "soft_preference"
	RequirementOptimizationObjective RequirementClass = "optimization_objective"
)

func (c RequirementClass) Valid() bool {
	switch c {
	case RequirementHardConstraint, RequirementSoftPreference, RequirementOptimizationObjective:
		return true
	default:
		return false
	}
}

type ProviderPreferenceMode string

const (
	ProviderRequired    ProviderPreferenceMode = "required"
	ProviderPreferred   ProviderPreferenceMode = "preferred"
	ProviderAvoid       ProviderPreferenceMode = "avoid"
	ProviderUnspecified ProviderPreferenceMode = "unspecified"
)

func (m ProviderPreferenceMode) Valid() bool {
	switch m {
	case ProviderRequired, ProviderPreferred, ProviderAvoid, ProviderUnspecified:
		return true
	default:
		return false
	}
}

type ReuseMode string

const (
	ReusePreferred ReuseMode = "reuse_preferred"
	ReuseRequired  ReuseMode = "reuse_required"
	ReuseOptional  ReuseMode = "reuse_optional"
)

func (m ReuseMode) Valid() bool {
	switch m {
	case ReusePreferred, ReuseRequired, ReuseOptional:
		return true
	default:
		return false
	}
}

type Requirement struct {
	ID       string           `json:"id"`
	Class    RequirementClass `json:"class"`
	Domain   string           `json:"domain"`
	Key      string           `json:"key"`
	Operator string           `json:"operator"`
	Value    json.RawMessage  `json:"value"`
}

func (r Requirement) Validate() error {
	if err := validateCanonicalID("requirement id", r.ID); err != nil {
		return err
	}
	if !r.Class.Valid() {
		return fmt.Errorf("requirement class %q is unsupported", r.Class)
	}
	if err := validateCanonicalID("requirement domain", r.Domain); err != nil {
		return err
	}
	if err := validateCanonicalID("requirement key", r.Key); err != nil {
		return err
	}
	if strings.TrimSpace(r.Operator) == "" || strings.TrimSpace(r.Operator) != r.Operator {
		return errors.New("requirement operator is required and must not contain surrounding whitespace")
	}
	if len(r.Value) == 0 || !json.Valid(r.Value) {
		return errors.New("requirement value must be valid JSON")
	}
	return nil
}

type ProviderPreference struct {
	ProviderID string                 `json:"provider_id"`
	Mode       ProviderPreferenceMode `json:"mode"`
}

func (p ProviderPreference) Validate() error {
	if err := validateCanonicalID("provider_id", p.ProviderID); err != nil {
		return err
	}
	if !p.Mode.Valid() {
		return fmt.Errorf("provider preference mode %q is unsupported", p.Mode)
	}
	return nil
}

type ReusePolicy struct {
	Mode              ReuseMode `json:"mode"`
	PreserveWorkloads bool      `json:"preserve_workloads"`
	AllowMigration    bool      `json:"allow_migration"`
	AllowReinstall    bool      `json:"allow_reinstall"`
}

func (p ReusePolicy) Validate() error {
	if !p.Mode.Valid() {
		return fmt.Errorf("reuse mode %q is unsupported", p.Mode)
	}
	if p.Mode == ReuseRequired && p.AllowReinstall {
		return errors.New("reuse_required cannot allow reinstall")
	}
	return nil
}

type InfrastructureIntent struct {
	corecontracts.ObjectMetadata
	DisplayName       string `json:"display_name"`
	CurrentRevisionID string `json:"current_revision_id,omitempty"`
}

func (i InfrastructureIntent) Validate() error {
	if err := i.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidIntent, err)
	}
	if strings.TrimSpace(i.DisplayName) == "" || len(i.DisplayName) > 160 {
		return fmt.Errorf("%w: display_name must be 1..160 characters", ErrInvalidIntent)
	}
	if i.CurrentRevisionID != "" && strings.TrimSpace(i.CurrentRevisionID) != i.CurrentRevisionID {
		return fmt.Errorf("%w: current_revision_id has surrounding whitespace", ErrInvalidIntent)
	}
	return nil
}

type IntentRevision struct {
	corecontracts.ObjectMetadata
	IntentID            string               `json:"intent_id"`
	Ordinal             uint64               `json:"ordinal"`
	State               RevisionState        `json:"state"`
	Requirements        []Requirement        `json:"requirements"`
	ProviderPreferences []ProviderPreference `json:"provider_preferences,omitempty"`
	ReusePolicy         ReusePolicy          `json:"reuse_policy"`
}

func (r IntentRevision) Validate() error {
	if err := r.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidIntentRevision, err)
	}
	if strings.TrimSpace(r.IntentID) == "" || strings.TrimSpace(r.IntentID) != r.IntentID {
		return fmt.Errorf("%w: intent_id is required", ErrInvalidIntentRevision)
	}
	if r.Ordinal == 0 {
		return fmt.Errorf("%w: ordinal must be positive", ErrInvalidIntentRevision)
	}
	if !r.State.Valid() {
		return fmt.Errorf("%w: state %q is unsupported", ErrInvalidIntentRevision, r.State)
	}
	if len(r.Requirements) == 0 {
		return fmt.Errorf("%w: at least one requirement is required", ErrInvalidIntentRevision)
	}
	seenRequirements := map[string]struct{}{}
	for _, requirement := range r.Requirements {
		if err := requirement.Validate(); err != nil {
			return fmt.Errorf("%w: requirement %q: %v", ErrInvalidIntentRevision, requirement.ID, err)
		}
		if _, duplicate := seenRequirements[requirement.ID]; duplicate {
			return fmt.Errorf("%w: duplicate requirement %q", ErrInvalidIntentRevision, requirement.ID)
		}
		seenRequirements[requirement.ID] = struct{}{}
	}
	seenProviders := map[string]struct{}{}
	for _, preference := range r.ProviderPreferences {
		if err := preference.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidIntentRevision, err)
		}
		if _, duplicate := seenProviders[preference.ProviderID]; duplicate {
			return fmt.Errorf("%w: duplicate provider preference %q", ErrInvalidIntentRevision, preference.ProviderID)
		}
		seenProviders[preference.ProviderID] = struct{}{}
	}
	if err := r.ReusePolicy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidIntentRevision, err)
	}
	return nil
}

func validateCanonicalID(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || !intentIDPattern.MatchString(value) {
		return fmt.Errorf("%s must be a canonical lower-case identifier", field)
	}
	return nil
}
