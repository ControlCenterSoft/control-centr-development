package market

import (
	"strings"
	"testing"
)

func TestRollbackRecoveryDecisionClosesDefinitelyNotCommittedWithoutAuthority(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, false)
	reconciliation, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		source,
		admission,
		ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}

	decision, err := DecideModuleUpdateJobRollbackRecovery(
		reconciliation,
		"operator-1",
		"rollback is no longer required",
		ModuleRollbackRecoveryClose,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.RecoveryClosed || decision.RequiresFreshAdmissionLineage ||
		decision.NextAction != "none" {
		t.Fatalf("close decision did not remain terminal: %#v", decision)
	}
	if decision.ReconciliationID != reconciliation.ReconciliationID ||
		decision.AttemptID != reconciliation.AttemptID ||
		decision.RollbackAdmissionID != reconciliation.RollbackAdmissionID ||
		decision.RecordID != reconciliation.RecordID ||
		decision.JobID != reconciliation.JobID ||
		decision.ModuleID != reconciliation.ModuleID {
		t.Fatalf("recovery decision lost reconciliation lineage: %#v", decision)
	}
	assertRollbackRecoveryDecisionNonAuthorizing(t, decision)
}

func TestRollbackRecoveryDecisionRequestsOnlyFreshAdmissionRevalidation(t *testing.T) {
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
	reconciliation, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		observed,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.Outcome != ModuleUpdateJobRollbackReconciliationAmbiguous {
		t.Fatalf("test requires ambiguous source, got %#v", reconciliation)
	}

	var decisionID string
	for i := 0; i < 100; i++ {
		decision, err := DecideModuleUpdateJobRollbackRecovery(
			reconciliation,
			"operator-2",
			"revalidate current durable evidence before a new admission",
			ModuleRollbackRecoveryRevalidate,
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision.RecoveryClosed || !decision.RequiresFreshAdmissionLineage ||
			decision.NextAction != "revalidate-fresh-rollback-admission" {
			t.Fatalf("fresh-lineage boundary not preserved: %#v", decision)
		}
		assertRollbackRecoveryDecisionNonAuthorizing(t, decision)
		if i == 0 {
			decisionID = decision.DecisionID
		}
		if decision.DecisionID != decisionID {
			t.Fatalf("non-deterministic recovery decision id: %s != %s", decision.DecisionID, decisionID)
		}
	}
}

func TestRollbackRecoveryDecisionRejectsCommittedReconciliation(t *testing.T) {
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
	reconciliation, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		commit.State,
		admission,
		ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DecideModuleUpdateJobRollbackRecovery(
		reconciliation,
		"operator-3",
		"must not reopen terminal rollback",
		ModuleRollbackRecoveryRevalidate,
	); err == nil {
		t.Fatal("expected committed reconciliation to remain terminal")
	}
}

func TestRollbackRecoveryDecisionRejectsTamperedReconciliation(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, false)
	reconciliation, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		source,
		admission,
		ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(*ModuleUpdateJobRollbackReconciliationReceipt)
	}{
		{"identity", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.Reason = "tampered" }},
		{"retry-authority", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.AutomaticRetryAuthorized = true }},
		{"execution-authority", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.ExecutionAuthorized = true }},
		{"generic-command", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.GenericCommandAuthorized = true }},
		{"production-mutation", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.ProductionMutationAllowed = true }},
		{"user-data", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.PreserveUserData = false }},
		{"secret-material", func(r *ModuleUpdateJobRollbackReconciliationReceipt) { r.PreserveSecretMaterial = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tampered := reconciliation
			tc.mutate(&tampered)
			if _, err := DecideModuleUpdateJobRollbackRecovery(
				tampered,
				"operator-4",
				"reject tampered evidence",
				ModuleRollbackRecoveryClose,
			); err == nil {
				t.Fatalf("expected %s tampering rejection", tc.name)
			}
		})
	}
}

func TestRollbackRecoveryDecisionRejectsUnboundedOrAmbiguousAuditInput(t *testing.T) {
	admission, source := testRollbackCompletionPersistence(t, false)
	reconciliation, err := ReconcileModuleUpdateJobRollbackCommit(
		source,
		source,
		admission,
		ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		operator string
		reason   string
		decision ModuleUpdateJobRollbackRecoveryDecision
	}{
		{"blank-operator", "", "reason", ModuleRollbackRecoveryClose},
		{"trimmed-operator", " operator", "reason", ModuleRollbackRecoveryClose},
		{"control-operator", "operator\n2", "reason", ModuleRollbackRecoveryClose},
		{"blank-reason", "operator", "", ModuleRollbackRecoveryClose},
		{"long-reason", "operator", strings.Repeat("x", 513), ModuleRollbackRecoveryClose},
		{"unknown-decision", "operator", "reason", ModuleUpdateJobRollbackRecoveryDecision("RETRY")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecideModuleUpdateJobRollbackRecovery(
				reconciliation,
				tc.operator,
				tc.reason,
				tc.decision,
			); err == nil {
				t.Fatalf("expected %s rejection", tc.name)
			}
		})
	}
}

func assertRollbackRecoveryDecisionNonAuthorizing(
	t *testing.T,
	decision ModuleUpdateJobRollbackRecoveryDecisionReceipt,
) {
	t.Helper()
	if decision.PreviousAttemptReusable || decision.FreshRollbackAdmissionAuthorized ||
		decision.FurtherAttemptAuthorized || decision.AutomaticRetryAuthorized ||
		decision.ExecutionAuthorized || decision.GenericCommandAuthorized ||
		decision.ProductionMutationAllowed || !decision.PreserveUserData ||
		!decision.PreserveSecretMaterial {
		t.Fatalf("recovery decision widened authority or lost protected data: %#v", decision)
	}
}
