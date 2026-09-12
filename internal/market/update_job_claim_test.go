package market

import "testing"

func TestPrepareModuleUpdateJobRecordDeterministicAndSafe(t *testing.T) {
	admission := testUpdateAdmission()
	first, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("record not deterministic: %#v != %#v", first, second)
	}
	if first.State != ModuleJobStatePrepared || first.StateVersion != 1 || first.JournalSequence != 0 {
		t.Fatalf("unexpected prepared record: %#v", first)
	}
	if first.ExecutionAuthorized || !first.PreserveUserData || !first.PreserveSecretMaterial {
		t.Fatalf("unsafe prepared record: %#v", first)
	}
}

func TestClaimModuleUpdateJobUsesCASAndJournalsPreparedToRunning(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	result, err := ClaimModuleUpdateJob(record, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replay || !result.JournalAppendRequired {
		t.Fatalf("unexpected first-claim markers: %#v", result)
	}
	if result.Record.State != ModuleJobStateRunning || result.Record.StateVersion != 2 || result.Record.JournalSequence != 1 {
		t.Fatalf("unexpected claimed record: %#v", result.Record)
	}
	if result.Claim.ClaimID == "" || result.Claim.ClaimID != result.Record.ClaimID || result.Journal.ClaimID != result.Claim.ClaimID {
		t.Fatalf("claim evidence not bound: %#v", result)
	}
	if !result.Claim.AtomicPersistenceRequired || result.Claim.ExecutionAuthorized || result.Claim.ProductionMutationAllowed || result.Journal.ExecutionAuthorized {
		t.Fatalf("claim widened execution authority: %#v", result)
	}
	if result.Journal.FromState != ModuleJobStatePrepared || result.Journal.ToState != ModuleJobStateRunning || result.Journal.FromStateVersion != 1 || result.Journal.ToStateVersion != 2 {
		t.Fatalf("bad claim journal: %#v", result.Journal)
	}
}

func TestClaimModuleUpdateJobExactReplayIsIdempotent(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	first, err := ClaimModuleUpdateJob(record, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ClaimModuleUpdateJob(first.Record, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replay || replayed.JournalAppendRequired {
		t.Fatalf("replay should not append journal: %#v", replayed)
	}
	if replayed.Record != first.Record || replayed.Claim != first.Claim || replayed.Journal != first.Journal {
		t.Fatalf("exact replay changed evidence: first=%#v replay=%#v", first, replayed)
	}
}

func TestClaimModuleUpdateJobRejectsStaleVersionAndCompetingWorker(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	if _, err := ClaimModuleUpdateJob(record, admission, "worker-01", 2); err == nil {
		t.Fatal("expected stale prepared CAS rejection")
	}
	first, err := ClaimModuleUpdateJob(record, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ClaimModuleUpdateJob(first.Record, admission, "worker-02", 1); err == nil {
		t.Fatal("expected competing worker replay rejection")
	}
	if _, err := ClaimModuleUpdateJob(first.Record, admission, "worker-01", 2); err == nil {
		t.Fatal("expected already-claimed version rejection")
	}
}

func TestClaimModuleUpdateJobRejectsAdmissionAndRecordDrift(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	tamperedAdmission := admission
	tamperedAdmission.BundleID = "market-update-bundle:tampered"
	if _, err := ClaimModuleUpdateJob(record, tamperedAdmission, "worker-01", 1); err == nil {
		t.Fatal("expected admission drift rejection")
	}
	tamperedRecord := record
	tamperedRecord.AdmissionID = "market-update-admission:tampered"
	if _, err := ClaimModuleUpdateJob(tamperedRecord, admission, "worker-01", 1); err == nil {
		t.Fatal("expected durable record drift rejection")
	}
}

func TestClaimModuleUpdateJobRejectsMigrationEvidenceDrift(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	drifted := admission
	drifted.MigrationCount++
	drifted.AdmissionID = moduleUpdateJobAdmissionID(drifted)
	if _, err := ClaimModuleUpdateJob(record, drifted, "worker-01", 1); err == nil {
		t.Fatal("expected migration evidence drift rejection")
	}
}

func TestClaimModuleUpdateJobRejectsUnsafeAdmissionAndWorkerIdentity(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	unsafe := admission
	unsafe.ExecutionAuthorized = true
	unsafe.AdmissionID = moduleUpdateJobAdmissionID(unsafe)
	if _, err := ClaimModuleUpdateJob(record, unsafe, "worker-01", 1); err == nil {
		t.Fatal("expected execution-authorized admission rejection")
	}
	if _, err := ClaimModuleUpdateJob(record, admission, "../../shell $(id)", 1); err == nil {
		t.Fatal("expected unsafe worker identity rejection")
	}
}

func TestClaimModuleUpdateJobRejectsTamperedPersistedClaimEvidence(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	first, err := ClaimModuleUpdateJob(record, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	tampered := first.Record
	tampered.ClaimID = "market-update-claim:tampered"
	if _, err := ClaimModuleUpdateJob(tampered, admission, "worker-01", 1); err == nil {
		t.Fatal("expected persisted claim evidence rejection")
	}
}

func TestClaimModuleUpdateJobRejectsUnsupportedState(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	record.State = ModuleJobStateVerifying
	if _, err := ClaimModuleUpdateJob(record, admission, "worker-01", 1); err == nil {
		t.Fatal("expected non-claimable state rejection")
	}
}

func testUpdateAdmission() ModuleUpdateJobAdmission {
	admission := ModuleUpdateJobAdmission{
		JobID:                       "market-job:fixture",
		LifecycleIdempotencyKey:     "market-lifecycle:fixture",
		BundleID:                    "market-update-bundle:fixture",
		BundleIdempotencyKey:        "market-update:fixture",
		ModuleID:                    "inventory-agent",
		CurrentVersion:              "1.4.0",
		TargetVersion:               "1.5.0",
		Generation:                  7,
		BundleSHA256:                "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BundleSizeBytes:             4096,
		MigrationCount:              1,
		RollbackMode:                "RESTORE_PREVIOUS_VERSION_AND_DATA_SNAPSHOT",
		RequirePreMigrationSnapshot: true,
		PreserveUserData:            true,
		PreserveSecretMaterial:      true,
		ExecutionAuthorized:         false,
	}
	admission.AdmissionID = moduleUpdateJobAdmissionID(admission)
	return admission
}
