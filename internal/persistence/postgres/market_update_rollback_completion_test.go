package postgres

import (
	"context"
	"errors"
	"testing"

	"control-center/internal/market"
)

func TestMarketUpdateRollbackCompletionRepositoryAtomicCommitReplayAndReopen(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-inventory")
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: initial}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}

	first, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.AttemptConsumptionRequired || !first.JournalAppendRequired ||
		!first.State.HasReceipt || backend.persistCalls != 1 {
		t.Fatalf("first rollback completion is not one atomic consumption: %#v", first)
	}
	assertRollbackCompletionRepositorySafety(t, first)

	replay, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.AttemptConsumptionRequired || replay.JournalAppendRequired ||
		replay.State != first.State || backend.persistCalls != 1 {
		t.Fatalf("rollback completion replay changed durable evidence: %#v", replay)
	}

	reopened, err := repository.Reopen(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Replay || reopened.State != first.State || reopened.Receipt != first.Receipt {
		t.Fatalf("restart reopen changed rollback completion evidence: %#v", reopened)
	}
	assertRollbackCompletionRepositorySafety(t, reopened)
}

func TestMarketUpdateRollbackCompletionRepositoryFailedRollbackRequiresRecovery(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-dns")
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: initial}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}

	result, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackFailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Record.State != market.ModuleJobStateFailed || !result.RecoveryRequired ||
		!result.Receipt.RecoveryRequired {
		t.Fatalf("failed rollback lost recovery semantics: %#v", result)
	}
	assertRollbackCompletionRepositorySafety(t, result)
}

func TestMarketUpdateRollbackCompletionRepositoryRejectsStaleOrChangedOutcome(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-backup")
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: initial}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion+1,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected stale state-version rejection")
	}
	if backend.persistCalls != 0 || backend.state != initial {
		t.Fatal("stale completion modified durable state")
	}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackFailed,
	); err == nil {
		t.Fatal("expected changed terminal outcome rejection")
	}
}

func TestMarketUpdateRollbackCompletionRepositoryTransactionFailureDoesNotConsumeAttempt(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-audit")
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: initial, failCommit: true}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected simulated transaction commit failure")
	}
	if backend.state != initial {
		t.Fatalf("failed transaction consumed rollback attempt: %#v", backend.state)
	}

	backend.failCommit = false
	result, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replay || !result.State.HasReceipt {
		t.Fatalf("retry after uncommitted transaction was not a fresh deterministic commit: %#v", result)
	}
}

func TestMarketUpdateRollbackCompletionRepositoryPersistenceFailureRollsBack(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-metrics")
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: initial, failPersist: true}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	); err == nil {
		t.Fatal("expected simulated completion persistence failure")
	}
	if backend.state != initial || backend.state.HasReceipt {
		t.Fatalf("persistence failure leaked torn terminal state: %#v", backend.state)
	}
}

func TestMarketUpdateRollbackCompletionRepositoryReconcilesLostResponse(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-reconcile")
	expected, err := market.CommitModuleUpdateJobRollbackCAS(
		initial,
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, observed := range map[string]market.ModuleUpdateJobRollbackPersistenceState{
		"committed":                expected.State,
		"definitely-not-committed": initial,
	} {
		t.Run(name, func(t *testing.T) {
			backend := &fakeMarketUpdateRollbackCompletionBackend{state: observed}
			repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}
			reconciliation, err := repository.Reconcile(
				context.Background(),
				admission,
				market.ModuleUpdateJobRollbackSucceeded,
			)
			if err != nil {
				t.Fatal(err)
			}
			if reconciliation.FurtherAttemptAuthorized || reconciliation.AutomaticRetryAuthorized ||
				reconciliation.ExecutionAuthorized || reconciliation.GenericCommandAuthorized ||
				reconciliation.ProductionMutationAllowed {
				t.Fatalf("reconciliation widened authority: %#v", reconciliation)
			}
			if name == "committed" {
				if reconciliation.Outcome != market.ModuleUpdateJobRollbackReconciliationCommitted ||
					!reconciliation.TerminalEvidenceConfirmed || reconciliation.FreshDecisionRequired {
					t.Fatalf("durable commit was not recovered exactly: %#v", reconciliation)
				}
			} else if reconciliation.Outcome != market.ModuleUpdateJobRollbackReconciliationDefinitelyNotCommitted ||
				!reconciliation.FreshDecisionRequired {
				t.Fatalf("exact pre-CAS state was not classified safely: %#v", reconciliation)
			}
		})
	}

	ambiguous := initial
	ambiguous.Record.State = market.ModuleJobStateRolledBack
	ambiguous.Record.StateVersion++
	ambiguous.Record.JournalSequence = initial.Attempt.AttemptSequence
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: ambiguous}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}
	reconciliation, err := repository.Reconcile(
		context.Background(),
		admission,
		market.ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.Outcome != market.ModuleUpdateJobRollbackReconciliationAmbiguous ||
		!reconciliation.FreshDecisionRequired {
		t.Fatalf("partial durable state did not fail closed: %#v", reconciliation)
	}
}

func TestMarketUpdateRollbackCompletionRepositoryRejectsTamperedReceipt(t *testing.T) {
	admission, initial := marketUpdateRollbackCompletionTestState(t, "completion-tamper")
	result, err := market.CommitModuleUpdateJobRollbackCAS(
		initial,
		admission,
		initial.Attempt.ExpectedStateVersion,
		initial.Attempt.ExpectedJournalSequence,
		market.ModuleUpdateJobRollbackSucceeded,
	)
	if err != nil {
		t.Fatal(err)
	}
	result.State.Receipt.AutomaticRetryAuthorized = true
	backend := &fakeMarketUpdateRollbackCompletionBackend{state: result.State}
	repository := &MarketUpdateJobRollbackCompletionRepository{backend: backend}
	if _, err := repository.Reopen(context.Background(), admission); err == nil {
		t.Fatal("expected tampered completion authority rejection")
	}
}

func TestNewMarketUpdateJobRollbackCompletionRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewMarketUpdateJobRollbackCompletionRepository(nil); err == nil {
		t.Fatal("expected nil database rejection")
	}
}

func marketUpdateRollbackCompletionTestState(
	t *testing.T,
	moduleID string,
) (market.ModuleUpdateJobAdmission, market.ModuleUpdateJobRollbackPersistenceState) {
	t.Helper()
	admission, initial := marketUpdateRollbackClaimTestState(t, moduleID)
	claimed, err := market.CommitModuleUpdateJobRollbackClaimCAS(
		initial,
		admission,
		"rollback-worker-01",
		4,
		3,
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := market.AdmitModuleUpdateJobRollbackAttempt(claimed.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	state, err := market.InitializeModuleUpdateJobRollbackPersistence(claimed.State, attempt, admission)
	if err != nil {
		t.Fatal(err)
	}
	return admission, state
}

func assertRollbackCompletionRepositorySafety(
	t *testing.T,
	result market.ModuleUpdateJobRollbackCommitResult,
) {
	t.Helper()
	if result.FurtherAttemptAuthorized || result.AutomaticRetryAuthorized ||
		result.ExecutionAuthorized || result.ProductionMutationAllowed ||
		result.Receipt.FurtherAttemptAuthorized || result.Receipt.AutomaticRetryAuthorized ||
		result.Receipt.ExecutionAuthorized || result.Receipt.GenericCommandAuthorized ||
		result.Receipt.ProductionMutationAllowed {
		t.Fatalf("rollback completion widened authority: %#v", result)
	}
	if !result.Receipt.AttemptConsumed || !result.Receipt.AtomicPersistenceRequired ||
		!result.Receipt.PreserveUserData || !result.Receipt.PreserveSecretMaterial ||
		!result.Receipt.Terminal {
		t.Fatalf("rollback completion weakened safety guarantees: %#v", result.Receipt)
	}
}

type fakeMarketUpdateRollbackCompletionBackend struct {
	state        market.ModuleUpdateJobRollbackPersistenceState
	failCommit   bool
	failPersist  bool
	persistCalls int
}

func (b *fakeMarketUpdateRollbackCompletionBackend) Begin(
	context.Context,
) (marketUpdateRollbackCompletionTx, error) {
	return &fakeMarketUpdateRollbackCompletionTx{backend: b, working: b.state}, nil
}

type fakeMarketUpdateRollbackCompletionTx struct {
	backend *fakeMarketUpdateRollbackCompletionBackend
	working market.ModuleUpdateJobRollbackPersistenceState
	closed  bool
}

func (t *fakeMarketUpdateRollbackCompletionTx) Load(
	_ context.Context,
	recordID string,
	_ market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobRollbackPersistenceState, error) {
	if t.working.Record.RecordID != recordID {
		return market.ModuleUpdateJobRollbackPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	return t.working, nil
}

func (t *fakeMarketUpdateRollbackCompletionTx) PersistCompletionCAS(
	_ context.Context,
	current market.ModuleUpdateJobRollbackPersistenceState,
	candidate market.ModuleUpdateJobRollbackPersistenceState,
) error {
	if t.backend.failPersist {
		return errors.New("simulated rollback completion persistence failure")
	}
	if t.working != current {
		return ErrMarketUpdateJobRollbackCompletionConflict
	}
	t.working = candidate
	t.backend.persistCalls++
	return nil
}

func (t *fakeMarketUpdateRollbackCompletionTx) Commit() error {
	if t.backend.failCommit {
		return errors.New("simulated rollback completion commit failure")
	}
	t.backend.state = t.working
	t.closed = true
	return nil
}

func (t *fakeMarketUpdateRollbackCompletionTx) Rollback() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return nil
}
