package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/market"
)

func TestMarketUpdateVerificationRepositoryAtomicCommitReplayAndReopen(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "inventory-agent")
	initial := verificationPersistenceState(t, admission)
	backend := &fakeMarketUpdateVerificationBackend{state: initial}
	repository := &MarketUpdateJobVerificationRepository{backend: backend}

	first, err := repository.Commit(
		context.Background(),
		admission,
		3,
		market.ModuleUpdateJobVerificationPassed,
		"verify:inventory-agent",
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.JournalAppendRequired || !first.State.HasVerificationReceipt {
		t.Fatalf("first verification commit is not atomic record+receipt evidence: %#v", first)
	}
	if first.State.Record.State != market.ModuleJobStateSucceeded ||
		!first.Receipt.Terminal || first.RollbackRequired {
		t.Fatalf("successful verification has wrong terminal semantics: %#v", first)
	}
	assertVerificationRepositorySafety(t, first)
	persistCalls := backend.persistCalls

	replay, err := repository.Commit(
		context.Background(),
		admission,
		3,
		market.ModuleUpdateJobVerificationPassed,
		"verify:inventory-agent",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.JournalAppendRequired || replay.State != first.State {
		t.Fatalf("exact verification replay changed persisted evidence: %#v", replay)
	}
	if backend.persistCalls != persistCalls {
		t.Fatal("exact verification replay appended or rewrote persistence")
	}

	reopened, err := repository.Reopen(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Replay || reopened.State != first.State || reopened.Receipt != first.Receipt {
		t.Fatalf("restart reopen changed verification evidence: %#v", reopened)
	}
	if backend.persistCalls != persistCalls {
		t.Fatal("restart reopen mutated verification persistence")
	}
}

func TestMarketUpdateVerificationRepositoryFailureCreatesRollbackHandoffOnly(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "dns-tools")
	initial := verificationPersistenceState(t, admission)
	backend := &fakeMarketUpdateVerificationBackend{state: initial}
	repository := &MarketUpdateJobVerificationRepository{backend: backend}

	result, err := repository.Commit(
		context.Background(),
		admission,
		3,
		market.ModuleUpdateJobVerificationFailed,
		"verify:dns-tools-failed",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Record.State != market.ModuleJobStateRollingBack ||
		!result.RollbackRequired || !result.Receipt.RollbackRequiredNow || result.Receipt.Terminal {
		t.Fatalf("failed verification did not produce bounded rollback handoff: %#v", result)
	}
	assertVerificationRepositorySafety(t, result)
}

func TestMarketUpdateVerificationRepositoryRollsBackFailedCommitAndRetriesDeterministically(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "backup-tools")
	initial := verificationPersistenceState(t, admission)
	expected, err := market.CommitModuleUpdateJobVerificationCAS(
		initial,
		admission,
		3,
		market.ModuleUpdateJobVerificationPassed,
		"verify:backup-tools",
	)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeMarketUpdateVerificationBackend{state: initial, failCommit: true}
	repository := &MarketUpdateJobVerificationRepository{backend: backend}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		3,
		market.ModuleUpdateJobVerificationPassed,
		"verify:backup-tools",
	); err == nil {
		t.Fatal("expected simulated verification commit failure")
	}
	if backend.state != initial {
		t.Fatalf("failed transaction leaked torn verification state: %#v", backend.state)
	}

	backend.failCommit = false
	retried, err := repository.Commit(
		context.Background(),
		admission,
		3,
		market.ModuleUpdateJobVerificationPassed,
		"verify:backup-tools",
	)
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != expected.State || retried.Receipt != expected.Receipt {
		t.Fatalf("retry after failed commit was not deterministic: %#v != %#v", retried, expected)
	}
}

func TestMarketUpdateVerificationRepositoryRollsBackPersistenceFailure(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "audit-tools")
	initial := verificationPersistenceState(t, admission)
	backend := &fakeMarketUpdateVerificationBackend{state: initial, failPersist: true}
	repository := &MarketUpdateJobVerificationRepository{backend: backend}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		3,
		market.ModuleUpdateJobVerificationFailed,
		"verify:audit-tools-failed",
	); err == nil {
		t.Fatal("expected simulated verification persistence failure")
	}
	if backend.state != initial || backend.state.HasVerificationReceipt {
		t.Fatalf("persistence failure leaked torn verification state: %#v", backend.state)
	}
}

func TestMarketUpdateVerificationRepositoryRejectsStaleStateVersion(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "metrics-tools")
	initial := verificationPersistenceState(t, admission)
	backend := &fakeMarketUpdateVerificationBackend{state: initial}
	repository := &MarketUpdateJobVerificationRepository{backend: backend}

	if _, err := repository.Commit(
		context.Background(),
		admission,
		2,
		market.ModuleUpdateJobVerificationPassed,
		"verify:metrics-tools",
	); err == nil {
		t.Fatal("expected stale verification state-version rejection")
	}
	if backend.state != initial {
		t.Fatal("stale verification commit modified durable state")
	}
}

func TestMarketUpdateVerificationRepositoryReopenRejectsPendingVerification(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "logging-tools")
	initial := verificationPersistenceState(t, admission)
	backend := &fakeMarketUpdateVerificationBackend{state: initial}
	repository := &MarketUpdateJobVerificationRepository{backend: backend}

	if _, err := repository.Reopen(context.Background(), admission); err == nil {
		t.Fatal("expected pending VERIFYING state to require explicit recovery/revalidation")
	}
	if backend.state != initial || backend.persistCalls != 0 {
		t.Fatal("pending verification reopen must remain read-only")
	}
}

func TestNewMarketUpdateJobVerificationRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewMarketUpdateJobVerificationRepository(nil); err == nil {
		t.Fatal("expected nil database rejection")
	}
}

func TestMarketUpdateVerificationPostgresCASAndRestart(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL Market verification integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var schemaReady bool
	if err := db.QueryRowContext(ctx, `SELECT
        to_regclass('cc_market_update_jobs') IS NOT NULL
        AND to_regclass('cc_market_update_job_claim_journal') IS NOT NULL
        AND to_regclass('cc_market_update_job_apply_journal') IS NOT NULL
        AND to_regclass('cc_market_update_job_verification_journal') IS NOT NULL`).Scan(&schemaReady); err != nil || !schemaReady {
		t.Fatalf("database must be migrated through 0011: ready=%v err=%v", schemaReady, err)
	}

	moduleID := fmt.Sprintf("verification-adapter-%d", time.Now().UnixNano())
	admission := marketUpdateClaimTestAdmission(t, moduleID)
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		tx, err := db.BeginTx(cleanupCtx, nil)
		if err != nil {
			t.Errorf("begin verification cleanup: %v", err)
			return
		}
		defer tx.Rollback()
		for _, query := range []string{
			`DELETE FROM cc_market_update_job_verification_journal WHERE record_id=$1`,
			`DELETE FROM cc_market_update_job_apply_journal WHERE record_id=$1`,
			`DELETE FROM cc_market_update_job_claim_journal WHERE record_id=$1`,
			`DELETE FROM cc_market_update_jobs WHERE record_id=$1`,
		} {
			if _, err := tx.ExecContext(cleanupCtx, query, record.RecordID); err != nil {
				t.Errorf("verification cleanup failed: %v", err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			t.Errorf("commit verification cleanup: %v", err)
		}
	})

	claimRepository, err := NewMarketUpdateJobClaimRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimRepository.Initialize(ctx, admission); err != nil {
		t.Fatal(err)
	}
	if _, err := claimRepository.Claim(ctx, admission, "worker-01", 1); err != nil {
		t.Fatal(err)
	}
	applyRepository, err := NewMarketUpdateJobApplyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyRepository.CommitSuccess(ctx, admission, 2, ""); err != nil {
		t.Fatal(err)
	}
	verificationRepository, err := NewMarketUpdateJobVerificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := verificationRepository.Commit(
		ctx,
		admission,
		3,
		market.ModuleUpdateJobVerificationPassed,
		"verify:postgres-restart",
	)
	if err != nil {
		t.Fatal(err)
	}

	restarted, err := NewMarketUpdateJobVerificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := restarted.Reopen(ctx, admission)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State != committed.State || reopened.Receipt != committed.Receipt || !reopened.Replay {
		t.Fatalf("PostgreSQL restart changed exact verification evidence: %#v", reopened)
	}
	var verificationCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM cc_market_update_job_verification_journal WHERE record_id=$1`, record.RecordID).Scan(&verificationCount); err != nil {
		t.Fatal(err)
	}
	if verificationCount != 1 {
		t.Fatalf("expected exactly one immutable verification receipt, got %d", verificationCount)
	}
}

func verificationPersistenceState(
	t *testing.T,
	admission market.ModuleUpdateJobAdmission,
) market.ModuleUpdateJobVerificationPersistenceState {
	t.Helper()
	claimed := claimedApplyPersistenceState(t, admission)
	applied, err := market.CommitModuleUpdateJobApplySuccessCAS(claimed, admission, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := market.InitializeModuleUpdateJobVerificationPersistence(applied.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertVerificationRepositorySafety(
	t *testing.T,
	result market.ModuleUpdateJobVerificationCommitResult,
) {
	t.Helper()
	if result.ExecutionAuthorized || result.ProductionMutation ||
		result.Receipt.ExecutionAuthorized || result.Receipt.ProductionMutationAllowed {
		t.Fatalf("verification persistence widened authority: %#v", result)
	}
	if !result.Receipt.PreserveUserData || !result.Receipt.PreserveSecretMaterial {
		t.Fatalf("verification persistence dropped preservation guarantees: %#v", result.Receipt)
	}
}

type fakeMarketUpdateVerificationBackend struct {
	state        market.ModuleUpdateJobVerificationPersistenceState
	failCommit   bool
	failPersist  bool
	persistCalls int
}

func (b *fakeMarketUpdateVerificationBackend) Begin(context.Context) (marketUpdateVerificationTx, error) {
	return &fakeMarketUpdateVerificationTx{backend: b, working: b.state}, nil
}

type fakeMarketUpdateVerificationTx struct {
	backend *fakeMarketUpdateVerificationBackend
	working market.ModuleUpdateJobVerificationPersistenceState
	closed  bool
}

func (t *fakeMarketUpdateVerificationTx) Load(
	_ context.Context,
	recordID string,
) (market.ModuleUpdateJobVerificationPersistenceState, error) {
	if t.working.Record.RecordID != recordID {
		return market.ModuleUpdateJobVerificationPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	return t.working, nil
}

func (t *fakeMarketUpdateVerificationTx) PersistVerificationCAS(
	_ context.Context,
	current market.ModuleUpdateJobVerificationPersistenceState,
	candidate market.ModuleUpdateJobVerificationPersistenceState,
) error {
	if t.backend.failPersist {
		return errors.New("simulated verification persistence failure")
	}
	if t.working != current {
		return ErrMarketUpdateJobVerificationConflict
	}
	t.working = candidate
	t.backend.persistCalls++
	return nil
}

func (t *fakeMarketUpdateVerificationTx) Commit() error {
	if t.backend.failCommit {
		return errors.New("simulated verification commit failure")
	}
	t.backend.state = t.working
	t.closed = true
	return nil
}

func (t *fakeMarketUpdateVerificationTx) Rollback() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return nil
}
