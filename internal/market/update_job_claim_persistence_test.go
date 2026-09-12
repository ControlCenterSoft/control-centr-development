package market

import "testing"

func TestInitializeModuleUpdateJobClaimPersistenceSafePreparedState(t *testing.T) {
	admission := testUpdateAdmission()
	record, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	state, err := InitializeModuleUpdateJobClaimPersistence(record, admission)
	if err != nil {
		t.Fatal(err)
	}
	if state.Record != record || state.HasJournal || state.Journal != (ModuleUpdateJobClaimJournalEntry{}) {
		t.Fatalf("unexpected prepared persistence state: %#v", state)
	}
}

func TestCommitModuleUpdateJobClaimCASProducesAtomicRecoverableState(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)

	committed, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Replay || !committed.JournalAppendRequired {
		t.Fatalf("unexpected first commit markers: %#v", committed)
	}
	if !committed.State.HasJournal || committed.State.Record.State != ModuleJobStateRunning {
		t.Fatalf("record and journal were not committed together: %#v", committed.State)
	}
	if committed.State.Record.ClaimID != committed.Claim.ClaimID || committed.State.Journal.ClaimID != committed.Claim.ClaimID {
		t.Fatalf("claim persistence evidence mismatch: %#v", committed)
	}
	if committed.ExecutionAuthorized || committed.ProductionMutation || committed.Claim.ExecutionAuthorized || committed.Claim.ProductionMutationAllowed {
		t.Fatalf("claim persistence widened authority: %#v", committed)
	}

	reopened, err := ReopenModuleUpdateJobClaimPersistence(committed.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Record != committed.State.Record || reopened.Claim != committed.Claim || reopened.Journal != committed.State.Journal {
		t.Fatalf("reopen changed persisted evidence: %#v != %#v", reopened, committed)
	}
}

func TestCommitModuleUpdateJobClaimCASExactReplayIsIdempotent(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)
	first, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitModuleUpdateJobClaimCAS(first.State, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replay || second.JournalAppendRequired {
		t.Fatalf("exact replay must not append a second journal: %#v", second)
	}
	if second.State != first.State || second.Claim != first.Claim {
		t.Fatalf("exact replay changed durable evidence: first=%#v second=%#v", first, second)
	}
}

func TestCommitModuleUpdateJobClaimCASCrashBeforeCommitCanRetry(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)
	first, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash before the caller atomically persists first.State: retry
	// from the unchanged PREPARED durable state must reproduce exact evidence.
	retried, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != first.State || retried.Claim != first.Claim {
		t.Fatalf("retry after pre-commit crash was not deterministic: first=%#v retry=%#v", first, retried)
	}
}

func TestCommitModuleUpdateJobClaimCASRejectsStaleAndCompetingWorker(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)
	if _, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 2); err == nil {
		t.Fatal("expected stale CAS rejection")
	}
	first, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitModuleUpdateJobClaimCAS(first.State, admission, "worker-02", 1); err == nil {
		t.Fatal("expected competing worker rejection")
	}
}

func TestCommitModuleUpdateJobClaimCASRejectsTornRecordOnlyWrite(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)
	first, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	torn := ModuleUpdateJobClaimPersistenceState{Record: first.State.Record}
	if _, err := CommitModuleUpdateJobClaimCAS(torn, admission, "worker-01", 1); err == nil {
		t.Fatal("expected torn RUNNING record without journal rejection")
	}
	if _, err := ReopenModuleUpdateJobClaimPersistence(torn, admission); err == nil {
		t.Fatal("expected torn RUNNING record reopen rejection")
	}
}

func TestCommitModuleUpdateJobClaimCASRejectsJournalBeforeRecordWrite(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)
	first, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	torn := ModuleUpdateJobClaimPersistenceState{
		Record:     state.Record,
		Journal:    first.State.Journal,
		HasJournal: true,
	}
	if _, err := CommitModuleUpdateJobClaimCAS(torn, admission, "worker-01", 1); err == nil {
		t.Fatal("expected PREPARED state with journal rejection")
	}
}

func TestReopenModuleUpdateJobClaimPersistenceRejectsTamperAndAdmissionDrift(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	state, _ := InitializeModuleUpdateJobClaimPersistence(record, admission)
	first, err := CommitModuleUpdateJobClaimCAS(state, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}

	tampered := first.State
	tampered.Journal.JournalID = "market-update-claim-journal:tampered"
	if _, err := ReopenModuleUpdateJobClaimPersistence(tampered, admission); err == nil {
		t.Fatal("expected tampered journal rejection")
	}

	drifted := admission
	drifted.MigrationCount++
	drifted.AdmissionID = moduleUpdateJobAdmissionID(drifted)
	if _, err := ReopenModuleUpdateJobClaimPersistence(first.State, drifted); err == nil {
		t.Fatal("expected admission drift rejection")
	}
}
