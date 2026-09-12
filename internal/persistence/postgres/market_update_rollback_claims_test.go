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

func TestMarketUpdateRollbackClaimRepositoryAtomicClaimReplayAndReopen(t *testing.T) {
	admission, initial := marketUpdateRollbackClaimTestState(t, "rollback-inventory")
	backend := &fakeMarketUpdateRollbackClaimBackend{state: initial}
	repository := &MarketUpdateJobRollbackClaimRepository{backend: backend}

	first, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.ClaimAppendRequired || !first.State.HasClaim {
		t.Fatalf("first rollback claim is not an atomic durable append: %#v", first)
	}
	assertRollbackClaimRepositorySafety(t, first)
	persistCalls := backend.persistCalls

	replay, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.ClaimAppendRequired || replay.State != first.State {
		t.Fatalf("exact rollback claim replay changed durable evidence: %#v", replay)
	}
	if backend.persistCalls != persistCalls {
		t.Fatal("exact rollback claim replay appended persistence twice")
	}

	reopened, err := repository.Reopen(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Replay || reopened.State != first.State || reopened.Claim != first.Claim {
		t.Fatalf("restart reopen changed rollback claim evidence: %#v", reopened)
	}
	assertRollbackClaimRepositorySafety(t, reopened)
}

func TestMarketUpdateRollbackClaimRepositoryRejectsStaleAndCompetingWorkers(t *testing.T) {
	admission, initial := marketUpdateRollbackClaimTestState(t, "rollback-dns")
	backend := &fakeMarketUpdateRollbackClaimBackend{state: initial}
	repository := &MarketUpdateJobRollbackClaimRepository{backend: backend}

	if _, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 3, 3); err == nil {
		t.Fatal("expected stale state-version rejection")
	}
	if _, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 2); err == nil {
		t.Fatal("expected stale journal-sequence rejection")
	}
	if backend.state != initial || backend.persistCalls != 0 {
		t.Fatal("stale rollback claim modified durable state")
	}

	first, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Claim(context.Background(), admission, "rollback-worker-02", 4, 3); err == nil {
		t.Fatal("expected competing rollback worker rejection")
	}
	if backend.state != first.State || backend.persistCalls != 1 {
		t.Fatal("competing rollback worker modified durable state")
	}
}

func TestMarketUpdateRollbackClaimRepositoryRollsBackFailedCommitAndRetries(t *testing.T) {
	admission, initial := marketUpdateRollbackClaimTestState(t, "rollback-backup")
	expected, err := market.CommitModuleUpdateJobRollbackClaimCAS(
		initial,
		admission,
		"rollback-worker-01",
		4,
		3,
	)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeMarketUpdateRollbackClaimBackend{state: initial, failCommit: true}
	repository := &MarketUpdateJobRollbackClaimRepository{backend: backend}

	if _, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3); err == nil {
		t.Fatal("expected simulated rollback-claim commit failure")
	}
	if backend.state != initial {
		t.Fatalf("failed transaction leaked rollback claim: %#v", backend.state)
	}

	backend.failCommit = false
	retried, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != expected.State || retried.Claim != expected.Claim {
		t.Fatalf("retry after failed commit was not deterministic: %#v != %#v", retried, expected)
	}
}

func TestMarketUpdateRollbackClaimRepositoryRollsBackPersistenceFailure(t *testing.T) {
	admission, initial := marketUpdateRollbackClaimTestState(t, "rollback-audit")
	backend := &fakeMarketUpdateRollbackClaimBackend{state: initial, failPersist: true}
	repository := &MarketUpdateJobRollbackClaimRepository{backend: backend}

	if _, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3); err == nil {
		t.Fatal("expected simulated rollback-claim persistence failure")
	}
	if backend.state != initial || backend.state.HasClaim {
		t.Fatalf("persistence failure leaked torn rollback claim: %#v", backend.state)
	}
}

func TestMarketUpdateRollbackClaimRepositoryRejectsTamperedPersistedEvidence(t *testing.T) {
	admission, initial := marketUpdateRollbackClaimTestState(t, "rollback-metrics")
	backend := &fakeMarketUpdateRollbackClaimBackend{state: initial}
	repository := &MarketUpdateJobRollbackClaimRepository{backend: backend}
	first, err := repository.Claim(context.Background(), admission, "rollback-worker-01", 4, 3)
	if err != nil {
		t.Fatal(err)
	}

	backend.state.Claim.ExecutionAuthorized = true
	if _, err := repository.Reopen(context.Background(), admission); err == nil {
		t.Fatal("expected tampered rollback-claim authority rejection")
	}
	backend.state = first.State
	backend.state.Claim.PreserveSecretMaterial = false
	if _, err := repository.Reopen(context.Background(), admission); err == nil {
		t.Fatal("expected dropped secret-preservation guarantee rejection")
	}
}

func TestNewMarketUpdateJobRollbackClaimRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewMarketUpdateJobRollbackClaimRepository(nil); err == nil {
		t.Fatal("expected nil database rejection")
	}
}

func TestMarketUpdateRollbackClaimPostgresCASAndRestart(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL rollback-claim integration test skipped")
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
        to_regclass('cc_market_update_job_verification_journal') IS NOT NULL
        AND to_regclass('cc_market_update_job_rollback_claims') IS NOT NULL`).Scan(&schemaReady); err != nil || !schemaReady {
		t.Fatalf("database must be migrated through 0012: ready=%v err=%v", schemaReady, err)
	}

	moduleID := fmt.Sprintf("rollback-adapter-%d", time.Now().UnixNano())
	admission := marketUpdateClaimTestAdmission(t, moduleID)
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		tx, err := db.BeginTx(cleanupCtx, nil)
		if err != nil {
			t.Errorf("begin rollback-claim cleanup: %v", err)
			return
		}
		defer tx.Rollback()
		for _, query := range []string{
			`DELETE FROM cc_market_update_job_rollback_claims WHERE record_id=$1`,
			`DELETE FROM cc_market_update_job_verification_journal WHERE record_id=$1`,
			`DELETE FROM cc_market_update_job_apply_journal WHERE record_id=$1`,
			`DELETE FROM cc_market_update_job_claim_journal WHERE record_id=$1`,
			`DELETE FROM cc_market_update_jobs WHERE record_id=$1`,
		} {
			if _, err := tx.ExecContext(cleanupCtx, query, record.RecordID); err != nil {
				t.Errorf("rollback-claim cleanup failed: %v", err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			t.Errorf("commit rollback-claim cleanup: %v", err)
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
	if _, err := verificationRepository.Commit(
		ctx,
		admission,
		3,
		market.ModuleUpdateJobVerificationFailed,
		"verify:rollback-postgres",
	); err != nil {
		t.Fatal(err)
	}

	rollbackRepository, err := NewMarketUpdateJobRollbackClaimRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := rollbackRepository.Claim(ctx, admission, "rollback-worker-01", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rollbackRepository.Claim(ctx, admission, "rollback-worker-02", 4, 3); err == nil {
		t.Fatal("expected PostgreSQL competing rollback-worker rejection")
	}

	restarted, err := NewMarketUpdateJobRollbackClaimRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := restarted.Reopen(ctx, admission)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State != claimed.State || reopened.Claim != claimed.Claim || !reopened.Replay {
		t.Fatalf("PostgreSQL restart changed exact rollback claim evidence: %#v", reopened)
	}
	var rollbackClaimCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM cc_market_update_job_rollback_claims WHERE record_id=$1`,
		record.RecordID,
	).Scan(&rollbackClaimCount); err != nil {
		t.Fatal(err)
	}
	if rollbackClaimCount != 1 {
		t.Fatalf("expected exactly one immutable rollback claim, got %d", rollbackClaimCount)
	}
}

func marketUpdateRollbackClaimTestState(
	t *testing.T,
	moduleID string,
) (market.ModuleUpdateJobAdmission, market.ModuleUpdateJobRollbackClaimPersistenceState) {
	t.Helper()
	admission := marketUpdateClaimTestAdmission(t, moduleID)
	verification := verificationPersistenceState(t, admission)
	failed, err := market.CommitModuleUpdateJobVerificationCAS(
		verification,
		admission,
		3,
		market.ModuleUpdateJobVerificationFailed,
		"verify:"+moduleID+":failed",
	)
	if err != nil {
		t.Fatal(err)
	}
	rollbackAdmission, err := market.PlanModuleUpdateJobRollbackAdmission(failed.State, admission)
	if err != nil {
		t.Fatal(err)
	}
	state, err := market.InitializeModuleUpdateJobRollbackClaimPersistence(
		failed.State,
		rollbackAdmission,
		admission,
	)
	if err != nil {
		t.Fatal(err)
	}
	return admission, state
}

func assertRollbackClaimRepositorySafety(
	t *testing.T,
	result market.ModuleUpdateJobRollbackClaimCommitResult,
) {
	t.Helper()
	if result.ExecutionAuthorized || result.ProductionMutation ||
		result.Claim.ExecutionAuthorized || result.Claim.ProductionMutationAllowed {
		t.Fatalf("rollback-claim persistence widened authority: %#v", result)
	}
	if !result.Claim.PreserveUserData || !result.Claim.PreserveSecretMaterial ||
		!result.Claim.SingleUse || !result.Claim.AtomicPersistenceRequired ||
		!result.Claim.FreshRevalidationRequired {
		t.Fatalf("rollback-claim persistence weakened safety guarantees: %#v", result.Claim)
	}
}

type fakeMarketUpdateRollbackClaimBackend struct {
	state        market.ModuleUpdateJobRollbackClaimPersistenceState
	failCommit   bool
	failPersist  bool
	persistCalls int
}

func (b *fakeMarketUpdateRollbackClaimBackend) Begin(context.Context) (marketUpdateRollbackClaimTx, error) {
	return &fakeMarketUpdateRollbackClaimTx{backend: b, working: b.state}, nil
}

type fakeMarketUpdateRollbackClaimTx struct {
	backend *fakeMarketUpdateRollbackClaimBackend
	working market.ModuleUpdateJobRollbackClaimPersistenceState
	closed  bool
}

func (t *fakeMarketUpdateRollbackClaimTx) Load(
	_ context.Context,
	recordID string,
	_ market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobRollbackClaimPersistenceState, error) {
	if t.working.Source.Record.RecordID != recordID {
		return market.ModuleUpdateJobRollbackClaimPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	return t.working, nil
}

func (t *fakeMarketUpdateRollbackClaimTx) PersistClaimCAS(
	_ context.Context,
	current market.ModuleUpdateJobRollbackClaimPersistenceState,
	candidate market.ModuleUpdateJobRollbackClaimPersistenceState,
) error {
	if t.backend.failPersist {
		return errors.New("simulated rollback-claim persistence failure")
	}
	if t.working != current {
		return ErrMarketUpdateJobRollbackClaimConflict
	}
	t.working = candidate
	t.backend.persistCalls++
	return nil
}

func (t *fakeMarketUpdateRollbackClaimTx) Commit() error {
	if t.backend.failCommit {
		return errors.New("simulated rollback-claim commit failure")
	}
	t.backend.state = t.working
	t.closed = true
	return nil
}

func (t *fakeMarketUpdateRollbackClaimTx) Rollback() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return nil
}
