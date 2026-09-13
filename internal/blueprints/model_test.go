package blueprints

import (
	"testing"
	"time"

	"control-center/internal/corecontracts"
)

func blueprintMetadata(id string) corecontracts.ObjectMetadata {
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	return corecontracts.ObjectMetadata{
		ObjectID:        id,
		ScopeID:         "global",
		OwnerScope:      "global",
		Generation:      1,
		ResourceVersion: "rv-1",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func validBlueprintRevision() BlueprintRevision {
	return BlueprintRevision{
		ObjectMetadata:      blueprintMetadata("blueprint-revision:ha-virtualization:1"),
		BlueprintID:         "blueprint:ha-virtualization",
		Ordinal:             1,
		QualificationStatus: QualificationQualified,
		BlueprintDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SignatureRef:        "signature:ha-virtualization:1",
		IntentClasses:       []string{"virtualization", "high-availability"},
		Roles: []BlueprintRole{
			{
				ID:                   "virtualization-node",
				Kind:                 "compute",
				MinCount:             3,
				MaxCount:             0,
				RequiredCapabilities: []string{"cluster.join", "vm.migrate"},
			},
			{
				ID:                   "backup-service",
				Kind:                 "backup",
				MinCount:             1,
				MaxCount:             1,
				RequiredCapabilities: []string{"backup.integrate", "restore"},
			},
		},
		TopologyRules: []TopologyRule{
			{FromRole: "backup-service", ToRole: "virtualization-node", Relation: "protects", Required: true},
		},
		ExpansionStrategies: []string{"scale-out", "new-cluster"},
	}
}

func TestBlueprintRevisionValidate(t *testing.T) {
	revision := validBlueprintRevision()
	if err := revision.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestQualifiedBlueprintRequiresSignature(t *testing.T) {
	revision := validBlueprintRevision()
	revision.SignatureRef = ""
	if err := revision.Validate(); err == nil {
		t.Fatal("qualified blueprint without signature_ref must be rejected")
	}
}

func TestTopologyRuleMustReferenceKnownRole(t *testing.T) {
	revision := validBlueprintRevision()
	revision.TopologyRules[0].ToRole = "missing"
	if err := revision.Validate(); err == nil {
		t.Fatal("topology rule with missing role must be rejected")
	}
}

func TestBlueprintRoleCountRange(t *testing.T) {
	revision := validBlueprintRevision()
	revision.Roles[0].MaxCount = 2
	if err := revision.Validate(); err == nil {
		t.Fatal("max_count below min_count must be rejected")
	}
}
