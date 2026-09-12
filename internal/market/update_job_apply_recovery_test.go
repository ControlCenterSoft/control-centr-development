package market

import "testing"

func TestRecoverModuleUpdateJobApplyPersistenceRunningDeterministicAndSafe(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	first, err := RecoverModuleUpdateJobApplyPersistence(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RecoverModuleUpdateJobApplyPersistence(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("recovery plan is not deterministic: %#v != %#v", first, second)
	}
	if first.NextStep != ModuleUpdateJobApplyRecoveryRevalidateApply || first.ApplyReceiptID != "" {
		t.Fatalf("unexpected running recovery plan: %#v", first)
	}
	if !first.PreMigrationSnapshotEvidenceRequired || !first.PreserveUserData || !first.PreserveSecretMaterial {
		t.Fatalf("running recovery dropped safety evidence: %#v", first)
	}
	if first.ExecutionAuthorized || first.ProductionMutationAllowed {
		t.Fatalf("recovery plan widened authority: %#v", first)
	}
}

func TestRecoverModuleUpdateJobApplyPersistenceVerifyingBindsReceipt(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	committed, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := RecoverModuleUpdateJobApplyPersistence(committed.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NextStep != ModuleUpdateJobApplyRecoveryRevalidateVerification {
		t.Fatalf("unexpected verifying recovery step: %#v", plan)
	}
	if plan.ApplyReceiptID != committed.Receipt.ReceiptID || plan.StateVersion != committed.State.Record.StateVersion {
		t.Fatalf("recovery plan lost apply receipt lineage: %#v", plan)
	}
	if plan.ExecutionAuthorized || plan.ProductionMutationAllowed {
		t.Fatalf("verifying recovery widened authority: %#v", plan)
	}
}

func TestRecoverModuleUpdateJobApplyPersistenceRejectsReceiptDrift(t *testing.T) {
	admission := testApplyAdmission(true)
	state := testClaimedApplyState(t, admission)
	committed, err := CommitModuleUpdateJobApplySuccessCAS(state, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	committed.State.ApplyReceipt.TargetVersion = "9.9.9"
	if _, err := RecoverModuleUpdateJobApplyPersistence(committed.State, admission); err == nil {
		t.Fatal("expected tampered apply receipt rejection")
	}
}

func TestRecoverModuleUpdateJobApplyPersistenceRejectsClaimDrift(t *testing.T) {
	admission := testApplyAdmission(false)
	state := testClaimedApplyState(t, admission)
	state.ClaimJournal.WorkerID = "worker-02"
	if _, err := RecoverModuleUpdateJobApplyPersistence(state, admission); err == nil {
		t.Fatal("expected claim lineage drift rejection")
	}
}

func TestRecoverModuleUpdateJobApplyPersistenceApplicationOnlyDoesNotInventSnapshotRequirement(t *testing.T) {
	admission := testApplyAdmission(false)
	state := testClaimedApplyState(t, admission)
	plan, err := RecoverModuleUpdateJobApplyPersistence(state, admission)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PreMigrationSnapshotEvidenceRequired {
		t.Fatalf("application-only recovery invented snapshot requirement: %#v", plan)
	}
}
