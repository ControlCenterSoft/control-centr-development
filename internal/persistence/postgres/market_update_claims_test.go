package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"control-center/internal/market"
)

func TestMarketUpdateClaimRepositoryAtomicClaimAndReplay(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "inventory-agent")
	backend := &fakeMarketUpdateClaimBackend{}
	repository := &MarketUpdateJobClaimRepository{backend: backend}

	prepared, err := repository.Initialize(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Record.State != market.ModuleJobStatePrepared || prepared.HasJournal {
		t.Fatalf("unexpected prepared state: %#v", prepared)
	}
	first, err := repository.Claim(context.Background(), admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || !first.JournalAppendRequired || !first.State.HasJournal {
		t.Fatalf("first claim is not an atomic record+journal commit: %#v", first)
	}
	if first.ExecutionAuthorized || first.ProductionMutation {
		t.Fatalf("claim widened authority: %#v", first)
	}
	persistCalls := backend.persistCalls

	replay, err := repository.Claim(context.Background(), admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.JournalAppendRequired || replay.State != first.State {
		t.Fatalf("exact replay changed persisted evidence: %#v", replay)
	}
	if backend.persistCalls != persistCalls {
		t.Fatal("exact replay appended or rewrote claim persistence")
	}

	reopened, err := repository.Reopen(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Record != first.State.Record || reopened.Journal != first.State.Journal {
		t.Fatalf("restart reopen changed durable evidence: %#v", reopened)
	}
}

func TestMarketUpdateClaimRepositoryRollsBackFailedCommitAndRetriesDeterministically(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "dns-tools")
	backend := &fakeMarketUpdateClaimBackend{}
	repository := &MarketUpdateJobClaimRepository{backend: backend}
	prepared, err := repository.Initialize(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := market.CommitModuleUpdateJobClaimCAS(prepared, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}

	backend.failCommit = true
	if _, err := repository.Claim(context.Background(), admission, "worker-01", 1); err == nil {
		t.Fatal("expected simulated commit failure")
	}
	if backend.state != prepared {
		t.Fatalf("failed transaction leaked torn state: %#v", backend.state)
	}

	backend.failCommit = false
	retried, err := repository.Claim(context.Background(), admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if retried.State != expected.State || retried.Claim != expected.Claim {
		t.Fatalf("retry after failed commit was not deterministic: %#v != %#v", retried, expected)
	}
}

func TestMarketUpdateClaimRepositoryRejectsStaleAndCompetingWorkers(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "audit-tools")
	backend := &fakeMarketUpdateClaimBackend{}
	repository := &MarketUpdateJobClaimRepository{backend: backend}
	prepared, err := repository.Initialize(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Claim(context.Background(), admission, "worker-01", 2); err == nil {
		t.Fatal("expected stale state-version rejection")
	}
	if backend.state != prepared {
		t.Fatal("stale claim modified durable state")
	}
	first, err := repository.Claim(context.Background(), admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Claim(context.Background(), admission, "worker-02", 1); err == nil {
		t.Fatal("expected competing worker rejection")
	}
	if backend.state != first.State {
		t.Fatal("competing worker modified durable state")
	}
}

func TestMarketUpdateClaimRepositoryRollsBackPersistenceFailure(t *testing.T) {
	admission := marketUpdateClaimTestAdmission(t, "backup-tools")
	backend := &fakeMarketUpdateClaimBackend{}
	repository := &MarketUpdateJobClaimRepository{backend: backend}
	prepared, err := repository.Initialize(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	backend.failPersist = true
	if _, err := repository.Claim(context.Background(), admission, "worker-01", 1); err == nil {
		t.Fatal("expected persistence failure")
	}
	if backend.state != prepared || backend.state.HasJournal {
		t.Fatalf("persistence failure leaked torn state: %#v", backend.state)
	}
}

func TestNewMarketUpdateJobClaimRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewMarketUpdateJobClaimRepository(nil); err == nil {
		t.Fatal("expected nil database rejection")
	}
}

func TestMarketUpdateClaimPostgresCASAndRestart(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL Market claim integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var schemaReady bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('cc_market_update_jobs') IS NOT NULL
        AND to_regclass('cc_market_update_job_claim_journal') IS NOT NULL`).Scan(&schemaReady); err != nil || !schemaReady {
		t.Fatalf("database must be migrated through 0009: ready=%v err=%v", schemaReady, err)
	}

	moduleID := fmt.Sprintf("market-adapter-%d", time.Now().UnixNano())
	admission := marketUpdateClaimTestAdmission(t, moduleID)
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM cc_market_update_job_claim_journal WHERE record_id=$1`, record.RecordID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM cc_market_update_jobs WHERE record_id=$1`, record.RecordID)
	})

	repository, err := NewMarketUpdateJobClaimRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Initialize(ctx, admission); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.Claim(ctx, admission, "worker-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Claim(ctx, admission, "worker-02", 1); err == nil {
		t.Fatal("expected PostgreSQL competing-worker rejection")
	}

	restarted, err := NewMarketUpdateJobClaimRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := restarted.Reopen(ctx, admission)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Record != claimed.State.Record || reopened.Journal != claimed.State.Journal {
		t.Fatalf("PostgreSQL restart changed exact claim evidence: %#v", reopened)
	}
	var journalCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM cc_market_update_job_claim_journal WHERE record_id=$1`, record.RecordID).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if journalCount != 1 {
		t.Fatalf("expected exactly one immutable claim journal, got %d", journalCount)
	}
}

func marketUpdateClaimTestAdmission(t *testing.T, moduleID string) market.ModuleUpdateJobAdmission {
	t.Helper()
	request := market.ModuleLifecycleRequest{
		ModuleID:       moduleID,
		Action:         market.ModuleActionUpdate,
		CurrentState:   market.ModuleStateActive,
		CurrentVersion: "1.4.0",
		TargetVersion:  "1.5.0",
		Generation:     42,
	}
	lifecycle, err := market.PlanModuleLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := market.PlanModuleLifecycleJob(lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("bounded-market-update-bundle")
	digest := sha256.Sum256(payload)
	bundle, err := market.PlanModuleUpdateBundle(market.ModuleUpdateBundleRequest{
		ModuleID:             request.ModuleID,
		CurrentVersion:       request.CurrentVersion,
		TargetVersion:        request.TargetVersion,
		CurrentSchemaVersion: "1.0.0",
		TargetSchemaVersion:  "1.0.0",
		BundleSHA256:         hex.EncodeToString(digest[:]),
		BundleSizeBytes:      uint64(len(payload)),
		Generation:           request.Generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := market.AdmitModuleUpdateJob(
		request,
		lifecycle,
		job,
		bundle,
		payload,
		map[string][]byte{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return admission
}

type fakeMarketUpdateClaimBackend struct {
	state        market.ModuleUpdateJobClaimPersistenceState
	hasState     bool
	failCommit   bool
	failPersist  bool
	persistCalls int
}

func (b *fakeMarketUpdateClaimBackend) Begin(context.Context) (marketUpdateClaimTx, error) {
	return &fakeMarketUpdateClaimTx{
		backend:  b,
		working:  b.state,
		hasState: b.hasState,
	}, nil
}

type fakeMarketUpdateClaimTx struct {
	backend  *fakeMarketUpdateClaimBackend
	working  market.ModuleUpdateJobClaimPersistenceState
	hasState bool
	closed   bool
}

func (t *fakeMarketUpdateClaimTx) InsertPrepared(_ context.Context, record market.ModuleUpdateJobRecord) error {
	if !t.hasState {
		t.working = market.ModuleUpdateJobClaimPersistenceState{Record: record}
		t.hasState = true
	}
	return nil
}

func (t *fakeMarketUpdateClaimTx) Load(_ context.Context, recordID string) (market.ModuleUpdateJobClaimPersistenceState, error) {
	if !t.hasState || t.working.Record.RecordID != recordID {
		return market.ModuleUpdateJobClaimPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	return t.working, nil
}

func (t *fakeMarketUpdateClaimTx) PersistClaimCAS(
	_ context.Context,
	current market.ModuleUpdateJobClaimPersistenceState,
	candidate market.ModuleUpdateJobClaimPersistenceState,
) error {
	if t.backend.failPersist {
		return errors.New("simulated persistence failure")
	}
	if !t.hasState || t.working != current {
		return ErrMarketUpdateJobClaimConflict
	}
	t.working = candidate
	t.backend.persistCalls++
	return nil
}

func (t *fakeMarketUpdateClaimTx) Commit() error {
	if t.backend.failCommit {
		return errors.New("simulated commit failure")
	}
	t.backend.state = t.working
	t.backend.hasState = t.hasState
	t.closed = true
	return nil
}

func (t *fakeMarketUpdateClaimTx) Rollback() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return nil
}
