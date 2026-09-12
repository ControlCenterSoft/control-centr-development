package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"control-center/internal/market"
)

var ErrMarketUpdateJobVerificationConflict = errors.New("market update job verification persistence conflict")

type MarketUpdateJobVerificationRepository struct {
	backend marketUpdateVerificationBackend
}

func NewMarketUpdateJobVerificationRepository(db *sql.DB) (*MarketUpdateJobVerificationRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &MarketUpdateJobVerificationRepository{backend: sqlMarketUpdateVerificationBackend{db: db}}, nil
}

// Commit atomically persists VERIFYING -> SUCCEEDED|ROLLING_BACK and the
// immutable verification receipt. Failed verification only creates rollback
// handoff evidence; it never executes rollback or module code.
func (r *MarketUpdateJobVerificationRepository) Commit(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
	expectedStateVersion uint64,
	outcome market.ModuleUpdateJobVerificationOutcome,
	verificationEvidenceID string,
) (market.ModuleUpdateJobVerificationCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	defer tx.Rollback()

	current, err := tx.Load(ctx, record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	result, err := market.CommitModuleUpdateJobVerificationCAS(
		current,
		admission,
		expectedStateVersion,
		outcome,
		verificationEvidenceID,
	)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	if result.ExecutionAuthorized || result.ProductionMutation || result.Receipt.ExecutionAuthorized ||
		result.Receipt.ProductionMutationAllowed {
		return market.ModuleUpdateJobVerificationCommitResult{}, errors.New("Market update verification widened execution authority")
	}
	if !result.Replay {
		if !result.JournalAppendRequired || !result.State.HasVerificationReceipt {
			return market.ModuleUpdateJobVerificationCommitResult{}, errors.New("first Market update verification lacks atomic receipt evidence")
		}
		if err := tx.PersistVerificationCAS(ctx, current, result.State); err != nil {
			return market.ModuleUpdateJobVerificationCommitResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	return result, nil
}

// Reopen revalidates a committed verification result after restart. It is
// strictly read-only: pending VERIFYING work must be resumed through the apply
// recovery contract instead of inventing an outcome here.
func (r *MarketUpdateJobVerificationRepository) Reopen(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobVerificationCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	defer tx.Rollback()

	persisted, err := tx.Load(ctx, record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	if !persisted.HasVerificationReceipt {
		return market.ModuleUpdateJobVerificationCommitResult{}, errors.New("Market update verification is not committed")
	}
	receipt := persisted.VerificationReceipt
	result, err := market.CommitModuleUpdateJobVerificationCAS(
		persisted,
		admission,
		receipt.FromStateVersion,
		receipt.Outcome,
		receipt.VerificationEvidenceID,
	)
	if err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	if !result.Replay || result.JournalAppendRequired || result.State != persisted || result.Receipt != receipt {
		return market.ModuleUpdateJobVerificationCommitResult{}, errors.New("Market update verification reopen is not an exact replay")
	}
	if result.ExecutionAuthorized || result.ProductionMutation || result.Receipt.ExecutionAuthorized ||
		result.Receipt.ProductionMutationAllowed {
		return market.ModuleUpdateJobVerificationCommitResult{}, errors.New("Market update verification reopen widened execution authority")
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobVerificationCommitResult{}, err
	}
	return result, nil
}

type marketUpdateVerificationBackend interface {
	Begin(context.Context) (marketUpdateVerificationTx, error)
}

type marketUpdateVerificationTx interface {
	Load(context.Context, string) (market.ModuleUpdateJobVerificationPersistenceState, error)
	PersistVerificationCAS(
		context.Context,
		market.ModuleUpdateJobVerificationPersistenceState,
		market.ModuleUpdateJobVerificationPersistenceState,
	) error
	Commit() error
	Rollback() error
}

type sqlMarketUpdateVerificationBackend struct{ db *sql.DB }

func (b sqlMarketUpdateVerificationBackend) Begin(ctx context.Context) (marketUpdateVerificationTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlMarketUpdateVerificationTx{tx: tx}, nil
}

type sqlMarketUpdateVerificationTx struct{ tx *sql.Tx }

func (t *sqlMarketUpdateVerificationTx) Load(
	ctx context.Context,
	recordID string,
) (market.ModuleUpdateJobVerificationPersistenceState, error) {
	record, err := scanMarketUpdateJobRecord(t.tx.QueryRowContext(ctx, `
SELECT record_id,job_id,admission_id,state,state_version,journal_sequence,
       claim_id,claimed_by,claimed_from_state_version,
       preserve_user_data,preserve_secret_material,execution_authorized
FROM cc_market_update_jobs
WHERE record_id=$1
FOR UPDATE`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return market.ModuleUpdateJobVerificationPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	if err != nil {
		return market.ModuleUpdateJobVerificationPersistenceState{}, err
	}

	claimJournal, err := scanMarketUpdateJobClaimJournal(t.tx.QueryRowContext(ctx, `
SELECT journal_id,sequence,record_id,job_id,admission_id,claim_id,worker_id,
       from_state,to_state,from_state_version,to_state_version,execution_authorized
FROM cc_market_update_job_claim_journal
WHERE record_id=$1`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return market.ModuleUpdateJobVerificationPersistenceState{}, fmt.Errorf(
			"%w: verification state lacks claim journal",
			ErrMarketUpdateJobVerificationConflict,
		)
	}
	if err != nil {
		return market.ModuleUpdateJobVerificationPersistenceState{}, err
	}

	applyReceipt, err := scanMarketUpdateJobApplyReceipt(t.tx.QueryRowContext(ctx, `
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
		return market.ModuleUpdateJobVerificationPersistenceState{}, fmt.Errorf(
			"%w: verification state lacks apply receipt",
			ErrMarketUpdateJobVerificationConflict,
		)
	}
	if err != nil {
		return market.ModuleUpdateJobVerificationPersistenceState{}, err
	}

	state := market.ModuleUpdateJobVerificationPersistenceState{
		Record:       record,
		ClaimJournal: claimJournal,
		ApplyReceipt: applyReceipt,
	}
	receipt, err := scanMarketUpdateJobVerificationReceipt(t.tx.QueryRowContext(ctx, `
SELECT receipt_id,record_id,job_id,admission_id,claim_id,worker_id,
       apply_receipt_id,verification_evidence_id,outcome,from_state,to_state,
       from_state_version,to_state_version,journal_sequence,preserve_user_data,
       preserve_secret_material,rollback_required_now,terminal,execution_authorized,
       production_mutation_allowed
FROM cc_market_update_job_verification_journal
WHERE record_id=$1`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return market.ModuleUpdateJobVerificationPersistenceState{}, err
	}
	state.VerificationReceipt = receipt
	state.HasVerificationReceipt = true
	return state, nil
}

func (t *sqlMarketUpdateVerificationTx) PersistVerificationCAS(
	ctx context.Context,
	current market.ModuleUpdateJobVerificationPersistenceState,
	candidate market.ModuleUpdateJobVerificationPersistenceState,
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
		return fmt.Errorf("%w: stale Market update verification CAS", ErrMarketUpdateJobVerificationConflict)
	}

	receipt := candidate.VerificationReceipt
	_, err = t.tx.ExecContext(ctx, `
INSERT INTO cc_market_update_job_verification_journal
  (receipt_id,record_id,job_id,admission_id,claim_id,worker_id,apply_receipt_id,
   verification_evidence_id,outcome,from_state,to_state,from_state_version,to_state_version,
   journal_sequence,preserve_user_data,preserve_secret_material,rollback_required_now,terminal,
   execution_authorized,production_mutation_allowed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		receipt.ReceiptID,
		receipt.RecordID,
		receipt.JobID,
		receipt.AdmissionID,
		receipt.ClaimID,
		receipt.WorkerID,
		receipt.ApplyReceiptID,
		receipt.VerificationEvidenceID,
		string(receipt.Outcome),
		string(receipt.FromState),
		string(receipt.ToState),
		receipt.FromStateVersion,
		receipt.ToStateVersion,
		receipt.JournalSequence,
		receipt.PreserveUserData,
		receipt.PreserveSecretMaterial,
		receipt.RollbackRequiredNow,
		receipt.Terminal,
		receipt.ExecutionAuthorized,
		receipt.ProductionMutationAllowed,
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: duplicate Market update verification receipt", ErrMarketUpdateJobVerificationConflict)
	}
	return err
}

func (t *sqlMarketUpdateVerificationTx) Commit() error   { return t.tx.Commit() }
func (t *sqlMarketUpdateVerificationTx) Rollback() error { return t.tx.Rollback() }

func scanMarketUpdateJobVerificationReceipt(
	scanner marketClaimScanner,
) (market.ModuleUpdateJobVerificationReceipt, error) {
	var receipt market.ModuleUpdateJobVerificationReceipt
	var outcome, fromState, toState string
	if err := scanner.Scan(
		&receipt.ReceiptID,
		&receipt.RecordID,
		&receipt.JobID,
		&receipt.AdmissionID,
		&receipt.ClaimID,
		&receipt.WorkerID,
		&receipt.ApplyReceiptID,
		&receipt.VerificationEvidenceID,
		&outcome,
		&fromState,
		&toState,
		&receipt.FromStateVersion,
		&receipt.ToStateVersion,
		&receipt.JournalSequence,
		&receipt.PreserveUserData,
		&receipt.PreserveSecretMaterial,
		&receipt.RollbackRequiredNow,
		&receipt.Terminal,
		&receipt.ExecutionAuthorized,
		&receipt.ProductionMutationAllowed,
	); err != nil {
		return market.ModuleUpdateJobVerificationReceipt{}, err
	}
	receipt.Outcome = market.ModuleUpdateJobVerificationOutcome(outcome)
	receipt.FromState = market.ModuleLifecycleJobState(fromState)
	receipt.ToState = market.ModuleLifecycleJobState(toState)
	return receipt, nil
}
