package market

import "testing"

func TestCommitModuleUpdateJobVerificationCASPassedDeterministicAndSafe(t *testing.T) {
	admission, state := testVerificationState(t)
	first, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationPassed, "verify:market-42")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationPassed, "verify:market-42")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("verification completion is not deterministic: %#v != %#v", first, second)
	}
	if first.State.Record.State != ModuleJobStateSucceeded || !first.Receipt.Terminal || first.RollbackRequired {
		t.Fatalf("successful verification has wrong terminal semantics: %#v", first)
	}
	assertVerificationSafety(t, first)
}

func TestCommitModuleUpdateJobVerificationCASFailedRequiresRollback(t *testing.T) {
	admission, state := testVerificationState(t)
	result, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42")
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Record.State != ModuleJobStateRollingBack || result.Receipt.Terminal || !result.RollbackRequired || !result.Receipt.RollbackRequiredNow {
		t.Fatalf("failed verification did not create rollback handoff: %#v", result)
	}
	assertVerificationSafety(t, result)
}

func TestCommitModuleUpdateJobVerificationCASExactReplayIsIdempotent(t *testing.T) {
	admission, state := testVerificationState(t)
	first, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := CommitModuleUpdateJobVerificationCAS(first.State, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42")
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.JournalAppendRequired || replay.State != first.State || replay.Receipt != first.Receipt {
		t.Fatalf("exact replay changed verification evidence: %#v", replay)
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsChangedReplayOutcome(t *testing.T) {
	admission, state := testVerificationState(t)
	first, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationPassed, "verify:market-42")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitModuleUpdateJobVerificationCAS(first.State, admission, 3, ModuleUpdateJobVerificationFailed, "verify:market-42"); err == nil {
		t.Fatal("expected changed replay outcome rejection")
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsChangedReplayEvidence(t *testing.T) {
	admission, state := testVerificationState(t)
	first, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationPassed, "verify:market-42")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitModuleUpdateJobVerificationCAS(first.State, admission, 3, ModuleUpdateJobVerificationPassed, "verify:other"); err == nil {
		t.Fatal("expected changed verification evidence rejection")
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsStaleVersion(t *testing.T) {
	admission, state := testVerificationState(t)
	if _, err := CommitModuleUpdateJobVerificationCAS(state, admission, 2, ModuleUpdateJobVerificationPassed, "verify:market-42"); err == nil {
		t.Fatal("expected stale verification state-version rejection")
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsUnsafeEvidenceIDs(t *testing.T) {
	admission, state := testVerificationState(t)
	for _, evidenceID := range []string{"", " verify:market-42", "../../verify $(id)"} {
		if _, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationPassed, evidenceID); err == nil {
			t.Fatalf("expected unsafe verification evidence %q rejection", evidenceID)
		}
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsApplyReceiptDrift(t *testing.T) {
	admission, state := testVerificationState(t)
	state.ApplyReceipt.BundleSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationPassed, "verify:market-42"); err == nil {
		t.Fatal("expected apply receipt drift rejection")
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsReceiptAuthorityTampering(t *testing.T) {
	admission, state := testVerificationState(t)
	first, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42")
	if err != nil {
		t.Fatal(err)
	}
	tampered := first.State
	tampered.VerificationReceipt.ExecutionAuthorized = true
	if _, err := CommitModuleUpdateJobVerificationCAS(tampered, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42"); err == nil {
		t.Fatal("expected widened verification authority rejection")
	}
}

func TestCommitModuleUpdateJobVerificationCASRejectsRollbackSemanticTampering(t *testing.T) {
	admission, state := testVerificationState(t)
	first, err := CommitModuleUpdateJobVerificationCAS(state, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42")
	if err != nil {
		t.Fatal(err)
	}
	tampered := first.State
	tampered.VerificationReceipt.RollbackRequiredNow = false
	if _, err := CommitModuleUpdateJobVerificationCAS(tampered, admission, 3, ModuleUpdateJobVerificationFailed, "verify:failed-42"); err == nil {
		t.Fatal("expected rollback semantic tampering rejection")
	}
}

func testVerificationState(t *testing.T) (ModuleUpdateJobAdmission, ModuleUpdateJobVerificationPersistenceState) {
	t.Helper()
	admission := testApplyAdmission(true)
	claimed := testClaimedApplyState(t, admission)
	applied, err := CommitModuleUpdateJobApplySuccessCAS(claimed, admission, 2, "snapshot:market-42")
	if err != nil {
		t.Fatal(err)
	}
	state, err := InitializeModuleUpdateJobVerificationPersistence(applied.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	return admission, state
}

func assertVerificationSafety(t *testing.T, result ModuleUpdateJobVerificationCommitResult) {
	t.Helper()
	if result.Replay || !result.JournalAppendRequired {
		t.Fatalf("first verification completion markers are wrong: %#v", result)
	}
	if result.ExecutionAuthorized || result.ProductionMutation || result.Receipt.ExecutionAuthorized || result.Receipt.ProductionMutationAllowed {
		t.Fatalf("verification widened authority: %#v", result)
	}
	if !result.Receipt.PreserveUserData || !result.Receipt.PreserveSecretMaterial {
		t.Fatalf("verification dropped preservation guarantees: %#v", result.Receipt)
	}
	if result.Receipt.FromState != ModuleJobStateVerifying || result.Receipt.FromStateVersion != 3 || result.Receipt.ToStateVersion != 4 || result.Receipt.JournalSequence != 3 {
		t.Fatalf("verification transition lineage is wrong: %#v", result.Receipt)
	}
}
