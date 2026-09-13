package solutions

import (
	"testing"
	"time"

	"control-center/internal/corecontracts"
)

func metadata(id string) corecontracts.ObjectMetadata {
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

func TestTopologyValidate(t *testing.T) {
	topology := Topology{
		Nodes: []TopologyNode{
			{ID: "network", Kind: "network-fabric"},
			{ID: "cluster", Kind: "virtualization-cluster", ProviderBinding: "provider-binding:cluster-a"},
			{ID: "backup", Kind: "backup-service"},
		},
		Edges: []TopologyEdge{
			{From: "cluster", To: "network", Relation: "depends_on"},
			{From: "backup", To: "cluster", Relation: "protects"},
		},
	}
	if err := topology.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	topology.Edges = append(topology.Edges, TopologyEdge{From: "missing", To: "cluster", Relation: "depends_on"})
	if err := topology.Validate(); err == nil {
		t.Fatal("edge with missing node must be rejected")
	}
}

func TestArchitectureValidationRequiresBlockedStatusForBlockingFinding(t *testing.T) {
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	result := ArchitectureValidationResult{
		ObjectMetadata:     metadata("validation:solution-a:1"),
		SolutionRevisionID: "solution-revision:a:1",
		Status:             ValidationValid,
		EvaluatedAt:        now,
		FreshUntil:         now.Add(time.Hour),
		Findings: []ValidationFinding{
			{Code: "rack-domain-conflict", Severity: FindingBlocking, Reason: "required rack independence is not proven"},
		},
	}
	if err := result.Validate(); err == nil {
		t.Fatal("blocking finding must require blocked status")
	}
	result.Status = ValidationBlocked
	if err := result.Validate(); err != nil {
		t.Fatalf("blocked result should validate: %v", err)
	}
}

func TestDeploymentPlanValidateDAG(t *testing.T) {
	plan := DeploymentPlan{
		ObjectMetadata:     metadata("deployment-plan:a:1"),
		SolutionRevisionID: "solution-revision:a:1",
		PlanDigest:         "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		State:              PlanValidated,
		Steps: []DeploymentStep{
			{ID: "network", Capability: "fabric.configure", TargetRef: "fabric-a"},
			{ID: "cluster", Capability: "cluster.create", TargetRef: "cluster-a", DependsOn: []string{"network"}},
			{ID: "backup", Capability: "backup.integrate", TargetRef: "backup-a", DependsOn: []string{"cluster"}},
		},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	plan.Steps[0].DependsOn = []string{"backup"}
	if err := plan.Validate(); err == nil {
		t.Fatal("dependency cycle must be rejected")
	}
}

func TestSolutionRevisionValidateDigest(t *testing.T) {
	revision := SolutionRevision{
		ObjectMetadata: metadata("solution-revision:a:1"),
		SolutionID:     "solution-a",
		Ordinal:        1,
		State:          RevisionProposed,
		TopologyDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	revision.TopologyDigest = "latest"
	if err := revision.Validate(); err == nil {
		t.Fatal("non-digest topology identity must be rejected")
	}
}
