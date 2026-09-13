package providers

import (
	"reflect"
	"testing"
)

func TestCapabilityValidateDoesNotMutateDeclarations(t *testing.T) {
	capability := CapabilitySpec{
		ID:                  "cluster.create",
		ManagementLevels:    []ManagementLevel{ManagementManaged},
		Mutation:            true,
		RiskClass:           RiskHigh,
		RequiredPermissions: []string{"z.permission", "a.permission"},
		RequiredSecretRefs:  []string{"z-secret", "a-secret"},
		Locks:               []string{"z-lock", "a-lock"},
		Idempotency:         "operation-key",
	}
	levels := append([]ManagementLevel(nil), capability.ManagementLevels...)
	permissions := append([]string(nil), capability.RequiredPermissions...)
	secrets := append([]string(nil), capability.RequiredSecretRefs...)
	locks := append([]string(nil), capability.Locks...)

	if err := capability.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !reflect.DeepEqual(capability.ManagementLevels, levels) {
		t.Fatal("Validate() mutated ManagementLevels")
	}
	if !reflect.DeepEqual(capability.RequiredPermissions, permissions) {
		t.Fatal("Validate() mutated RequiredPermissions")
	}
	if !reflect.DeepEqual(capability.RequiredSecretRefs, secrets) {
		t.Fatal("Validate() mutated RequiredSecretRefs")
	}
	if !reflect.DeepEqual(capability.Locks, locks) {
		t.Fatal("Validate() mutated Locks")
	}
}
