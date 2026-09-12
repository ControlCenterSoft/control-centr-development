package market

import "testing"

func TestModuleUpdateJobRollbackClaimDeterministicAndReplaySafe(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission)
	if err != nil {
		t.Fatal(err)
	}

	first, err := CommitModuleUpdateJobRollbackClaimCAS(
		initialized,
		admission,
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.ClaimAppendRequired {
		t.Fatalf("first rollback claim has invalid persistence semantics: %#v", first)
	}
	if first.Claim.ClaimID == "" ||
		first.Claim.RollbackAdmissionID != rollbackAdmission.RollbackAdmissionID ||
		first.Claim.RecordID != rollbackAdmission.RecordID ||
		first.Claim.JobID != rollbackAdmission.JobID ||
		first.Claim.SourceAdmissionID != rollbackAdmission.SourceAdmissionID ||
		first.Claim.VerificationReceiptID != rollbackAdmission.VerificationReceiptID ||
		first.Claim.ExpectedStateVersion != rollbackAdmission.ExpectedStateVersion ||
		first.Claim.ExpectedJournalSequence != rollbackAdmission.ExpectedJournalSequence ||
		first.Claim.ClaimSequence != rollbackAdmission.ExpectedJournalSequence+1 {
		t.Fatalf("rollback claim lost exact lineage: %#v", first.Claim)
	}
	if !first.Claim.SingleUse ||
		!first.Claim.AtomicPersistenceRequired ||
		!first.Claim.FreshRevalidationRequired ||
		!first.Claim.PreserveUserData ||
		!first.Claim.PreserveSecretMaterial ||
		first.Claim.ExecutionAuthorized ||
		first.Claim.ProductionMutationAllowed ||
		first.ExecutionAuthorized ||
		first.ProductionMutation {
		t.Fatalf("rollback claim widened authority: %#v", first)
	}

	replay, err := CommitModuleUpdateJobRollbackClaimCAS(
		first.State,
		admission,
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.ClaimAppendRequired || replay.Claim != first.Claim || replay.State.Claim != first.State.Claim {
		t.Fatalf("exact rollback claim replay changed durable evidence: %#v", replay)
	}

	reopened, err := ReopenModuleUpdateJobRollbackClaimPersistence(replay.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	if reopened != first.Claim {
		t.Fatalf("restart reopen changed rollback claim: %#v != %#v", reopened, first.Claim)
	}
}

func TestModuleUpdateJobRollbackClaimRejectsCompetingWorker(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
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
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitModuleUpdateJobRollbackClaimCAS(
		claimed.State,
		admission,
		"rollback-worker-2",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	); err == nil {
		t.Fatal("expected competing rollback worker rejection")
	}
}

func TestModuleUpdateJobRollbackClaimRejectsStaleRevision(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitModuleUpdateJobRollbackClaimCAS(
		initialized,
		admission,
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion+1,
		rollbackAdmission.ExpectedJournalSequence,
	); err == nil {
		t.Fatal("expected stale rollback state-version rejection")
	}
	if _, err := CommitModuleUpdateJobRollbackClaimCAS(
		initialized,
		admission,
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence+1,
	); err == nil {
		t.Fatal("expected stale rollback journal-sequence rejection")
	}
}

func TestModuleUpdateJobRollbackClaimRejectsSourceRevisionDrift(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission)
	if err != nil {
		t.Fatal(err)
	}
	initialized.Source.Record.StateVersion++
	if _, err := CommitModuleUpdateJobRollbackClaimCAS(
		initialized,
		admission,
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	); err == nil {
		t.Fatal("expected rollback source revision drift rejection")
	}
}

func TestModuleUpdateJobRollbackClaimRejectsAuthorityTampering(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
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
		"rollback-worker-1",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	claimed.State.Claim.ExecutionAuthorized = true
	if _, err := ReopenModuleUpdateJobRollbackClaimPersistence(claimed.State, admission); err == nil {
		t.Fatal("expected rollback claim authority tampering rejection")
	}
}

func TestModuleUpdateJobRollbackClaimRejectsAdmissionTampering(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	rollbackAdmission.ProductionMutationAllowed = true
	if _, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission); err == nil {
		t.Fatal("expected widened rollback admission authority rejection")
	}
}

func TestModuleUpdateJobRollbackClaimSupportsApplicationOnlyRollback(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, false)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	if rollbackAdmission.RollbackMode != RollbackPreviousVersion || rollbackAdmission.PreMigrationSnapshotID != "" {
		t.Fatalf("application-only rollback admission is invalid: %#v", rollbackAdmission)
	}
	initialized, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := CommitModuleUpdateJobRollbackClaimCAS(
		initialized,
		admission,
		"rollback-worker-app-only",
		rollbackAdmission.ExpectedStateVersion,
		rollbackAdmission.ExpectedJournalSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Claim.RollbackAdmissionID != rollbackAdmission.RollbackAdmissionID || claimed.Claim.ExecutionAuthorized {
		t.Fatalf("application-only rollback claim lost safety lineage: %#v", claimed.Claim)
	}
}

func TestModuleUpdateJobRollbackClaimRequiresCommittedClaimOnReopen(t *testing.T) {
	admission, source := testFailedVerificationRollbackState(t, true)
	rollbackAdmission, err := PlanModuleUpdateJobRollbackAdmission(source, admission)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := InitializeModuleUpdateJobRollbackClaimPersistence(source, rollbackAdmission, admission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReopenModuleUpdateJobRollbackClaimPersistence(initialized, admission); err == nil {
		t.Fatal("expected reopen without committed rollback claim rejection")
	}
}
