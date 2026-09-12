package market

import "testing"

func TestCommitModuleUpdateJobApplySuccessCASDeterministicAndSafe(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	first, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("apply completion is not deterministic: %#v != %#v", first, second)
	}
	if first.Replay || !first.JournalAppendRequired {
		t.Fatalf("first apply completion markers are wrong: %#v", first)
	}
	if first.State.Record.State != ModuleJobStateVerifying ||
		first.State.Record.StateVersion != 3 ||
		first.State.Record.JournalSequence != 2 {
		t.Fatalf("unexpected verifying record: %#v", first.State.Record)
	}
	if first.Receipt.FromState != ModuleJobStateRunning ||
		first.Receipt.ToState != ModuleJobStateVerifying ||
		first.Receipt.FromStateVersion != 2 ||
		first.Receipt.ToStateVersion != 3 ||
		first.Receipt.JournalSequence != 2 {
		t.Fatalf("unexpected apply receipt transition: %#v", first.Receipt)
	}
	if first.ExecutionAuthorized ||
		first.ProductionMutation ||
		first.Receipt.ExecutionAuthorized ||
		first.Receipt.ProductionMutationAllowed {
		t.Fatalf("apply receipt widened authority: %#v", first)
	}
	if !first.Receipt.PreserveUserData ||
		!first.Receipt.PreserveSecretMaterial ||
		!first.Receipt.VerificationRequired ||
		!first.Receipt.RollbackRequiredOnFailure {
		t.Fatalf("apply receipt dropped safety requirements: %#v", first.Receipt)
	}
}

func TestCommitModuleUpdateJobApplySuccessCASExactReplayIsIdempotent(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	first, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := CommitModuleUpdateJobApplySuccessCAS(first.State, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replay || replayed.JournalAppendRequired {
		t.Fatalf("exact replay should not append evidence: %#v", replayed)
	}
	if replayed.State != first.State || replayed.Receipt != first.Receipt {
		t.Fatalf("exact replay changed evidence: %#v != %#v", replayed, first)
	}
}

func TestCommitModuleUpdateJobApplySuccessCASRequiresMigrationSnapshot(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	for _, snapshotID := range []string{"", " snapshot:market-42", "../../snapshot $(id)"} {
		if _, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, snapshotID); err == nil {
			t.Fatalf("expected unsafe snapshot %q rejection", snapshotID)
		}
	}
}

func TestCommitModuleUpdateJobApplySuccessCASApplicationOnlyRejectsSnapshotEvidence(t *testing.T) {
	admission := testApplyAdmission(false)
	state := testClaimedApplyState(t, admission)
	if _, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:not-required"); err == nil {
		t.Fatal("expected application-only update to reject unrelated snapshot evidence")
	}
	result, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.PreMigrationSnapshotID != "" || result.Receipt.RequirePreMigrationSnapshot {
		t.Fatalf("application-only update invented migration snapshot evidence: %#v", result.Receipt)
	}
}

func TestCommitModuleUpdateJobApplySuccessCASRejectsStaleVersion(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	if _, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 1, "snapshot:market-42"); err == nil {
		t.Fatal("expected stale state-version rejection")
	}
}

func TestCommitModuleUpdateJobApplySuccessCASRejectsClaimJournalDrift(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	state.ClaimJournal.WorkerID = "worker-02"
	if _, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42"); err == nil {
		t.Fatal("expected claim journal drift rejection")
	}
}

func TestCommitModuleUpdateJobApplySuccessCASRejectsAdmissionDrift(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	drifted := admission
	drifted.BundleSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	drifted.AdmissionID = "market-update-admission:drifted"
	if _, err := CommitModuleUpdateJobApplySuccessCAS(state, drifted, 2, "snapshot:market-42"); err == nil {
		t.Fatal("expected admission drift rejection")
	}
}

func TestCommitModuleUpdateJobApplySuccessCASRejectsPersistedReceiptTampering(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	first, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	tampered := first.State
	tampered.ApplyReceipt.TargetVersion = "9.9.9"
	if _, err := CommitModuleUpdateJobApplySuccessCAS(tampered, admission, 2, "snapshot:market-42"); err == nil {
		t.Fatal("expected persisted apply receipt tampering rejection")
	}
}

func TestCommitModuleUpdateJobApplySuccessCASRejectsUnprovenState(t *testing.T) {
	admission := testApplyAdmission(true)
	record, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	state := ModuleUpdateJobApplyPersistenceState{Record: record}
	if _, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 1, "snapshot:market-42"); err == nil {
		t.Fatal("expected PREPARED state rejection")
	}
}

func testApplyAdmission(withMigration bool) ModuleUpdateJobAdmission {
	admission := ModuleUpdateJobAdmission{
		JobID:                   "market-job:apply-fixture",
		LifecycleIdempotencyKey: "market-lifecycle:apply-fixture",
		BundleID:                "market-update-bundle:apply-fixture",
		BundleIdempotencyKey:    "market-update:apply-fixture",
		ModuleID:                "inventory-agent",
		CurrentVersion:          "1.4.0",
		TargetVersion:           "1.5.0",
		Generation:              7,
		BundleSHA256:            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BundleSizeBytes:         4096,
		RollbackMode:            "RESTORE_PREVIOUS_VERSION",
		PreserveUserData:        true,
		PreserveSecretMaterial:  true,
		ExecutionAuthorized:     false,
	}
	if withMigration {
		admission.MigrationCount = 1
		admission.RequirePreMigrationSnapshot = true
		admission.RollbackMode = "RESTORE_PREVIOUS_VERSION_AND_DATA_SNAPSHOT"
	}
	admission.AdmissionID = moduleUpdateJobAdmissionID(admission)
	return admission
}

func testClaimedApplyState(t *testing.T, admission ModuleUpdateJobAdmission) ModuleUpdateJobApplyPersistenceState {
	t.Helper()
	record, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := ClaimModuleUpdateJob(record, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}
	state, err := InitializeModuleUpdateJobApplyPersistence(claimed.Record, admission, claimed.Journal)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
