package providers

import (
	"testing"
	"time"

	"control-center/internal/corecontracts"
)

func validManifest() Manifest {
	return Manifest{
		SchemaVersion:            ManifestSchemaV1,
		ProviderID:               "proxmox-ve",
		ContractVersion:          "1",
		ProviderVersion:          "1.0.0",
		ProductFamily:            "Proxmox VE",
		SupportedProductVersions: []string{"9.x"},
		ManagementLevels:         []ManagementLevel{ManagementObserved, ManagementConnected, ManagementManaged},
		Capabilities: []CapabilitySpec{
			{
				ID:               "discover",
				ManagementLevels: []ManagementLevel{ManagementObserved, ManagementConnected, ManagementManaged},
				Mutation:         false,
				RiskClass:        RiskReadOnly,
				Idempotency:      "repeatable-read",
			},
			{
				ID:                  "cluster.create",
				ManagementLevels:    []ManagementLevel{ManagementManaged},
				Mutation:            true,
				RiskClass:           RiskHigh,
				RequiredPermissions: []string{"cluster.manage"},
				RequiredSecretRefs:  []string{"proxmox-api"},
				Locks:               []string{"cluster-membership"},
				Idempotency:         "provider-operation-key",
				Verification:        []string{"cluster-quorum"},
				FailureModel:        "partial-create",
				Rollback:            "remove-unjoined-members",
				Recovery:            "operator-reconcile",
			},
		},
		QualificationStatus: QualificationQualified,
	}
}

func TestManifestValidateAndSupports(t *testing.T) {
	manifest := validManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !manifest.Supports("cluster.create", ManagementManaged) {
		t.Fatal("managed cluster.create should be supported")
	}
	if manifest.Supports("cluster.create", ManagementObserved) {
		t.Fatal("managed-only cluster.create must not be exposed at observed level")
	}
	if !manifest.Supports("discover", ManagementObserved) {
		t.Fatal("discover should be available at observed level")
	}
	if manifest.Supports("cluster.create", ManagementLevel("unsupported")) {
		t.Fatal("unsupported management level must fail closed")
	}
	if manifest.Supports("remove.anything", ManagementManaged) {
		t.Fatal("undeclared capability must not be supported")
	}
}

func TestManifestRejectsUnsafeCapabilityMetadata(t *testing.T) {
	manifest := validManifest()
	manifest.Capabilities[1].RiskClass = RiskReadOnly
	if err := manifest.Validate(); err == nil {
		t.Fatal("mutation with read_only risk must be rejected")
	}

	manifest = validManifest()
	manifest.Capabilities = append(manifest.Capabilities, manifest.Capabilities[0])
	if err := manifest.Validate(); err == nil {
		t.Fatal("duplicate capability must be rejected")
	}

	manifest = validManifest()
	manifest.Capabilities[1].ManagementLevels = []ManagementLevel{ManagementObserved}
	if err := manifest.Validate(); err == nil {
		t.Fatal("observed management level must not expose a mutation capability")
	}

	manifest = validManifest()
	manifest.ManagementLevels = []ManagementLevel{ManagementObserved, ManagementManaged}
	manifest.Capabilities[0].ManagementLevels = []ManagementLevel{ManagementConnected}
	if err := manifest.Validate(); err == nil {
		t.Fatal("capability level not declared by manifest must be rejected")
	}
}

func TestBindingValidate(t *testing.T) {
	now := time.Date(2026, 9, 13, 7, 0, 0, 0, time.UTC)
	binding := Binding{
		ObjectMetadata: corecontracts.ObjectMetadata{
			ObjectID:        "provider-binding:cluster-a",
			ScopeID:         "site-a",
			OwnerScope:      "site-a",
			Generation:      1,
			ResourceVersion: "rv-1",
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		ProviderID:      "proxmox-ve",
		ProviderVersion: "1.0.0",
		ContractVersion: "1",
		ProductFamily:   "Proxmox VE",
		ProductVersion:  "9.2",
		ManagementLevel: ManagementManaged,
		TargetRef:       "cluster-a",
		Health:          BindingHealthy,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	binding.ManagementLevel = ManagementLevel("magic")
	if err := binding.Validate(); err == nil {
		t.Fatal("unknown management level must be rejected")
	}
}
