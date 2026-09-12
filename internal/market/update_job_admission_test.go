package market

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestAdmitModuleUpdateJobDeterministicAndNonAuthorizing(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	first, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("admission is not deterministic: %#v != %#v", first, second)
	}
	if first.ExecutionAuthorized {
		t.Fatal("admission must not authorize execution")
	}
	if !first.PreserveUserData || !first.PreserveSecretMaterial {
		t.Fatal("admission must preserve user data and secret material")
	}
	if !first.RequirePreMigrationSnapshot || first.RollbackMode != RollbackDataSnapshot {
		t.Fatalf("unsafe rollback contract: %#v", first)
	}
}

func TestAdmitModuleUpdateJobRejectsLifecycleOrJobDrift(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	fixture.lifecycle.TargetVersion = "1.6.0"
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected lifecycle drift rejection")
	}

	fixture = newUpdateAdmissionFixture(t, true)
	fixture.job.JobID = "market-job:tampered"
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected job drift rejection")
	}
}

func TestAdmitModuleUpdateJobRejectsGenerationOrVersionMismatch(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	fixture.bundle.Generation++
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected generation mismatch rejection")
	}

	fixture = newUpdateAdmissionFixture(t, true)
	fixture.bundle.TargetVersion = "1.6.0"
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected version mismatch rejection")
	}
}

func TestAdmitModuleUpdateJobRejectsTamperedBundlePayload(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	fixture.bundlePayload = []byte("tampered-bundle")
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected bundle payload rejection")
	}
}

func TestAdmitModuleUpdateJobRequiresExactMigrationPayloadSet(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	delete(fixture.migrationPayloads, "schema-1-to-2")
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected missing migration rejection")
	}

	fixture = newUpdateAdmissionFixture(t, true)
	fixture.migrationPayloads["unexpected"] = []byte("unexpected")
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected extra migration rejection")
	}
}

func TestAdmitModuleUpdateJobRejectsTamperedMigrationPayload(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	fixture.migrationPayloads["schema-1-to-2"] = []byte("tampered-migration")
	if _, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected migration payload rejection")
	}
}

func TestValidateModuleUpdateJobAdmissionRejectsEvidenceMutation(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, true)
	admission, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateModuleUpdateJobAdmission(admission, fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err != nil {
		t.Fatalf("expected admission to validate: %v", err)
	}

	admission.BundleID = "market-update-bundle:tampered"
	if err := ValidateModuleUpdateJobAdmission(admission, fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected admission evidence mutation rejection")
	}

	admission, err = AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads)
	if err != nil {
		t.Fatal(err)
	}
	admission.ExecutionAuthorized = true
	if err := ValidateModuleUpdateJobAdmission(admission, fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected execution authorization rejection")
	}
}

func TestAdmitModuleUpdateJobApplicationOnlyRollbackContract(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, false)
	admission, err := AdmitModuleUpdateJob(fixture.request, fixture.lifecycle, fixture.job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads)
	if err != nil {
		t.Fatal(err)
	}
	if admission.RequirePreMigrationSnapshot {
		t.Fatal("application-only update must not invent a migration snapshot requirement")
	}
	if admission.RollbackMode != RollbackPreviousVersion || admission.MigrationCount != 0 {
		t.Fatalf("unexpected application-only rollback contract: %#v", admission)
	}
}

func TestAdmitModuleUpdateJobRejectsNoopLifecycle(t *testing.T) {
	fixture := newUpdateAdmissionFixture(t, false)
	request := fixture.request
	request.TargetVersion = request.CurrentVersion
	lifecycle, err := PlanModuleLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := PlanModuleLifecycleJob(lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdmitModuleUpdateJob(request, lifecycle, job, fixture.bundle, fixture.bundlePayload, fixture.migrationPayloads); err == nil {
		t.Fatal("expected no-op update admission rejection")
	}
}

type updateAdmissionFixture struct {
	request           ModuleLifecycleRequest
	lifecycle         ModuleLifecyclePlan
	job               ModuleLifecycleJobPlan
	bundle            ModuleUpdateBundlePlan
	bundlePayload     []byte
	migrationPayloads map[string][]byte
}

func newUpdateAdmissionFixture(t *testing.T, withMigration bool) updateAdmissionFixture {
	t.Helper()
	request := ModuleLifecycleRequest{
		ModuleID:       "inventory-agent",
		Action:         ModuleActionUpdate,
		CurrentState:   ModuleStateActive,
		CurrentVersion: "1.4.0",
		TargetVersion:  "1.5.0",
		Generation:     42,
	}
	lifecycle, err := PlanModuleLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := PlanModuleLifecycleJob(lifecycle)
	if err != nil {
		t.Fatal(err)
	}

	bundlePayload := []byte("module-bundle-v2")
	bundleRequest := ModuleUpdateBundleRequest{
		ModuleID:             request.ModuleID,
		CurrentVersion:       request.CurrentVersion,
		TargetVersion:        request.TargetVersion,
		CurrentSchemaVersion: "1.0.0",
		TargetSchemaVersion:  "1.0.0",
		BundleSHA256:         updateAdmissionDigest(bundlePayload),
		BundleSizeBytes:      uint64(len(bundlePayload)),
		Generation:           request.Generation,
	}
	migrationPayloads := map[string][]byte{}
	if withMigration {
		migrationPayload := []byte("ALTER TABLE state ADD COLUMN safe_value TEXT;")
		bundleRequest.TargetSchemaVersion = "2.0.0"
		bundleRequest.Migrations = []ModuleMigrationArtifact{{
			ID:                "schema-1-to-2",
			FromSchemaVersion: "1.0.0",
			ToSchemaVersion:   "2.0.0",
			SHA256:            updateAdmissionDigest(migrationPayload),
			SizeBytes:         uint64(len(migrationPayload)),
		}}
		migrationPayloads["schema-1-to-2"] = migrationPayload
	}
	bundle, err := PlanModuleUpdateBundle(bundleRequest)
	if err != nil {
		t.Fatal(err)
	}
	return updateAdmissionFixture{
		request:           request,
		lifecycle:         lifecycle,
		job:               job,
		bundle:            bundle,
		bundlePayload:     bundlePayload,
		migrationPayloads: migrationPayloads,
	}
}

func updateAdmissionDigest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
