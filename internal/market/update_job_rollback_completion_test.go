package market

import "testing"

func testRollbackCompletionPersistence(
	t *testing.T,
	withMigration bool,
) (ModuleUpdateJobAdmission, ModuleUpdateJobRollbackPersistenceState) {
	t.Helper()
	admission, claimState := testCommittedRollbackClaim(t, withMigration)
	attempt, err := AdmitModuleUpdateJobRollbackAttempt(claimState, admission)
	if err != nil {
		t.Fatal(err)
	}
	state, err := InitializeModuleUpdateJobRollbackPersistence(claimState, attempt, admission)
	if err != nil {
		t.Fatal(err)
	}
	return admission, state
}

func TestRollbackCompletionSuccessIsAtomicTerminalAndReplaySafe(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, true)
	version := state.Attempt.ExpectedStateVersion
	journal := state.Attempt.ExpectedJournalSequence
	first, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.AttemptConsumptionRequired || !first.JournalAppendRequired || first.RecoveryRequired {
		t.Fatalf("bad first result: %#v", first)
	}
	if first.State.Record.State != ModuleJobStateRolledBack ||
		first.State.Record.StateVersion != version+1 ||
		first.State.Record.JournalSequence != state.Attempt.AttemptSequence {
		t.Fatalf("bad terminal record: %#v", first.State.Record)
	}
	r := first.Receipt
	if !r.AttemptConsumed || !r.AtomicPersistenceRequired || !r.Terminal || r.RecoveryRequired ||
		r.FurtherAttemptAuthorized || r.AutomaticRetryAuthorized || r.ExecutionAuthorized ||
		r.GenericCommandAuthorized || r.ProductionMutationAllowed {
		t.Fatalf("unsafe receipt: %#v", r)
	}
	if r.RestoreVersion != state.Attempt.RestoreVersion ||
		r.PreMigrationSnapshotID != state.Attempt.PreMigrationSnapshotID ||
		r.ReceiptID == "" {
		t.Fatalf("lost restore lineage: %#v", r)
	}
	replay, err := CommitModuleUpdateJobRollbackCAS(
		first.State,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.AttemptConsumptionRequired || replay.JournalAppendRequired || replay.Receipt != r {
		t.Fatalf("replay not idempotent: %#v", replay)
	}
}

func TestRollbackCompletionFailureRequiresRecoveryWithoutRetryAuthority(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, false)
	result, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		state.Attempt.ExpectedStateVersion,
		state.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Record.State != ModuleJobStateFailed ||
		!result.RecoveryRequired || !result.Receipt.RecoveryRequired {
		t.Fatalf("failed rollback semantics: %#v", result)
	}
	if result.FurtherAttemptAuthorized || result.AutomaticRetryAuthorized ||
		result.ExecutionAuthorized || result.ProductionMutationAllowed {
		t.Fatalf("failed rollback widened authority: %#v", result)
	}
	if result.Receipt.PreMigrationSnapshotID != "" {
		t.Fatalf("application-only rollback invented snapshot: %#v", result.Receipt)
	}
}

func TestRollbackCompletionRejectsRevisionDrift(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, true)
	version := state.Attempt.ExpectedStateVersion
	journal := state.Attempt.ExpectedJournalSequence
	if _, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version+1,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected state drift rejection")
	}
	if _, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version,
		journal+1,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected journal drift rejection")
	}
	state.Record.StateVersion++
	if _, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected durable record drift rejection")
	}
}

func TestRollbackCompletionRejectsAttemptAndReceiptTampering(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, true)
	version := state.Attempt.ExpectedStateVersion
	journal := state.Attempt.ExpectedJournalSequence
	state.Attempt.RestoreVersion = state.Attempt.FailedTargetVersion
	if _, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected attempt lineage rejection")
	}

	admission, state = testRollbackCompletionPersistence(t, true)
	version = state.Attempt.ExpectedStateVersion
	journal = state.Attempt.ExpectedJournalSequence
	result, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	result.State.Receipt.AutomaticRetryAuthorized = true
	if _, err := CommitModuleUpdateJobRollbackCAS(
		result.State,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected receipt authority rejection")
	}
}

func TestRollbackCompletionRejectsOutcomeChangeOnReplay(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, true)
	version := state.Attempt.ExpectedStateVersion
	journal := state.Attempt.ExpectedJournalSequence
	result, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitModuleUpdateJobRollbackCAS(
		result.State,
		admission,
		version,
		journal,
		ModuleUpdateJobRollbackFailed,
	); err == nil {
		t.Fatal("expected changed replay outcome rejection")
	}
}

func TestRollbackCompletionRejectsUnknownOutcome(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, true)
	if _, err := CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		state.Attempt.ExpectedStateVersion,
		state.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackOutcome("UNKNOWN"),
	); err == nil {
		t.Fatal("expected unknown rollback outcome rejection")
	}
}

func TestRollbackReceiptIdentityIsDeterministic(t *testing.T) {
	admission, state := testRollbackCompletionPersistence(t, true)
	version := state.Attempt.ExpectedStateVersion
	journal := state.Attempt.ExpectedJournalSequence
	var id string
	for i := 0; i < 100; i++ {
		result, err := CommitModuleUpdateJobRollbackCAS(
			state,
			admission,
			version,
			journal,
			ModuleUpdateJobRollbackSucceeded,
		)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			id = result.Receipt.ReceiptID
		}
		if result.Receipt.ReceiptID != id {
			t.Fatalf("non-deterministic id: %s != %s", result.Receipt.ReceiptID, id)
		}
	}
}
