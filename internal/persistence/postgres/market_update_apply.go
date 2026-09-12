package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"control-center/internal/market"
)

var ErrMarketUpdateJobApplyConflict = errors.New("market update job apply persistence conflict")

type MarketUpdateJobApplyRepository struct {
	backend marketUpdateApplyBackend
}

func NewMarketUpdateJobApplyRepository(db *sql.DB) (*MarketUpdateJobApplyRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &MarketUpdateJobApplyRepository{backend: sqlMarketUpdateApplyBackend{db: db}}, nil
}

// CommitSuccess atomically persists RUNNING -> VERIFYING and the immutable apply
// receipt. Exact replay is read-only and never appends a second journal entry.
func (r *MarketUpdateJobApplyRepository) CommitSuccess(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
	expectedStateVersion uint64,
	preMigrationSnapshotID string,
) (market.ModuleUpdateJobApplyCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobApplyCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobApplyCommitResult{}, err
	}
	defer tx.Rollback()

	current, err := tx.Load(ctx, record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobApplyCommitResult{}, err
	}
	result, err := market.CommitModuleUpdateJobApplySuccessCAS(
		current,
		admission,
		expectedStateVersion,
		preMigrationSnapshotID,
	)
	if err != nil {
		return market.ModuleUpdateJobApplyCommitResult{}, err
	}
	if result.ExecutionAuthorized || result.ProductionMutation || result.Receipt.ExecutionAuthorized ||
		result.Receipt.ProductionMutationAllowed {
		return market.ModuleUpdateJobApplyCommitResult{}, errors.New("Market update apply widened execution authority")
	}
	if !result.Replay {
		if !result.JournalAppendRequired || !result.State.HasApplyReceipt {
			return market.ModuleUpdateJobApplyCommitResult{}, errors.New("first Market update apply lacks atomic receipt evidence")
		}
		if err := tx.PersistApplyCAS(ctx, current, result.State); err != nil {
			return market.ModuleUpdateJobApplyCommitResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobApplyCommitResult{}, err
	}
	return result, nil
}

// Reopen revalidates a durable VERIFYING record and reconstructs restart
// recovery evidence without executing verification, rollback, or module code.
func (r *MarketUpdateJobApplyRepository) Reopen(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobApplyRecoveryPlan, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobApplyRecoveryPlan{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobApplyRecoveryPlan{}, err
	}
	defer tx.Rollback()

	persisted, err := tx.Load(ctx, record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobApplyRecoveryPlan{}, err
	}
	plan, err := market.RecoverModuleUpdateJobApplyPersistence(persisted, admission)
	if err != nil {
		return market.ModuleUpdateJobApplyRecoveryPlan{}, err
	}
	if plan.ExecutionAuthorized || plan.ProductionMutationAllowed {
		return market.ModuleUpdateJobApplyRecoveryPlan{}, errors.New("Market update apply recovery widened execution authority")
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobApplyRecoveryPlan{}, err
	}
	return plan, nil
}

type marketUpdateApplyBackend interface {
	Begin(context.Context) (marketUpdateApplyTx, error)
}

type marketUpdateApplyTx interface {
	Load(context.Context, string) (market.ModuleUpdateJobApplyPersistenceState, error)
	PersistApplyCAS(context.Context, market.ModuleUpdateJobApplyPersistenceState, market.ModuleUpdateJobApplyPersistenceState) error
	Commit() error
	Rollback() error
}

type sqlMarketUpdateApplyBackend struct{ db *sql.DB }

func (b sqlMarketUpdateApplyBackend) Begin(ctx context.Context) (marketUpdateApplyTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlMarketUpdateApplyTx{tx: tx}, nil
}

type sqlMarketUpdateApplyTx struct{ tx *sql.Tx }

func (t *sqlMarketUpdateApplyTx) Load(
	ctx context.Context,
	recordID string,
) (market.ModuleUpdateJobApplyPersistenceState, error) {
	record, err := scanMarketUpdateJobRecord(t.tx.QueryRowContext(ctx, `
SELECT record_id,job_id,admission_id,state,state_version,journal_sequence,
       claim_id,claimed_by,claimed_from_state_version,
       preserve_user_data,preserve_secret_material,execution_authorized
FROM cc_market_update_jobs
WHERE record_id=$1
FOR UPDATE`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return market.ModuleUpdateJobApplyPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	if err != nil {
		return market.ModuleUpdateJobApplyPersistenceState{}, err
	}

	claimJournal, err := scanMarketUpdateJobClaimJournal(t.tx.QueryRowContext(ctx, `
SELECT journal_id,sequence,record_id,job_id,admission_id,claim_id,worker_id,
       from_state,to_state,from_state_version,to_state_version,execution_authorized
FROM cc_market_update_job_claim_journal
WHERE record_id=$1`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return market.ModuleUpdateJobApplyPersistenceState{}, fmt.Errorf("%w: apply state lacks claim journal", ErrMarketUpdateJobApplyConflict)
	}
	if err != nil {
		return market.ModuleUpdateJobApplyPersistenceState{}, err
	}

	state := market.ModuleUpdateJobApplyPersistenceState{Record: record, ClaimJournal: claimJournal}
	receipt, err := scanMarketUpdateJobApplyReceipt(t.tx.QueryRowContext(ctx, `
SELECT receipt_id,record_id,job_id,admission_id,claim_id,worker_id,
       bundle_id,bundle_sha256,bundle_size_bytes,current_version,target_version,
       generation,migration_count,rollback_mode,require_pre_migration_snapshot,
       pre_migration_snapshot_id,from_state,to_state,from_state_version,to_state_version,
       journal_sequence,preserve_user_data,preserve_secret_material,
       verification_required,rollback_required_on_failure,execution_authorized,
       production_mutation_allowed
FROM cc_market_update_job_apply_journal
WHERE record_id=$1`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return market.ModuleUpdateJobApplyPersistenceState{}, err
	}
	state.ApplyReceipt = receipt
	state.HasApplyReceipt = true
	return state, nil
}

func (t *sqlMarketUpdateApplyTx) PersistApplyCAS(
	ctx context.Context,
	current market.ModuleUpdateJobApplyPersistenceState,
	candidate market.ModuleUpdateJobApplyPersistenceState,
) error {
	result, err := t.tx.ExecContext(ctx, `
UPDATE cc_market_update_jobs
SET state=$1,state_version=$2,journal_sequence=$3
WHERE record_id=$4 AND job_id=$5 AND admission_id=$6
  AND state=$7 AND state_version=$8 AND journal_sequence=$9
  AND claim_id=$10 AND claimed_by=$11 AND claimed_from_state_version=$12
  AND preserve_user_data AND preserve_secret_material AND NOT execution_authorized`,
		string(candidate.Record.State),
		candidate.Record.StateVersion,
		candidate.Record.JournalSequence,
		current.Record.RecordID,
		current.Record.JobID,
		current.Record.AdmissionID,
		string(current.Record.State),
		current.Record.StateVersion,
		current.Record.JournalSequence,
		current.Record.ClaimID,
		current.Record.ClaimedBy,
		current.Record.ClaimedFromStateVersion,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("%w: stale Market update apply CAS", ErrMarketUpdateJobApplyConflict)
	}

	receipt := candidate.ApplyReceipt
	_, err = t.tx.ExecContext(ctx, `
INSERT INTO cc_market_update_job_apply_journal
  (receipt_id,record_id,job_id,admission_id,claim_id,worker_id,
   bundle_id,bundle_sha256,bundle_size_bytes,current_version,target_version,
   generation,migration_count,rollback_mode,require_pre_migration_snapshot,
   pre_migration_snapshot_id,from_state,to_state,from_state_version,to_state_version,
   journal_sequence,preserve_user_data,preserve_secret_material,verification_required,
   rollback_required_on_failure,execution_authorized,production_mutation_allowed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)`,
		receipt.ReceiptID,
		receipt.RecordID,
		receipt.JobID,
		receipt.AdmissionID,
		receipt.ClaimID,
		receipt.WorkerID,
		receipt.BundleID,
		receipt.BundleSHA256,
		receipt.BundleSizeBytes,
		receipt.CurrentVersion,
		receipt.TargetVersion,
		receipt.Generation,
		receipt.MigrationCount,
		receipt.RollbackMode,
		receipt.RequirePreMigrationSnapshot,
		nullableText(receipt.PreMigrationSnapshotID),
		string(receipt.FromState),
		string(receipt.ToState),
		receipt.FromStateVersion,
		receipt.ToStateVersion,
		receipt.JournalSequence,
		receipt.PreserveUserData,
		receipt.PreserveSecretMaterial,
		receipt.VerificationRequired,
		receipt.RollbackRequiredOnFailure,
		receipt.ExecutionAuthorized,
		receipt.ProductionMutationAllowed,
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: duplicate Market update apply receipt", ErrMarketUpdateJobApplyConflict)
	}
	return err
}

func (t *sqlMarketUpdateApplyTx) Commit() error   { return t.tx.Commit() }
func (t *sqlMarketUpdateApplyTx) Rollback() error { return t.tx.Rollback() }

func scanMarketUpdateJobApplyReceipt(scanner marketClaimScanner) (market.ModuleUpdateJobApplyReceipt, error) {
	var receipt market.ModuleUpdateJobApplyReceipt
	var preMigrationSnapshotID sql.NullString
	var fromState, toState string
	if err := scanner.Scan(
		&receipt.ReceiptID,
		&receipt.RecordID,
		&receipt.JobID,
		&receipt.AdmissionID,
		&receipt.ClaimID,
		&receipt.WorkerID,
		&receipt.BundleID,
		&receipt.BundleSHA256,
		&receipt.BundleSizeBytes,
		&receipt.CurrentVersion,
		&receipt.TargetVersion,
		&receipt.Generation,
		&receipt.MigrationCount,
		&receipt.RollbackMode,
		&receipt.RequirePreMigrationSnapshot,
		&preMigrationSnapshotID,
		&fromState,
		&toState,
		&receipt.FromStateVersion,
		&receipt.ToStateVersion,
		&receipt.JournalSequence,
		&receipt.PreserveUserData,
		&receipt.PreserveSecretMaterial,
		&receipt.VerificationRequired,
		&receipt.RollbackRequiredOnFailure,
		&receipt.ExecutionAuthorized,
		&receipt.ProductionMutationAllowed,
	); err != nil {
		return market.ModuleUpdateJobApplyReceipt{}, err
	}
	if preMigrationSnapshotID.Valid {
		receipt.PreMigrationSnapshotID = preMigrationSnapshotID.String
	}
	receipt.FromState = market.ModuleLifecycleJobState(fromState)
	receipt.ToState = market.ModuleLifecycleJobState(toState)
	return receipt, nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
