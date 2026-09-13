// Package solutions defines the side-effect-free InfrastructureSolution
// contracts for milestone 0.43. Orchestration, persistence and execution are
// separate adapters around these types.
package solutions

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"control-center/internal/corecontracts"
)

var (
	ErrInvalidSolution   = errors.New("invalid infrastructure solution")
	ErrInvalidRevision   = errors.New("invalid solution revision")
	ErrInvalidTopology   = errors.New("invalid solution topology")
	ErrInvalidValidation = errors.New("invalid architecture validation result")
	ErrInvalidPlan       = errors.New("invalid deployment plan")
	sha256Pattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type LifecycleState string

const (
	StateDraft            LifecycleState = "draft"
	StateDesigned         LifecycleState = "designed"
	StateValidated        LifecycleState = "validated"
	StatePlanned          LifecycleState = "planned"
	StateDeploying        LifecycleState = "deploying"
	StateAdopting         LifecycleState = "adopting"
	StateVerifying        LifecycleState = "verifying"
	StateReady            LifecycleState = "ready"
	StateOperating        LifecycleState = "operating"
	StateScaling          LifecycleState = "scaling"
	StateUpdating         LifecycleState = "updating"
	StateReconfiguring    LifecycleState = "reconfiguring"
	StateMaintaining      LifecycleState = "maintaining"
	StateMigrating        LifecycleState = "migrating"
	StateRecovering       LifecycleState = "recovering"
	StateShrinking        LifecycleState = "shrinking"
	StateDecommissioning  LifecycleState = "decommissioning"
	StateDegraded         LifecycleState = "degraded"
	StateFailed           LifecycleState = "failed"
	StateUnknown          LifecycleState = "unknown"
	StateRecoveryRequired LifecycleState = "recovery_required"
)

func (s LifecycleState) Valid() bool {
	switch s {
	case StateDraft, StateDesigned, StateValidated, StatePlanned, StateDeploying,
		StateAdopting, StateVerifying, StateReady, StateOperating, StateScaling,
		StateUpdating, StateReconfiguring, StateMaintaining, StateMigrating,
		StateRecovering, StateShrinking, StateDecommissioning, StateDegraded,
		StateFailed, StateUnknown, StateRecoveryRequired:
		return true
	default:
		return false
	}
}

type InfrastructureSolution struct {
	corecontracts.ObjectMetadata
	DisplayName       string         `json:"display_name"`
	State             LifecycleState `json:"state"`
	CurrentRevisionID string         `json:"current_revision_id,omitempty"`
}

func (s InfrastructureSolution) Validate() error {
	if err := s.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSolution, err)
	}
	if strings.TrimSpace(s.DisplayName) == "" || len(s.DisplayName) > 160 {
		return fmt.Errorf("%w: display_name must be 1..160 characters", ErrInvalidSolution)
	}
	if !s.State.Valid() {
		return fmt.Errorf("%w: unsupported state %q", ErrInvalidSolution, s.State)
	}
	if s.CurrentRevisionID != "" && strings.TrimSpace(s.CurrentRevisionID) != s.CurrentRevisionID {
		return fmt.Errorf("%w: current_revision_id has surrounding whitespace", ErrInvalidSolution)
	}
	return nil
}

type RevisionState string

const (
	RevisionProposed   RevisionState = "proposed"
	RevisionValidated  RevisionState = "validated"
	RevisionPlanned    RevisionState = "planned"
	RevisionActive     RevisionState = "active"
	RevisionSuperseded RevisionState = "superseded"
	RevisionStale      RevisionState = "stale"
)

func (s RevisionState) Valid() bool {
	switch s {
	case RevisionProposed, RevisionValidated, RevisionPlanned, RevisionActive, RevisionSuperseded, RevisionStale:
		return true
	default:
		return false
	}
}

type SolutionRevision struct {
	corecontracts.ObjectMetadata
	SolutionID          string        `json:"solution_id"`
	Ordinal             uint64        `json:"ordinal"`
	State               RevisionState `json:"state"`
	IntentRevisionID    string        `json:"intent_revision_id,omitempty"`
	BlueprintRevisionID string        `json:"blueprint_revision_id,omitempty"`
	TopologyDigest      string        `json:"topology_digest"`
}

func (r SolutionRevision) Validate() error {
	if err := r.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRevision, err)
	}
	if strings.TrimSpace(r.SolutionID) == "" || strings.TrimSpace(r.SolutionID) != r.SolutionID {
		return fmt.Errorf("%w: solution_id is required", ErrInvalidRevision)
	}
	if r.Ordinal == 0 {
		return fmt.Errorf("%w: ordinal must be positive", ErrInvalidRevision)
	}
	if !r.State.Valid() {
		return fmt.Errorf("%w: unsupported state %q", ErrInvalidRevision, r.State)
	}
	if !sha256Pattern.MatchString(r.TopologyDigest) {
		return fmt.Errorf("%w: topology_digest must be sha256:<64 lowercase hex>", ErrInvalidRevision)
	}
	return nil
}

type TopologyNode struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	ProviderBinding string `json:"provider_binding,omitempty"`
	FailureDomain   string `json:"failure_domain,omitempty"`
}

type TopologyEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

type Topology struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

func (t Topology) Validate() error {
	if len(t.Nodes) == 0 {
		return fmt.Errorf("%w: at least one node is required", ErrInvalidTopology)
	}
	nodes := make(map[string]struct{}, len(t.Nodes))
	for _, node := range t.Nodes {
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Kind) == "" {
			return fmt.Errorf("%w: node id and kind are required", ErrInvalidTopology)
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			return fmt.Errorf("%w: duplicate node %q", ErrInvalidTopology, node.ID)
		}
		nodes[node.ID] = struct{}{}
	}
	seenEdges := map[string]struct{}{}
	for _, edge := range t.Edges {
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("%w: edge source %q does not exist", ErrInvalidTopology, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("%w: edge target %q does not exist", ErrInvalidTopology, edge.To)
		}
		if edge.From == edge.To {
			return fmt.Errorf("%w: self edge %q is not allowed", ErrInvalidTopology, edge.From)
		}
		if strings.TrimSpace(edge.Relation) == "" {
			return fmt.Errorf("%w: edge relation is required", ErrInvalidTopology)
		}
		key := edge.From + "\x00" + edge.To + "\x00" + edge.Relation
		if _, duplicate := seenEdges[key]; duplicate {
			return fmt.Errorf("%w: duplicate edge %s -> %s (%s)", ErrInvalidTopology, edge.From, edge.To, edge.Relation)
		}
		seenEdges[key] = struct{}{}
	}
	return nil
}

type ValidationStatus string

const (
	ValidationValid   ValidationStatus = "valid"
	ValidationWarning ValidationStatus = "warning"
	ValidationBlocked ValidationStatus = "blocked"
	ValidationUnknown ValidationStatus = "unknown"
)

func (s ValidationStatus) Valid() bool {
	switch s {
	case ValidationValid, ValidationWarning, ValidationBlocked, ValidationUnknown:
		return true
	default:
		return false
	}
}

type FindingSeverity string

const (
	FindingInfo     FindingSeverity = "info"
	FindingWarning  FindingSeverity = "warning"
	FindingBlocking FindingSeverity = "blocking"
)

type ValidationFinding struct {
	Code            string          `json:"code"`
	Severity        FindingSeverity `json:"severity"`
	AffectedObjects []string        `json:"affected_objects,omitempty"`
	Reason          string          `json:"reason"`
	Remediation     string          `json:"recommended_remediation,omitempty"`
}

type ArchitectureValidationResult struct {
	corecontracts.ObjectMetadata
	SolutionRevisionID string              `json:"solution_revision_id"`
	Status             ValidationStatus    `json:"status"`
	Findings           []ValidationFinding `json:"findings,omitempty"`
	EvaluatedAt        time.Time           `json:"evaluated_at"`
	FreshUntil         time.Time           `json:"fresh_until"`
}

func (r ArchitectureValidationResult) Validate() error {
	if err := r.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidValidation, err)
	}
	if strings.TrimSpace(r.SolutionRevisionID) == "" {
		return fmt.Errorf("%w: solution_revision_id is required", ErrInvalidValidation)
	}
	if !r.Status.Valid() {
		return fmt.Errorf("%w: unsupported status %q", ErrInvalidValidation, r.Status)
	}
	if r.EvaluatedAt.IsZero() || r.FreshUntil.IsZero() || r.FreshUntil.Before(r.EvaluatedAt) {
		return fmt.Errorf("%w: invalid freshness window", ErrInvalidValidation)
	}
	blocking := false
	for _, finding := range r.Findings {
		if strings.TrimSpace(finding.Code) == "" || strings.TrimSpace(finding.Reason) == "" {
			return fmt.Errorf("%w: finding code and reason are required", ErrInvalidValidation)
		}
		switch finding.Severity {
		case FindingInfo, FindingWarning:
		case FindingBlocking:
			blocking = true
		default:
			return fmt.Errorf("%w: unsupported finding severity %q", ErrInvalidValidation, finding.Severity)
		}
	}
	if blocking && r.Status != ValidationBlocked {
		return fmt.Errorf("%w: blocking finding requires blocked status", ErrInvalidValidation)
	}
	return nil
}

type PlanState string

const (
	PlanDraft      PlanState = "draft"
	PlanValidated  PlanState = "validated"
	PlanStale      PlanState = "stale"
	PlanSuperseded PlanState = "superseded"
)

func (s PlanState) Valid() bool {
	switch s {
	case PlanDraft, PlanValidated, PlanStale, PlanSuperseded:
		return true
	default:
		return false
	}
}

type DeploymentStep struct {
	ID         string   `json:"id"`
	Capability string   `json:"capability"`
	TargetRef  string   `json:"target_ref"`
	DependsOn  []string `json:"depends_on,omitempty"`
}

type DeploymentPlan struct {
	corecontracts.ObjectMetadata
	SolutionRevisionID string           `json:"solution_revision_id"`
	PlanDigest         string           `json:"plan_digest"`
	State              PlanState        `json:"state"`
	Steps              []DeploymentStep `json:"steps"`
}

func (p DeploymentPlan) Validate() error {
	if err := p.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPlan, err)
	}
	if strings.TrimSpace(p.SolutionRevisionID) == "" {
		return fmt.Errorf("%w: solution_revision_id is required", ErrInvalidPlan)
	}
	if !sha256Pattern.MatchString(p.PlanDigest) {
		return fmt.Errorf("%w: plan_digest must be sha256:<64 lowercase hex>", ErrInvalidPlan)
	}
	if !p.State.Valid() {
		return fmt.Errorf("%w: unsupported plan state %q", ErrInvalidPlan, p.State)
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("%w: at least one deployment step is required", ErrInvalidPlan)
	}
	steps := make(map[string]DeploymentStep, len(p.Steps))
	for _, step := range p.Steps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.Capability) == "" || strings.TrimSpace(step.TargetRef) == "" {
			return fmt.Errorf("%w: step id, capability and target_ref are required", ErrInvalidPlan)
		}
		if _, duplicate := steps[step.ID]; duplicate {
			return fmt.Errorf("%w: duplicate step %q", ErrInvalidPlan, step.ID)
		}
		steps[step.ID] = step
	}
	for _, step := range p.Steps {
		seenDependencies := map[string]struct{}{}
		for _, dependency := range step.DependsOn {
			if dependency == step.ID {
				return fmt.Errorf("%w: step %q cannot depend on itself", ErrInvalidPlan, step.ID)
			}
			if _, ok := steps[dependency]; !ok {
				return fmt.Errorf("%w: step %q depends on unknown step %q", ErrInvalidPlan, step.ID, dependency)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return fmt.Errorf("%w: step %q has duplicate dependency %q", ErrInvalidPlan, step.ID, dependency)
			}
			seenDependencies[dependency] = struct{}{}
		}
	}
	if err := validateAcyclic(steps); err != nil {
		return err
	}
	return nil
}

func validateAcyclic(steps map[string]DeploymentStep) error {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(steps))
	ids := make([]string, 0, len(steps))
	for id := range steps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("%w: deployment plan dependency cycle includes %q", ErrInvalidPlan, id)
		case visited:
			return nil
		}
		state[id] = visiting
		for _, dependency := range steps[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = visited
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
