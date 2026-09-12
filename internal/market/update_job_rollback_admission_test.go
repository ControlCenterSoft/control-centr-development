package market

import "testing"

func TestPlanModuleUpdateJobRollbackAdmissionDeterministicAndSafe(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, true)
	first, err := PlanModuleUpdateJobRollbackAdmission(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanModuleUpdateJobRollbackAdmission(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("rollback admission is not deterministic: %#v != %#v", first, second)
	}
	if first.RollbackAdmissionID == "" ||
		first.RecordID != state.Record.RecordID ||
		first.VerificationReceiptID != state.VerificationReceipt.ReceiptID ||
		first.ApplyReceiptID != state.ApplyReceipt.ReceiptID ||
		first.RestoreVersion != admission.CurrentVersion ||
		first.FailedTargetVersion != admission.TargetVersion ||
		first.PreMigrationSnapshotID != state.ApplyReceipt.PreMigrationSnapshotID {
		t.Fatalf("rollback admission lost exact lineage: %#v", first)
	}
	if first.ExpectedState != ModuleJobStateRollingBack ||
		first.ExpectedStateVersion != 4 ||
		first.ExpectedJournalSequence != 3 ||
		!first.SingleUse ||
		!first.AtomicPersistenceRequired ||
		!first.FreshRevalidationRequired ||
		!first.PreserveUserData ||
		!first.PreserveSecretMaterial ||
		first.ExecutionAuthorized ||
		first.ProductionMutationAllowed {
		t.Fatalf("rollback admission violates safety contract: %#v", first)
	}
	if err := ValidateModuleUpdateJobRollbackAdmission(first, state, admission); err != nil {
		t.Fatalf("valid rollback admission rejected: %v", err)
	}
}

func TestPlanModuleUpdateJobRollbackAdmissionApplicationOnlyUsesPreviousVersion(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, false)
	planned, err := PlanModuleUpdateJobRollbackAdmission(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	if planned.RollbackMode != RollbackPreviousVersion || planned.PreMigrationSnapshotID != "" {
		t.Fatalf("application-only rollback invented snapshot semantics: %#v", planned)
	}
}

func TestPlanModuleUpdateJobRollbackAdmissionRejectsPassedVerification(t *testing.T) {
	admission, verifying := testVerificationState(t)
	passed, err := CommitModuleUpdateJobVerificationCAS(
		verifying,
		admission,
		verifying.Record.StateVersion,
		ModuleUpdateJobVerificationPassed,
		"verify:passed-rollback",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PlanModuleUpdateJobRollbackAdmission(passed.State, admission); err == nil {
		t.Fatal("expected successful verification to reject rollback admission")
	}
}

func TestPlanModuleUpdateJobRollbackAdmissionRejectsSnapshotLineageDrift(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, true)
	state.ApplyReceipt.PreMigrationSnapshotID = "snapshot:other"
	if _, err := PlanModuleUpdateJobRollbackAdmission(state, admission); err == nil {
		t.Fatal("expected apply snapshot lineage drift rejection")
	}
}

func TestPlanModuleUpdateJobRollbackAdmissionRejectsStaleLifecycleRevision(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, true)
	state.Record.StateVersion++
	if _, err := PlanModuleUpdateJobRollbackAdmission(state, admission); err == nil {
		t.Fatal("expected stale lifecycle state-version rejection")
	}
}

func TestPlanModuleUpdateJobRollbackAdmissionRejectsVerificationAuthorityTampering(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, true)
	state.VerificationReceipt.ExecutionAuthorized = true
	if _, err := PlanModuleUpdateJobRollbackAdmission(state, admission); err == nil {
		t.Fatal("expected widened verification authority rejection")
	}
}

func TestValidateModuleUpdateJobRollbackAdmissionRejectsAuthorityTampering(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, true)
	planned, err := PlanModuleUpdateJobRollbackAdmission(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	planned.ExecutionAuthorized = true
	if err := ValidateModuleUpdateJobRollbackAdmission(planned, state, admission); err == nil {
		t.Fatal("expected rollback authority tampering rejection")
	}
}

func TestPlanModuleUpdateJobRollbackAdmissionRejectsRollbackContractDrift(t *testing.T) {
	admission, state := testFailedVerificationRollbackState(t, true)
	drifted := admission
	drifted.RollbackMode = RollbackPreviousVersion
	drifted.RequirePreMigrationSnapshot = false
	drifted.MigrationCount = 0
	drifted.AdmissionID = moduleUpdateJobAdmissionID(drifted)
	if _, err := PlanModuleUpdateJobRollbackAdmission(state, drifted); err == nil {
		t.Fatal("expected rollback contract drift rejection")
	}
}

func testFailedVerificationRollbackState(
	t *testing.T,
	withMigration bool,
) (ModuleUpdateJobAdmission, ModuleUpdateJobVerificationPersistenceState) {
	t.Helper()
	admission := testApplyAdmission(withMigration)
	claimed := testClaimedApplyState(t, admission)
	snapshotID := ""
	if withMigration {
		snapshotID = "snapshot:market-rollback"
	}
	applied, err := CommitModuleUpdateJobApplySuccessCAS(
		claimed,
		admission,
		claimed.Record.StateVersion,
		snapshotID,
	)
	if err != nil {
		t.Fatal(err)
	}
	verifying, err := InitializeModuleUpdateJobVerificationPersistence(applied.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := CommitModuleUpdateJobVerificationCAS(
		verifying,
		admission,
		verifying.Record.StateVersion,
		ModuleUpdateJobVerificationFailed,
		"verify:rollback-required",
	)
	if err != nil {
		t.Fatal(err)
	}
	return admission, failed.State
}
