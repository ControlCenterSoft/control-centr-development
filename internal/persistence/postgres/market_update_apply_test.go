package postgres

import (
	"context"
	"errors"
	"testing"

	"control-center/internal/market"
)

func TestMarketUpdateApplyRepositoryAtomicCommitReplayAndReopen(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "inventory-agent")
	initial := claimedApplyPersistenceState(t, admission)
	backend := &fakeMarketUpdateApplyBackend{state: initial}
	repository := &MarketUpdateJobApplyRepository{backend: backend}

	first, err := repository.CommitSuccess(context.Background(), admission, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.JournalAppendRequired || !first.State.HasApplyReceipt {
		t.Fatalf("first apply commit is not atomic record+receipt evidence: %#v", first)
	}
	if first.ExecutionAuthorized || first.ProductionMutation || first.Receipt.ExecutionAuthorized ||
		first.Receipt.ProductionMutationAllowed {
		t.Fatalf("apply commit widened authority: %#v", first)
	}
	persistCalls := backend.persistCalls

	replay, err := repository.CommitSuccess(context.Background(), admission, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.JournalAppendRequired || replay.State != first.State {
		t.Fatalf("exact replay changed persisted apply evidence: %#v", replay)
	}
	if backend.persistCalls != persistCalls {
		t.Fatal("exact replay appended or rewrote apply receipt persistence")
	}

	recovery, err := repository.Reopen(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.ApplyReceiptID != first.Receipt.ReceiptID ||
		recovery.NextStep != market.ModuleUpdateJobApplyRecoveryRevalidateVerification {
		t.Fatalf("restart recovery lost exact apply receipt lineage: %#v", recovery)
	}
	if recovery.ExecutionAuthorized || recovery.ProductionMutationAllowed {
		t.Fatalf("restart recovery widened authority: %#v", recovery)
	}
}

func TestMarketUpdateApplyRepositoryRollsBackFailedCommitAndRetriesDeterministically(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "dns-tools")
	initial := claimedApplyPersistenceState(t, admission)
	expected, err := market.CommitModuleUpdateJobApplySuccessCAS(initial, admission, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeMarketUpdateApplyBackend{state: initial, failCommit: true}
	repository := &MarketUpdateJobApplyRepository{backend: backend}

	if _, err := repository.CommitSuccess(context.Background(), admission, 2, ""); err == nil {
		t.Fatal("expected simulated apply commit failure")
	}
	if backend.state != initial {
		t.Fatalf("failed transaction leaked torn apply state: %#v", backend.state)
	}

	backend.failCommit = false
	retried, err := repository.CommitSuccess(context.Background(), admission, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != expected.State || retried.Receipt != expected.Receipt {
		t.Fatalf("retry after failed commit was not deterministic: %#v != %#v", retried, expected)
	}
}

func TestMarketUpdateApplyRepositoryRollsBackPersistenceFailure(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "backup-tools")
	initial := claimedApplyPersistenceState(t, admission)
	backend := &fakeMarketUpdateApplyBackend{state: initial, failPersist: true}
	repository := &MarketUpdateJobApplyRepository{backend: backend}

	if _, err := repository.CommitSuccess(context.Background(), admission, 2, ""); err == nil {
		t.Fatal("expected simulated apply persistence failure")
	}
	if backend.state != initial || backend.state.HasApplyReceipt {
		t.Fatalf("persistence failure leaked torn apply state: %#v", backend.state)
	}
}

func TestMarketUpdateApplyRepositoryRejectsStaleStateVersion(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "audit-tools")
	initial := claimedApplyPersistenceState(t, admission)
	backend := &fakeMarketUpdateApplyBackend{state: initial}
	repository := &MarketUpdateJobApplyRepository{backend: backend}

	if _, err := repository.CommitSuccess(context.Background(), admission, 1, ""); err == nil {
		t.Fatal("expected stale apply state-version rejection")
	}
	if backend.state != initial {
		t.Fatal("stale apply commit modified durable state")
	}
}

func TestNewMarketUpdateJobApplyRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewMarketUpdateJobApplyRepository(nil); err == nil {
		t.Fatal("expected nil database rejection")
	}
}

func claimedApplyPersistenceState(
	t *testing.T,
	admission market.ModuleUpdateJobAdmission,
) market.ModuleUpdateJobApplyPersistenceState {
	t.Helper()
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := market.InitializeModuleUpdateJobClaimPersistence(record, admission)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := market.CommitModuleUpdateJobClaimCAS(prepared, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	state, err := market.InitializeModuleUpdateJobApplyPersistence(
		claimed.State.Record,
		admission,
		claimed.State.Journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

type fakeMarketUpdateApplyBackend struct {
	state        market.ModuleUpdateJobApplyPersistenceState
	failCommit   bool
	failPersist  bool
	persistCalls int
}

func (b *fakeMarketUpdateApplyBackend) Begin(context.Context) (marketUpdateApplyTx, error) {
	return &fakeMarketUpdateApplyTx{backend: b, working: b.state}, nil
}

type fakeMarketUpdateApplyTx struct {
	backend *fakeMarketUpdateApplyBackend
	working market.ModuleUpdateJobApplyPersistenceState
	closed  bool
}

func (t *fakeMarketUpdateApplyTx) Load(
	_ context.Context,
	recordID string,
) (market.ModuleUpdateJobApplyPersistenceState, error) {
	if t.working.Record.RecordID != recordID {
		return market.ModuleUpdateJobApplyPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	return t.working, nil
}

func (t *fakeMarketUpdateApplyTx) PersistApplyCAS(
	_ context.Context,
	current market.ModuleUpdateJobApplyPersistenceState,
	candidate market.ModuleUpdateJobApplyPersistenceState,
) error {
	if t.backend.failPersist {
		return errors.New("simulated apply persistence failure")
	}
	if t.working != current {
		return ErrMarketUpdateJobApplyConflict
	}
	t.working = candidate
	t.backend.persistCalls++
	return nil
}

func (t *fakeMarketUpdateApplyTx) Commit() error {
	if t.backend.failCommit {
		return errors.New("simulated apply commit failure")
	}
	t.backend.state = t.working
	t.closed = true
	return nil
}

func (t *fakeMarketUpdateApplyTx) Rollback() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return nil
}
