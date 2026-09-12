package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"control-center/internal/market"
)

var (
	ErrMarketUpdateJobRollbackCompletionNotFound = errors.New("market update job rollback completion not found")
	ErrMarketUpdateJobRollbackCompletionConflict = errors.New("market update job rollback completion persistence conflict")
)

type MarketUpdateJobRollbackCompletionRepository struct {
	backend marketUpdateRollbackCompletionBackend
}

func NewMarketUpdateJobRollbackCompletionRepository(
	db *sql.DB,
) (*MarketUpdateJobRollbackCompletionRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &MarketUpdateJobRollbackCompletionRepository{
		backend: sqlMarketUpdateRollbackCompletionBackend{db: db},
	}, nil
}

// Commit atomically consumes one typed rollback attempt, advances the durable
// Market job to its terminal state, and appends the immutable completion
// receipt. The repository never invents retry, command, or migration authority.
func (r *MarketUpdateJobRollbackCompletionRepository) Commit(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
	expectedStateVersion uint64,
	expectedJournalSequence uint64,
	outcome market.ModuleUpdateJobRollbackOutcome,
) (market.ModuleUpdateJobRollbackCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	defer tx.Rollback()

	current, err := tx.Load(ctx, record.RecordID, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	result, err := market.CommitModuleUpdateJobRollbackCAS(
		current,
		admission,
		expectedStateVersion,
		expectedJournalSequence,
		outcome,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	if err := validateRollbackCompletionResultSafety(result); err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	if !result.Replay {
		if !result.AttemptConsumptionRequired || !result.JournalAppendRequired || !result.State.HasReceipt {
			return market.ModuleUpdateJobRollbackCommitResult{}, errors.New(
				"first Market rollback completion lacks atomic durable evidence",
			)
		}
		if err := tx.PersistCompletionCAS(ctx, current, result.State); err != nil {
			return market.ModuleUpdateJobRollbackCommitResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	return result, nil
}

// Reopen loads and revalidates one terminal rollback completion after restart.
// It is read-only and must replay the exact original receipt.
func (r *MarketUpdateJobRollbackCompletionRepository) Reopen(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobRollbackCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	defer tx.Rollback()

	persisted, err := tx.Load(ctx, record.RecordID, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	if !persisted.HasReceipt {
		return market.ModuleUpdateJobRollbackCommitResult{}, ErrMarketUpdateJobRollbackCompletionNotFound
	}
	result, err := market.CommitModuleUpdateJobRollbackCAS(
		persisted,
		admission,
		persisted.Attempt.ExpectedStateVersion,
		persisted.Attempt.ExpectedJournalSequence,
		persisted.Receipt.Outcome,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	if !result.Replay || result.AttemptConsumptionRequired || result.JournalAppendRequired ||
		result.State != persisted || result.Receipt != persisted.Receipt {
		return market.ModuleUpdateJobRollbackCommitResult{}, errors.New(
			"Market rollback completion reopen is not an exact replay",
		)
	}
	if err := validateRollbackCompletionResultSafety(result); err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobRollbackCommitResult{}, err
	}
	return result, nil
}

// Reconcile classifies durable state after the caller lost the Commit result.
// Even DEFINITELY_NOT_COMMITTED only requests a fresh decision; this method
// never creates or authorizes a second rollback attempt.
func (r *MarketUpdateJobRollbackCompletionRepository) Reconcile(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
	expectedOutcome market.ModuleUpdateJobRollbackOutcome,
) (market.ModuleUpdateJobRollbackReconciliationReceipt, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, err
	}
	defer tx.Rollback()

	observed, err := tx.Load(ctx, record.RecordID, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, err
	}
	source, err := market.InitializeModuleUpdateJobRollbackPersistence(
		observed.Source,
		observed.Attempt,
		admission,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"cannot reconstruct rollback pre-commit source: %w",
			err,
		)
	}
	receipt, err := market.ReconcileModuleUpdateJobRollbackCommit(
		source,
		observed,
		admission,
		expectedOutcome,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, err
	}
	if !receipt.PreserveUserData || !receipt.PreserveSecretMaterial ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.ExecutionAuthorized || receipt.GenericCommandAuthorized ||
		receipt.ProductionMutationAllowed {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, errors.New(
			"Market rollback completion reconciliation widened authority",
		)
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobRollbackReconciliationReceipt{}, err
	}
	return receipt, nil
}

func validateRollbackCompletionResultSafety(result market.ModuleUpdateJobRollbackCommitResult) error {
	receipt := result.Receipt
	if result.FurtherAttemptAuthorized || result.AutomaticRetryAuthorized ||
		result.ExecutionAuthorized || result.ProductionMutationAllowed ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.ExecutionAuthorized || receipt.GenericCommandAuthorized ||
		receipt.ProductionMutationAllowed {
		return errors.New("Market rollback completion widened execution authority")
	}
	if !receipt.AttemptConsumed || !receipt.AtomicPersistenceRequired ||
		!receipt.PreserveUserData || !receipt.PreserveSecretMaterial || !receipt.Terminal {
		return errors.New("Market rollback completion weakened terminal safety guarantees")
	}
	return nil
}

type marketUpdateRollbackCompletionBackend interface {
	Begin(context.Context) (marketUpdateRollbackCompletionTx, error)
}

type marketUpdateRollbackCompletionTx interface {
	Load(
		context.Context,
		string,
		market.ModuleUpdateJobAdmission,
	) (market.ModuleUpdateJobRollbackPersistenceState, error)
	PersistCompletionCAS(
		context.Context,
		market.ModuleUpdateJobRollbackPersistenceState,
		market.ModuleUpdateJobRollbackPersistenceState,
	) error
	Commit() error
	Rollback() error
}

type sqlMarketUpdateRollbackCompletionBackend struct{ db *sql.DB }

func (b sqlMarketUpdateRollbackCompletionBackend) Begin(
	ctx context.Context,
) (marketUpdateRollbackCompletionTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlMarketUpdateRollbackCompletionTx{tx: tx}, nil
}

type sqlMarketUpdateRollbackCompletionTx struct{ tx *sql.Tx }

func (t *sqlMarketUpdateRollbackCompletionTx) Load(
	ctx context.Context,
	recordID string,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobRollbackPersistenceState, error) {
	receipt, sourceJSON, attemptJSON, err := t.loadCompletionReceipt(ctx, recordID)
	if errors.Is(err, sql.ErrNoRows) {
		claimState, loadErr := (&sqlMarketUpdateRollbackClaimTx{tx: t.tx}).Load(ctx, recordID, admission)
		if loadErr != nil {
			return market.ModuleUpdateJobRollbackPersistenceState{}, loadErr
		}
		if !claimState.HasClaim {
			return market.ModuleUpdateJobRollbackPersistenceState{}, ErrMarketUpdateJobRollbackClaimNotFound
		}
		attempt, loadErr := market.AdmitModuleUpdateJobRollbackAttempt(claimState, admission)
		if loadErr != nil {
			return market.ModuleUpdateJobRollbackPersistenceState{}, loadErr
		}
		return market.InitializeModuleUpdateJobRollbackPersistence(claimState, attempt, admission)
	}
	if err != nil {
		return market.ModuleUpdateJobRollbackPersistenceState{}, err
	}

	var source market.ModuleUpdateJobRollbackClaimPersistenceState
	if err := json.Unmarshal(sourceJSON, &source); err != nil {
		return market.ModuleUpdateJobRollbackPersistenceState{}, fmt.Errorf(
			"decode durable rollback completion source: %w",
			err,
		)
	}
	var attempt market.ModuleUpdateJobRollbackAttempt
	if err := json.Unmarshal(attemptJSON, &attempt); err != nil {
		return market.ModuleUpdateJobRollbackPersistenceState{}, fmt.Errorf(
			"decode durable rollback completion attempt: %w",
			err,
		)
	}
	record, err := scanMarketUpdateJobRecord(t.tx.QueryRowContext(ctx, `
SELECT record_id,job_id,admission_id,state,state_version,journal_sequence,
       claim_id,claimed_by,claimed_from_state_version,
       preserve_user_data,preserve_secret_material,execution_authorized
FROM cc_market_update_jobs
WHERE record_id=$1
FOR UPDATE`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return market.ModuleUpdateJobRollbackPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	if err != nil {
		return market.ModuleUpdateJobRollbackPersistenceState{}, err
	}
	state := market.ModuleUpdateJobRollbackPersistenceState{
		Source:     source,
		Attempt:    attempt,
		Record:     record,
		Receipt:    receipt,
		HasReceipt: true,
	}
	validated, err := market.CommitModuleUpdateJobRollbackCAS(
		state,
		admission,
		attempt.ExpectedStateVersion,
		attempt.ExpectedJournalSequence,
		receipt.Outcome,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackPersistenceState{}, fmt.Errorf(
			"invalid durable rollback completion: %w",
			err,
		)
	}
	if !validated.Replay || validated.State != state || validated.Receipt != receipt {
		return market.ModuleUpdateJobRollbackPersistenceState{}, errors.New(
			"durable rollback completion does not exact-replay",
		)
	}
	return state, nil
}

func (t *sqlMarketUpdateRollbackCompletionTx) loadCompletionReceipt(
	ctx context.Context,
	recordID string,
) (market.ModuleUpdateJobRollbackReceipt, []byte, []byte, error) {
	var receipt market.ModuleUpdateJobRollbackReceipt
	var outcome, fromState, toState string
	var sourceJSON, attemptJSON []byte
	err := t.tx.QueryRowContext(ctx, `
SELECT receipt_id,attempt_id,claim_id,rollback_admission_id,record_id,job_id,
       source_admission_id,verification_receipt_id,worker_id,module_id,
       restore_version,failed_target_version,generation,rollback_mode,
       pre_migration_snapshot_id,outcome,from_state,to_state,from_state_version,
       to_state_version,journal_sequence,attempt_consumed,atomic_persistence_required,
       preserve_user_data,preserve_secret_material,terminal,recovery_required,
       further_attempt_authorized,automatic_retry_authorized,execution_authorized,
       generic_command_authorized,production_mutation_allowed,source_state,attempt
FROM cc_market_update_job_rollback_completion_journal
WHERE record_id=$1`, recordID).Scan(
		&receipt.ReceiptID,
		&receipt.AttemptID,
		&receipt.ClaimID,
		&receipt.RollbackAdmissionID,
		&receipt.RecordID,
		&receipt.JobID,
		&receipt.SourceAdmissionID,
		&receipt.VerificationReceiptID,
		&receipt.WorkerID,
		&receipt.ModuleID,
		&receipt.RestoreVersion,
		&receipt.FailedTargetVersion,
		&receipt.Generation,
		&receipt.RollbackMode,
		&receipt.PreMigrationSnapshotID,
		&outcome,
		&fromState,
		&toState,
		&receipt.FromStateVersion,
		&receipt.ToStateVersion,
		&receipt.JournalSequence,
		&receipt.AttemptConsumed,
		&receipt.AtomicPersistenceRequired,
		&receipt.PreserveUserData,
		&receipt.PreserveSecretMaterial,
		&receipt.Terminal,
		&receipt.RecoveryRequired,
		&receipt.FurtherAttemptAuthorized,
		&receipt.AutomaticRetryAuthorized,
		&receipt.ExecutionAuthorized,
		&receipt.GenericCommandAuthorized,
		&receipt.ProductionMutationAllowed,
		&sourceJSON,
		&attemptJSON,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackReceipt{}, nil, nil, err
	}
	receipt.Outcome = market.ModuleUpdateJobRollbackOutcome(outcome)
	receipt.FromState = market.ModuleLifecycleJobState(fromState)
	receipt.ToState = market.ModuleLifecycleJobState(toState)
	return receipt, sourceJSON, attemptJSON, nil
}

func (t *sqlMarketUpdateRollbackCompletionTx) PersistCompletionCAS(
	ctx context.Context,
	current market.ModuleUpdateJobRollbackPersistenceState,
	candidate market.ModuleUpdateJobRollbackPersistenceState,
) error {
	if current.HasReceipt || !candidate.HasReceipt || candidate.Source != current.Source ||
		candidate.Attempt != current.Attempt {
		return fmt.Errorf(
			"%w: invalid Market rollback completion persistence transition",
			ErrMarketUpdateJobRollbackCompletionConflict,
		)
	}
	if current.Record.State != market.ModuleJobStateRollingBack ||
		candidate.Record.StateVersion != current.Record.StateVersion+1 ||
		candidate.Record.JournalSequence != current.Attempt.AttemptSequence {
		return fmt.Errorf(
			"%w: invalid Market rollback completion revision transition",
			ErrMarketUpdateJobRollbackCompletionConflict,
		)
	}

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
		return fmt.Errorf(
			"%w: stale Market rollback completion CAS",
			ErrMarketUpdateJobRollbackCompletionConflict,
		)
	}

	sourceJSON, err := json.Marshal(current.Source)
	if err != nil {
		return fmt.Errorf("encode rollback completion source: %w", err)
	}
	attemptJSON, err := json.Marshal(current.Attempt)
	if err != nil {
		return fmt.Errorf("encode rollback completion attempt: %w", err)
	}
	receipt := candidate.Receipt
	_, err = t.tx.ExecContext(ctx, `
INSERT INTO cc_market_update_job_rollback_completion_journal
  (receipt_id,attempt_id,claim_id,rollback_admission_id,record_id,job_id,
   source_admission_id,verification_receipt_id,worker_id,module_id,restore_version,
   failed_target_version,generation,rollback_mode,pre_migration_snapshot_id,outcome,
   from_state,to_state,from_state_version,to_state_version,journal_sequence,
   attempt_consumed,atomic_persistence_required,preserve_user_data,
   preserve_secret_material,terminal,recovery_required,further_attempt_authorized,
   automatic_retry_authorized,execution_authorized,generic_command_authorized,
   production_mutation_allowed,source_state,attempt)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
        $19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34)`,
		receipt.ReceiptID,
		receipt.AttemptID,
		receipt.ClaimID,
		receipt.RollbackAdmissionID,
		receipt.RecordID,
		receipt.JobID,
		receipt.SourceAdmissionID,
		receipt.VerificationReceiptID,
		receipt.WorkerID,
		receipt.ModuleID,
		receipt.RestoreVersion,
		receipt.FailedTargetVersion,
		receipt.Generation,
		receipt.RollbackMode,
		receipt.PreMigrationSnapshotID,
		string(receipt.Outcome),
		string(receipt.FromState),
		string(receipt.ToState),
		receipt.FromStateVersion,
		receipt.ToStateVersion,
		receipt.JournalSequence,
		receipt.AttemptConsumed,
		receipt.AtomicPersistenceRequired,
		receipt.PreserveUserData,
		receipt.PreserveSecretMaterial,
		receipt.Terminal,
		receipt.RecoveryRequired,
		receipt.FurtherAttemptAuthorized,
		receipt.AutomaticRetryAuthorized,
		receipt.ExecutionAuthorized,
		receipt.GenericCommandAuthorized,
		receipt.ProductionMutationAllowed,
		sourceJSON,
		attemptJSON,
	)
	if isUniqueViolation(err) {
		return fmt.Errorf(
			"%w: rollback attempt already consumed",
			ErrMarketUpdateJobRollbackCompletionConflict,
		)
	}
	return err
}

func (t *sqlMarketUpdateRollbackCompletionTx) Commit() error   { return t.tx.Commit() }
func (t *sqlMarketUpdateRollbackCompletionTx) Rollback() error { return t.tx.Rollback() }
