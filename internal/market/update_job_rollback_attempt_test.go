package market

import "testing"

func testCommittedRollbackClaim(t *testing.T, withMigration bool) (ModuleUpdateJobAdmission, ModuleUpdateJobRollbackClaimPersistenceState) {
	t.Helper()
	admission, source := testFailedVerificationRollbackState(t, withMigration)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := CommitModuleUpdateJobRollbackClaimCAS(
		initialized,
		admission,
		"rollback-worker-attempt",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return admission, claimed.State
}

func TestModuleUpdateJobRollbackAttemptIsDeterministicAndBounded(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, true)
	first, err := AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.AttemptID == "" {
		t.Fatalf("rollback attempt is not deterministic: %#v %#v", first, second)
	}
	if !first.RollbackExecutionAuthorized || first.GenericCommandAuthorized ||
		first.MigrationDowngradeAllowed || first.AutomaticRetryAuthorized ||
		first.ProductionMutationAllowed || !first.SingleUse || !first.FreshStateReadRequired ||
		!first.PreserveUserData || !first.PreserveSecretMaterial {
		t.Fatalf("rollback attempt widened authority: %#v", first)
	}
	if first.RestoreVersion != state.RollbackAdmission.RestoreVersion ||
		first.RollbackMode != state.RollbackAdmission.RollbackMode ||
		first.PreMigrationSnapshotID != state.RollbackAdmission.PreMigrationSnapshotID {
		t.Fatalf("rollback attempt changed sealed restore target: %#v", first)
	}
}

func TestModuleUpdateJobRollbackAttemptSupportsApplicationOnlyRollback(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, false)
	attempt, err := AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.RollbackMode != RollbackPreviousVersion || attempt.PreMigrationSnapshotID != "" {
		t.Fatalf("application-only rollback attempt is invalid: %#v", attempt)
	}
}

func TestModuleUpdateJobRollbackAttemptRejectsFreshRevisionDrift(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, true)
	state.Source.Record.StateVersion++
	if _, err := AdmitModuleUpdateJobRollbackAttempt(state, admission); err == nil {
		t.Fatal("expected state-version drift rejection")
	}
}

func TestModuleUpdateJobRollbackAttemptRejectsJournalDrift(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, true)
	state.Source.Record.JournalSequence++
	if _, err := AdmitModuleUpdateJobRollbackAttempt(state, admission); err == nil {
		t.Fatal("expected journal drift rejection")
	}
}

func TestModuleUpdateJobRollbackAttemptRejectsClaimTampering(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, true)
	state.Claim.WorkerID = "rollback-worker-forged"
	if _, err := AdmitModuleUpdateJobRollbackAttempt(state, admission); err == nil {
		t.Fatal("expected rollback claim tampering rejection")
	}
}

func TestModuleUpdateJobRollbackAttemptRejectsRestoreTargetTampering(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, true)
	attempt, err := AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	attempt.RestoreVersion = attempt.FailedTargetVersion
	if err := ValidateModuleUpdateJobRollbackAttempt(attempt, state, admission); err == nil {
		t.Fatal("expected caller-selected restore target rejection")
	}
}

func TestModuleUpdateJobRollbackAttemptRejectsAuthorityTampering(t *testing.T) {
	admission, state := testCommittedRollbackClaim(t, true)
	attempt, err := AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	attempt.GenericCommandAuthorized = true
	if err := ValidateModuleUpdateJobRollbackAttempt(attempt, state, admission); err == nil {
		t.Fatal("expected generic command authority rejection")
	}
	attempt, err = AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	attempt.MigrationDowngradeAllowed = true
	if err := ValidateModuleUpdateJobRollbackAttempt(attempt, state, admission); err == nil {
		t.Fatal("expected migration downgrade authority rejection")
	}
	attempt, err = AdmitModuleUpdateJobRollbackAttempt(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	attempt.AutomaticRetryAuthorized = true
	if err := ValidateModuleUpdateJobRollbackAttempt(attempt, state, admission); err == nil {
		t.Fatal("expected automatic retry authority rejection")
	}
}
