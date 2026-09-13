// Package synthesis defines side-effect-free candidate synthesis, decision and
// expansion-assessment contracts. Selecting a candidate creates proposed state;
// it never executes infrastructure mutations directly.
package synthesis

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"control-center/internal/corecontracts"
)

var (
	ErrInvalidAssessment          = errors.New("invalid synthesis assessment")
	ErrInvalidCandidate           = errors.New("invalid synthesis candidate")
	ErrInvalidDecision            = errors.New("invalid synthesis decision")
	ErrInvalidExpansionAssessment = errors.New("invalid expansion assessment")
	digestPattern                 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type AssessmentState string

const (
	AssessmentPending   AssessmentState = "pending"
	AssessmentRunning   AssessmentState = "running"
	AssessmentCompleted AssessmentState = "completed"
	AssessmentStale     AssessmentState = "stale"
	AssessmentFailed    AssessmentState = "failed"
)

func (s AssessmentState) Valid() bool {
	switch s {
	case AssessmentPending, AssessmentRunning, AssessmentCompleted, AssessmentStale, AssessmentFailed:
		return true
	default:
		return false
	}
}

type CandidateStatus string

const (
	CandidateValid            CandidateStatus = "valid"
	CandidateValidWithWarning CandidateStatus = "valid_with_warning"
	CandidateBlocked          CandidateStatus = "blocked"
)

func (s CandidateStatus) Valid() bool {
	switch s {
	case CandidateValid, CandidateValidWithWarning, CandidateBlocked:
		return true
	default:
		return false
	}
}

type ConstraintStatus string

const (
	ConstraintPass    ConstraintStatus = "pass"
	ConstraintWarning ConstraintStatus = "warning"
	ConstraintBlocked ConstraintStatus = "blocked"
	ConstraintUnknown ConstraintStatus = "unknown"
)

func (s ConstraintStatus) Valid() bool {
	switch s {
	case ConstraintPass, ConstraintWarning, ConstraintBlocked, ConstraintUnknown:
		return true
	default:
		return false
	}
}

type SynthesisAssessment struct {
	corecontracts.ObjectMetadata
	IntentRevisionID      string          `json:"intent_revision_id"`
	BlueprintRevisionID   string          `json:"blueprint_revision_id"`
	InputSnapshotDigest   string          `json:"input_snapshot_digest"`
	ProviderCatalogDigest string          `json:"provider_catalog_digest"`
	PolicyRevision        string          `json:"policy_revision"`
	State                 AssessmentState `json:"state"`
	EvaluatedAt           time.Time       `json:"evaluated_at,omitempty"`
}

func (a SynthesisAssessment) Validate() error {
	if err := a.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAssessment, err)
	}
	if strings.TrimSpace(a.IntentRevisionID) == "" || strings.TrimSpace(a.BlueprintRevisionID) == "" {
		return fmt.Errorf("%w: intent_revision_id and blueprint_revision_id are required", ErrInvalidAssessment)
	}
	if !digestPattern.MatchString(a.InputSnapshotDigest) || !digestPattern.MatchString(a.ProviderCatalogDigest) {
		return fmt.Errorf("%w: input/provider catalog digests must be sha256 identities", ErrInvalidAssessment)
	}
	if strings.TrimSpace(a.PolicyRevision) == "" {
		return fmt.Errorf("%w: policy_revision is required", ErrInvalidAssessment)
	}
	if !a.State.Valid() {
		return fmt.Errorf("%w: state %q is unsupported", ErrInvalidAssessment, a.State)
	}
	if a.State == AssessmentCompleted && a.EvaluatedAt.IsZero() {
		return fmt.Errorf("%w: completed assessment requires evaluated_at", ErrInvalidAssessment)
	}
	return nil
}

type ConstraintResult struct {
	RequirementID string           `json:"requirement_id"`
	Status        ConstraintStatus `json:"status"`
	Reason        string           `json:"reason"`
}

func (r ConstraintResult) Validate() error {
	if strings.TrimSpace(r.RequirementID) == "" || strings.TrimSpace(r.RequirementID) != r.RequirementID {
		return errors.New("requirement_id is required")
	}
	if !r.Status.Valid() {
		return fmt.Errorf("constraint status %q is unsupported", r.Status)
	}
	if strings.TrimSpace(r.Reason) == "" {
		return errors.New("constraint reason is required")
	}
	return nil
}

type SolutionCandidate struct {
	corecontracts.ObjectMetadata
	AssessmentID        string             `json:"assessment_id"`
	Status              CandidateStatus    `json:"status"`
	TopologyDigest      string             `json:"topology_digest"`
	ProviderBindingRefs []string           `json:"provider_binding_refs,omitempty"`
	Score               *float64           `json:"score,omitempty"`
	ConstraintResults   []ConstraintResult `json:"constraint_results"`
	Tradeoffs           []string           `json:"tradeoffs,omitempty"`
}

func (c SolutionCandidate) Validate() error {
	if err := c.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCandidate, err)
	}
	if strings.TrimSpace(c.AssessmentID) == "" {
		return fmt.Errorf("%w: assessment_id is required", ErrInvalidCandidate)
	}
	if !c.Status.Valid() {
		return fmt.Errorf("%w: status %q is unsupported", ErrInvalidCandidate, c.Status)
	}
	if !digestPattern.MatchString(c.TopologyDigest) {
		return fmt.Errorf("%w: topology_digest must be sha256 identity", ErrInvalidCandidate)
	}
	if c.Score != nil && (*c.Score < 0 || *c.Score > 1) {
		return fmt.Errorf("%w: score must be in range 0..1", ErrInvalidCandidate)
	}
	seenConstraints := map[string]struct{}{}
	blocked := false
	warning := false
	for _, result := range c.ConstraintResults {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidCandidate, err)
		}
		if _, duplicate := seenConstraints[result.RequirementID]; duplicate {
			return fmt.Errorf("%w: duplicate constraint result %q", ErrInvalidCandidate, result.RequirementID)
		}
		seenConstraints[result.RequirementID] = struct{}{}
		switch result.Status {
		case ConstraintBlocked, ConstraintUnknown:
			blocked = true
		case ConstraintWarning:
			warning = true
		}
	}
	if blocked && c.Status != CandidateBlocked {
		return fmt.Errorf("%w: blocked/unknown constraint requires blocked candidate", ErrInvalidCandidate)
	}
	if !blocked && warning && c.Status == CandidateValid {
		return fmt.Errorf("%w: warning constraint requires valid_with_warning candidate", ErrInvalidCandidate)
	}
	if err := validateUniqueStrings("provider binding reference", c.ProviderBindingRefs); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCandidate, err)
	}
	return nil
}

type SynthesisDecision struct {
	corecontracts.ObjectMetadata
	AssessmentID                string    `json:"assessment_id"`
	SelectedCandidateID         string    `json:"selected_candidate_id"`
	Rationale                   string    `json:"rationale"`
	ResultingSolutionRevisionID string    `json:"resulting_solution_revision_id,omitempty"`
	DecidedAt                   time.Time `json:"decided_at"`
}

func (d SynthesisDecision) Validate() error {
	if err := d.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDecision, err)
	}
	if strings.TrimSpace(d.AssessmentID) == "" || strings.TrimSpace(d.SelectedCandidateID) == "" {
		return fmt.Errorf("%w: assessment_id and selected_candidate_id are required", ErrInvalidDecision)
	}
	if strings.TrimSpace(d.Rationale) == "" {
		return fmt.Errorf("%w: rationale is required", ErrInvalidDecision)
	}
	if d.DecidedAt.IsZero() {
		return fmt.Errorf("%w: decided_at is required", ErrInvalidDecision)
	}
	return nil
}

type ExpansionStrategy string

const (
	ExpansionScaleUp      ExpansionStrategy = "scale_up"
	ExpansionScaleOut     ExpansionStrategy = "scale_out"
	ExpansionNewInstance  ExpansionStrategy = "new_instance"
	ExpansionNewCluster   ExpansionStrategy = "new_cluster"
	ExpansionSplitMigrate ExpansionStrategy = "split_migrate"
	ExpansionReplace      ExpansionStrategy = "replace"
	ExpansionNoChange     ExpansionStrategy = "no_change"
)

func (s ExpansionStrategy) Valid() bool {
	switch s {
	case ExpansionScaleUp, ExpansionScaleOut, ExpansionNewInstance, ExpansionNewCluster,
		ExpansionSplitMigrate, ExpansionReplace, ExpansionNoChange:
		return true
	default:
		return false
	}
}

type ExpansionOption struct {
	Strategy   ExpansionStrategy `json:"strategy"`
	Status     CandidateStatus   `json:"status"`
	Reason     string            `json:"reason"`
	PlanDigest string            `json:"plan_digest,omitempty"`
}

func (o ExpansionOption) Validate() error {
	if !o.Strategy.Valid() || !o.Status.Valid() {
		return errors.New("expansion strategy/status is unsupported")
	}
	if strings.TrimSpace(o.Reason) == "" {
		return errors.New("expansion reason is required")
	}
	if o.PlanDigest != "" && !digestPattern.MatchString(o.PlanDigest) {
		return errors.New("expansion plan_digest must be sha256 identity")
	}
	return nil
}

type ExpansionAssessment struct {
	corecontracts.ObjectMetadata
	SolutionRevisionID string            `json:"solution_revision_id"`
	InputSnapshotDigest string            `json:"input_snapshot_digest"`
	State              AssessmentState   `json:"state"`
	Options            []ExpansionOption `json:"options"`
	EvaluatedAt        time.Time         `json:"evaluated_at,omitempty"`
}

func (a ExpansionAssessment) Validate() error {
	if err := a.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExpansionAssessment, err)
	}
	if strings.TrimSpace(a.SolutionRevisionID) == "" {
		return fmt.Errorf("%w: solution_revision_id is required", ErrInvalidExpansionAssessment)
	}
	if !digestPattern.MatchString(a.InputSnapshotDigest) {
		return fmt.Errorf("%w: input_snapshot_digest must be sha256 identity", ErrInvalidExpansionAssessment)
	}
	if !a.State.Valid() {
		return fmt.Errorf("%w: state %q is unsupported", ErrInvalidExpansionAssessment, a.State)
	}
	if len(a.Options) == 0 {
		return fmt.Errorf("%w: at least one expansion option is required", ErrInvalidExpansionAssessment)
	}
	seen := map[ExpansionStrategy]struct{}{}
	for _, option := range a.Options {
		if err := option.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidExpansionAssessment, err)
		}
		if _, duplicate := seen[option.Strategy]; duplicate {
			return fmt.Errorf("%w: duplicate expansion strategy %q", ErrInvalidExpansionAssessment, option.Strategy)
		}
		seen[option.Strategy] = struct{}{}
	}
	if a.State == AssessmentCompleted && a.EvaluatedAt.IsZero() {
		return fmt.Errorf("%w: completed assessment requires evaluated_at", ErrInvalidExpansionAssessment)
	}
	return nil
}

func validateUniqueStrings(field string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%s must be non-empty and trimmed", field)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate %s %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
