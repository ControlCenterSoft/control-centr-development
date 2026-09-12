package market

import "testing"

func TestRecoverPersistedModuleUpdateJobClaimExactReplay(t *testing.T) {
	admission := testUpdateAdmission()
	record, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := ClaimModuleUpdateJob(record, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := RecoverPersistedModuleUpdateJobClaim(claimed.Record, admission, claimed.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Record != claimed.Record || recovered.Claim != claimed.Claim || recovered.Journal != claimed.Journal {
		t.Fatalf("recovery changed persisted evidence: claimed=%#v recovered=%#v", claimed, recovered)
	}
	if !recovered.Replay || recovered.JournalAppendRequired {
		t.Fatalf("recovery must be replay-only: %#v", recovered)
	}
	if recovered.Claim.ExecutionAuthorized || recovered.Claim.ProductionMutationAllowed || recovered.Journal.ExecutionAuthorized {
		t.Fatalf("recovery widened execution authority: %#v", recovered)
	}
}

func TestRecoverPersistedModuleUpdateJobClaimDeterministic(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	claimed, err := ClaimModuleUpdateJob(record, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}
	first, err := RecoverPersistedModuleUpdateJobClaim(claimed.Record, admission, claimed.Journal)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		next, err := RecoverPersistedModuleUpdateJobClaim(claimed.Record, admission, claimed.Journal)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("recovery is not deterministic: first=%#v next=%#v", first, next)
		}
	}
}

func TestRecoverPersistedModuleUpdateJobClaimRejectsJournalDrift(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	claimed, err := ClaimModuleUpdateJob(record, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*ModuleUpdateJobClaimJournalEntry){
		"journal id": func(entry *ModuleUpdateJobClaimJournalEntry) {
			entry.JournalID = "market-update-claim-journal:tampered"
		},
		"sequence":  func(entry *ModuleUpdateJobClaimJournalEntry) { entry.Sequence++ },
		"worker":    func(entry *ModuleUpdateJobClaimJournalEntry) { entry.WorkerID = "worker-02" },
		"claim id":  func(entry *ModuleUpdateJobClaimJournalEntry) { entry.ClaimID = "market-update-claim:tampered" },
		"authority": func(entry *ModuleUpdateJobClaimJournalEntry) { entry.ExecutionAuthorized = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			journal := claimed.Journal
			mutate(&journal)
			if _, err := RecoverPersistedModuleUpdateJobClaim(claimed.Record, admission, journal); err == nil {
				t.Fatalf("expected %s drift rejection", name)
			}
		})
	}
}

func TestRecoverPersistedModuleUpdateJobClaimRejectsRecordAndAdmissionDrift(t *testing.T) {
	admission := testUpdateAdmission()
	record, _ := PrepareModuleUpdateJobRecord(admission)
	claimed, err := ClaimModuleUpdateJob(record, admission, "worker-01", record.StateVersion)
	if err != nil {
		t.Fatal(err)
	}

	tamperedRecord := claimed.Record
	tamperedRecord.ClaimID = "market-update-claim:tampered"
	if _, err := RecoverPersistedModuleUpdateJobClaim(tamperedRecord, admission, claimed.Journal); err == nil {
		t.Fatal("expected tampered record rejection")
	}

	driftedAdmission := admission
	driftedAdmission.MigrationCount++
	driftedAdmission.AdmissionID = moduleUpdateJobAdmissionID(driftedAdmission)
	if _, err := RecoverPersistedModuleUpdateJobClaim(claimed.Record, driftedAdmission, claimed.Journal); err == nil {
		t.Fatal("expected admission drift rejection")
	}
}

func TestRecoverPersistedModuleUpdateJobClaimRejectsPreparedState(t *testing.T) {
	admission := testUpdateAdmission()
	record, err := PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverPersistedModuleUpdateJobClaim(record, admission, ModuleUpdateJobClaimJournalEntry{}); err == nil {
		t.Fatal("expected recovery of unclaimed PREPARED job to be rejected")
	}
}
