package synthesis

import (
	"testing"
	"time"

	"control-center/internal/corecontracts"
)

func synthesisMetadata(id string) corecontracts.ObjectMetadata {
	now := time.Date(2026, 9, 13, 8, 30, 0, 0, time.UTC)
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

func TestSynthesisAssessmentValidate(t *testing.T) {
	now := time.Date(2026, 9, 13, 8, 30, 0, 0, time.UTC)
	assessment := SynthesisAssessment{
		ObjectMetadata:        synthesisMetadata("synthesis-assessment:a:1"),
		IntentRevisionID:      "intent-revision:a:1",
		BlueprintRevisionID:   "blueprint-revision:ha:1",
		InputSnapshotDigest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ProviderCatalogDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PolicyRevision:        "policy-rv-1",
		State:                 AssessmentCompleted,
		EvaluatedAt:           now,
	}
	if err := assessment.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCandidateUnknownConstraintFailsClosed(t *testing.T) {
	candidate := SolutionCandidate{
		ObjectMetadata: synthesisMetadata("synthesis-candidate:a:1"),
		AssessmentID:   "synthesis-assessment:a:1",
		Status:         CandidateValid,
		TopologyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		ConstraintResults: []ConstraintResult{
			{RequirementID: "availability.node-failure", Status: ConstraintUnknown, Reason: "rack evidence is stale"},
		},
	}
	if err := candidate.Validate(); err == nil {
		t.Fatal("unknown mandatory constraint must not validate as a valid candidate")
	}
	candidate.Status = CandidateBlocked
	if err := candidate.Validate(); err != nil {
		t.Fatalf("blocked candidate should validate: %v", err)
	}
}

func TestCandidateWarningRequiresWarningStatus(t *testing.T) {
	candidate := SolutionCandidate{
		ObjectMetadata: synthesisMetadata("synthesis-candidate:a:2"),
		AssessmentID:   "synthesis-assessment:a:1",
		Status:         CandidateValid,
		TopologyDigest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ConstraintResults: []ConstraintResult{
			{RequirementID: "cost.preference", Status: ConstraintWarning, Reason: "cost evidence is incomplete"},
		},
	}
	if err := candidate.Validate(); err == nil {
		t.Fatal("warning constraint must require valid_with_warning")
	}
	candidate.Status = CandidateValidWithWarning
	if err := candidate.Validate(); err != nil {
		t.Fatalf("valid_with_warning candidate should validate: %v", err)
	}
}

func TestSynthesisDecisionRequiresRationale(t *testing.T) {
	now := time.Date(2026, 9, 13, 8, 30, 0, 0, time.UTC)
	decision := SynthesisDecision{
		ObjectMetadata:        synthesisMetadata("synthesis-decision:a:1"),
		AssessmentID:          "synthesis-assessment:a:1",
		SelectedCandidateID:   "synthesis-candidate:a:2",
		Rationale:             "selected for lower operational complexity",
		ResultingSolutionRevisionID: "solution-revision:a:2",
		DecidedAt:             now,
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	decision.Rationale = ""
	if err := decision.Validate(); err == nil {
		t.Fatal("decision without rationale must be rejected")
	}
}

func TestExpansionAssessmentValidate(t *testing.T) {
	now := time.Date(2026, 9, 13, 8, 30, 0, 0, time.UTC)
	assessment := ExpansionAssessment{
		ObjectMetadata:      synthesisMetadata("expansion-assessment:a:1"),
		SolutionRevisionID:  "solution-revision:a:1",
		InputSnapshotDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		State:               AssessmentCompleted,
		Options: []ExpansionOption{
			{Strategy: ExpansionScaleOut, Status: CandidateValid, Reason: "existing cluster can safely accept two nodes"},
			{Strategy: ExpansionNewCluster, Status: CandidateValidWithWarning, Reason: "better isolation but higher material delta"},
		},
		EvaluatedAt: now,
	}
	if err := assessment.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	assessment.Options = append(assessment.Options, ExpansionOption{
		Strategy: ExpansionScaleOut,
		Status:   CandidateBlocked,
		Reason:   "duplicate strategy",
	})
	if err := assessment.Validate(); err == nil {
		t.Fatal("duplicate expansion strategy must be rejected")
	}
}
