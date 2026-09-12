package market

import "testing"

func TestRollbackCompletionReconciliationConfirmsCommitted(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	commit, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		commit.State,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != ModuleUpdateJobRollbackReconciliationCommitted ||
		!receipt.TerminalEvidenceConfirmed || receipt.FreshDecisionRequired {
		t.Fatalf("committed rollback was not confirmed: %#v", receipt)
	}
	if receipt.ExpectedReceiptID != commit.Receipt.ReceiptID ||
		receipt.ObservedReceiptID != commit.Receipt.ReceiptID {
		t.Fatalf("receipt lineage lost: %#v", receipt)
	}
	assertRollbackReconciliationNonAuthorizing(t, receipt)
}

func TestRollbackCompletionReconciliationProvesDefinitelyNotCommitted(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, false)
	receipt, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		source,
		admission,
		ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != ModuleUpdateJobRollbackReconciliationDefinitelyNotCommitted ||
		receipt.TerminalEvidenceConfirmed || !receipt.FreshDecisionRequired {
		t.Fatalf("unchanged pre-CAS state not classified safely: %#v", receipt)
	}
	if receipt.ObservedReceiptID != "" || receipt.Reason != "exact_precommit_revision" {
		t.Fatalf("unexpected definitely-not-committed evidence: %#v", receipt)
	}
	assertRollbackReconciliationNonAuthorizing(t, receipt)
}

func TestRollbackCompletionReconciliationTreatsMissingReceiptAfterStateAdvanceAsAmbiguous(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	commit, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	observed := commit.State
	observed.HasReceipt = false
	observed.Receipt = ModuleUpdateJobRollbackReceipt{}

	receipt, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		observed,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != ModuleUpdateJobRollbackReconciliationAmbiguous ||
		receipt.TerminalEvidenceConfirmed || !receipt.FreshDecisionRequired {
		t.Fatalf("partial durable write must be ambiguous: %#v", receipt)
	}
	assertRollbackReconciliationNonAuthorizing(t, receipt)
}

func TestRollbackCompletionReconciliationTreatsDifferentOutcomeAsAmbiguous(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	commit, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		commit.State,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != ModuleUpdateJobRollbackReconciliationAmbiguous ||
		!receipt.FreshDecisionRequired {
		t.Fatalf("changed outcome must be ambiguous: %#v", receipt)
	}
	assertRollbackReconciliationNonAuthorizing(t, receipt)
}

func TestRollbackCompletionReconciliationRejectsCrossLineageState(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	observed := source
	observed.Attempt.JobID = "job-other"
	if _, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		observed,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected cross-lineage attempt rejection")
	}

	observed = source
	observed.Record.RecordID = "record-other"
	if _, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		observed,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected cross-lineage durable record rejection")
	}
}

func TestRollbackCompletionReconciliationRejectsTerminalSource(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	commit, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileModuleUpdateJobRollbackCommit(
		commit.State,
		commit.State,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected terminal source rejection")
	}
}

func TestRollbackCompletionReconciliationReceiptIdentityIsDeterministic(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	commit, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}

	var id string
	for i := 0; i < 100; i++ {
		receipt, err := ReconcileModuleUpdateJobRollbackCommit(
			source,
			commit.State,
			admission,
			ModuleUpdateJobRollbackSucceeded,
		)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			id = receipt.ReconciliationID
		}
		if receipt.ReconciliationID != id {
			t.Fatalf("non-deterministic reconciliation id: %s != %s", receipt.ReconciliationID, id)
		}
	}
}

func TestRollbackCompletionReconciliationNeverConvertsTamperingIntoAuthority(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, true)
	commit, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	observed := commit.State
	observed.Receipt.AutomaticRetryAuthorized = true

	receipt, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		observed,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != ModuleUpdateJobRollbackReconciliationAmbiguous {
		t.Fatalf("tampered terminal receipt must be ambiguous: %#v", receipt)
	}
	assertRollbackReconciliationNonAuthorizing(t, receipt)
}

func assertRollbackReconciliationNonAuthorizing(
	t *testing.T,
	receipt ModuleUpdateJobRollbackReconciliationReceipt,
) {
	t.Helper()
	if !receipt.PreserveUserData || !receipt.PreserveSecretMaterial ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.ExecutionAuthorized || receipt.GenericCommandAuthorized ||
		receipt.ProductionMutationAllowed {
		t.Fatalf("reconciliation widened authority: %#v", receipt)
	}
}
