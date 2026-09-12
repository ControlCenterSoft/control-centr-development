package market

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestPlanModuleUpdateBundleDeterministic(t *testing.T) {
	bundle := []byte("module-bundle-v2")
	migration := []byte("ALTER SCHEMA ADD COLUMN safe_value;")
	request := validUpdateBundleRequest(bundle, migration)

	first, err := PlanModuleUpdateBundle(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanModuleUpdateBundle(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.BundleID != second.BundleID || first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("plan is not deterministic: %#v %#v", first, second)
	}
	if first.ExecutionAuthorized {
		t.Fatal("planning must not authorize execution")
	}
	if !first.PreserveUserData || !first.PreserveSecretMaterial {
		t.Fatal("update must preserve user data and secret material")
	}
	if !first.RequirePreMigrationSnapshot || first.RollbackMode != RollbackDataSnapshot {
		t.Fatalf("unsafe migration rollback contract: %#v", first)
	}
}

func TestPlanModuleUpdateBundleRejectsDestructiveMigration(t *testing.T) {
	request := validUpdateBundleRequest([]byte("bundle"), []byte("migration"))
	request.Migrations[0].Destructive = true
	if _, err := PlanModuleUpdateBundle(request); err == nil {
		t.Fatal("expected destructive migration rejection")
	}
}

func TestPlanModuleUpdateBundleRejectsDownMigration(t *testing.T) {
	request := validUpdateBundleRequest([]byte("bundle"), []byte("migration"))
	request.Migrations[0].DownMigration = true
	if _, err := PlanModuleUpdateBundle(request); err == nil {
		t.Fatal("expected down migration rejection")
	}
}

func TestPlanModuleUpdateBundleRejectsMigrationGap(t *testing.T) {
	request := validUpdateBundleRequest([]byte("bundle"), []byte("migration"))
	request.Migrations[0].FromSchemaVersion = "1.1.0"
	if _, err := PlanModuleUpdateBundle(request); err == nil {
		t.Fatal("expected migration gap rejection")
	}
}

func TestPlanModuleUpdateBundleRejectsSchemaDowngrade(t *testing.T) {
	request := validUpdateBundleRequest([]byte("bundle"), []byte("migration"))
	request.CurrentSchemaVersion = "2.0.0"
	request.TargetSchemaVersion = "1.0.0"
	request.Migrations = nil
	if _, err := PlanModuleUpdateBundle(request); err == nil {
		t.Fatal("expected schema downgrade rejection")
	}
}

func TestPlanModuleUpdateBundleRejectsDuplicateMigrationID(t *testing.T) {
	request := validUpdateBundleRequest([]byte("bundle"), []byte("migration-1"))
	secondPayload := []byte("migration-2")
	request.TargetSchemaVersion = "1.2.0"
	request.Migrations[0].ToSchemaVersion = "1.1.0"
	request.Migrations = append(request.Migrations, ModuleMigrationArtifact{
		ID:                request.Migrations[0].ID,
		FromSchemaVersion: "1.1.0",
		ToSchemaVersion:   "1.2.0",
		SHA256:            digestHex(secondPayload),
		SizeBytes:         uint64(len(secondPayload)),
	})
	if _, err := PlanModuleUpdateBundle(request); err == nil {
		t.Fatal("expected duplicate migration id rejection")
	}
}

func TestVerifyUpdateBundlePayloadFailClosed(t *testing.T) {
	bundle := []byte("module-bundle-v2")
	migration := []byte("migration")
	plan, err := PlanModuleUpdateBundle(validUpdateBundleRequest(bundle, migration))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyUpdateBundlePayload(plan, bundle); err != nil {
		t.Fatalf("expected payload to verify: %v", err)
	}
	if err := VerifyUpdateBundlePayload(plan, []byte("module-bundle-vX")); err == nil {
		t.Fatal("expected tampered bundle rejection")
	}
}

func TestVerifyMigrationArtifactPayloadFailClosed(t *testing.T) {
	bundle := []byte("bundle")
	migration := []byte("migration")
	plan, err := PlanModuleUpdateBundle(validUpdateBundleRequest(bundle, migration))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyMigrationArtifactPayload(plan, "schema-1-to-2", migration); err != nil {
		t.Fatalf("expected migration payload to verify: %v", err)
	}
	if err := VerifyMigrationArtifactPayload(plan, "schema-1-to-2", []byte("tampered")); err == nil {
		t.Fatal("expected tampered migration rejection")
	}
	if err := VerifyMigrationArtifactPayload(plan, "unknown", migration); err == nil {
		t.Fatal("expected unknown migration rejection")
	}
}

func TestValidateModuleUpdateBundlePlanRejectsSafetyMutation(t *testing.T) {
	plan, err := PlanModuleUpdateBundle(validUpdateBundleRequest([]byte("bundle"), []byte("migration")))
	if err != nil {
		t.Fatal(err)
	}

	plan.PreserveUserData = false
	if err := ValidateModuleUpdateBundlePlan(plan); err == nil {
		t.Fatal("expected user-data preservation mutation rejection")
	}

	plan, err = PlanModuleUpdateBundle(validUpdateBundleRequest([]byte("bundle"), []byte("migration")))
	if err != nil {
		t.Fatal(err)
	}
	plan.ExecutionAuthorized = true
	if err := ValidateModuleUpdateBundlePlan(plan); err == nil {
		t.Fatal("expected execution authorization mutation rejection")
	}
}

func TestPlanModuleUpdateBundleWithoutSchemaChangeUsesVersionRollback(t *testing.T) {
	bundle := []byte("application-only-update")
	request := validUpdateBundleRequest(bundle, []byte("unused"))
	request.TargetSchemaVersion = request.CurrentSchemaVersion
	request.Migrations = nil
	plan, err := PlanModuleUpdateBundle(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequirePreMigrationSnapshot {
		t.Fatal("application-only update must not invent migration snapshot requirement")
	}
	if plan.RollbackMode != RollbackPreviousVersion {
		t.Fatalf("unexpected rollback mode %q", plan.RollbackMode)
	}
}

func TestPlanModuleUpdateBundleIdentityChangesWithMigrationEvidence(t *testing.T) {
	request := validUpdateBundleRequest([]byte("bundle"), []byte("migration"))
	first, err := PlanModuleUpdateBundle(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Migrations[0].SHA256 = digestHex([]byte("different-migration"))
	second, err := PlanModuleUpdateBundle(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.BundleID == second.BundleID {
		t.Fatal("bundle identity must change with migration checksum evidence")
	}
}

func validUpdateBundleRequest(bundle, migration []byte) ModuleUpdateBundleRequest {
	return ModuleUpdateBundleRequest{
		ModuleID:             "inventory-agent",
		CurrentVersion:       "1.4.0",
		TargetVersion:        "1.5.0",
		CurrentSchemaVersion: "1.0.0",
		TargetSchemaVersion:  "2.0.0",
		BundleSHA256:         digestHex(bundle),
		BundleSizeBytes:      uint64(len(bundle)),
		Generation:           42,
		Migrations: []ModuleMigrationArtifact{{
			ID:                "schema-1-to-2",
			FromSchemaVersion: "1.0.0",
			ToSchemaVersion:   "2.0.0",
			SHA256:            digestHex(migration),
			SizeBytes:         uint64(len(migration)),
		}},
	}
}

func digestHex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
