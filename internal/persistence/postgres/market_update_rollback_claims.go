package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"control-center/internal/market"
)

var (
	ErrMarketUpdateJobRollbackClaimNotFound = errors.New("market update job rollback claim not found")
	ErrMarketUpdateJobRollbackClaimConflict = errors.New("market update job rollback claim persistence conflict")
)

type MarketUpdateJobRollbackClaimRepository struct {
	backend marketUpdateRollbackClaimBackend
}

func NewMarketUpdateJobRollbackClaimRepository(db *sql.DB) (*MarketUpdateJobRollbackClaimRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &MarketUpdateJobRollbackClaimRepository{backend: sqlMarketUpdateRollbackClaimBackend{db: db}}, nil
}

// Claim atomically appends one rollback claim against the exact durable failed-
// verification revision. The claim is evidence only and never authorizes or
// executes rollback, migrations, module code, or any production mutation.
func (r *MarketUpdateJobRollbackClaimRepository) Claim(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
	workerID string,
	expectedStateVersion uint64,
	expectedJournalSequence uint64,
) (market.ModuleUpdateJobRollbackClaimCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	defer tx.Rollback()

	current, err := tx.Load(ctx, record.RecordID, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	result, err := market.CommitModuleUpdateJobRollbackClaimCAS(
		current,
		admission,
		workerID,
		expectedStateVersion,
		expectedJournalSequence,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	if result.ExecutionAuthorized || result.ProductionMutation ||
		result.Claim.ExecutionAuthorized || result.Claim.ProductionMutationAllowed {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, errors.New(
			"Market rollback claim widened execution authority",
		)
	}
	if !result.Claim.PreserveUserData || !result.Claim.PreserveSecretMaterial {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, errors.New(
			"Market rollback claim dropped preservation guarantees",
		)
	}
	if !result.Replay {
		if !result.ClaimAppendRequired || !result.State.HasClaim {
			return market.ModuleUpdateJobRollbackClaimCommitResult{}, errors.New(
				"first Market rollback claim lacks atomic durable evidence",
			)
		}
		if err := tx.PersistClaimCAS(ctx, current, result.State); err != nil {
			return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	return result, nil
}

// Reopen loads and revalidates one committed rollback claim after restart. It
// must replay exactly and cannot synthesize a new worker claim or authority.
func (r *MarketUpdateJobRollbackClaimRepository) Reopen(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobRollbackClaimCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	defer tx.Rollback()

	persisted, err := tx.Load(ctx, record.RecordID, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	if !persisted.HasClaim {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, ErrMarketUpdateJobRollbackClaimNotFound
	}
	claim, err := market.ReopenModuleUpdateJobRollbackClaimPersistence(persisted, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	result, err := market.CommitModuleUpdateJobRollbackClaimCAS(
		persisted,
		admission,
		claim.WorkerID,
		claim.ExpectedStateVersion,
		claim.ExpectedJournalSequence,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	if !result.Replay || result.ClaimAppendRequired || result.State != persisted || result.Claim != claim {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, errors.New(
			"Market rollback claim reopen is not an exact replay",
		)
	}
	if result.ExecutionAuthorized || result.ProductionMutation ||
		result.Claim.ExecutionAuthorized || result.Claim.ProductionMutationAllowed {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, errors.New(
			"Market rollback claim reopen widened execution authority",
		)
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	return result, nil
}

type marketUpdateRollbackClaimBackend interface {
	Begin(context.Context) (marketUpdateRollbackClaimTx, error)
}

type marketUpdateRollbackClaimTx interface {
	Load(
		context.Context,
		string,
		market.ModuleUpdateJobAdmission,
	) (market.ModuleUpdateJobRollbackClaimPersistenceState, error)
	PersistClaimCAS(
		context.Context,
		market.ModuleUpdateJobRollbackClaimPersistenceState,
		market.ModuleUpdateJobRollbackClaimPersistenceState,
	) error
	Commit() error
	Rollback() error
}

type sqlMarketUpdateRollbackClaimBackend struct{ db *sql.DB }

func (b sqlMarketUpdateRollbackClaimBackend) Begin(ctx context.Context) (marketUpdateRollbackClaimTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlMarketUpdateRollbackClaimTx{tx: tx}, nil
}

type sqlMarketUpdateRollbackClaimTx struct{ tx *sql.Tx }

func (t *sqlMarketUpdateRollbackClaimTx) Load(
	ctx context.Context,
	recordID string,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobRollbackClaimPersistenceState, error) {
	verificationState, err := (&sqlMarketUpdateVerificationTx{tx: t.tx}).Load(ctx, recordID)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimPersistenceState{}, err
	}
	rollbackAdmission, err := market.PlanModuleUpdateJobRollbackAdmission(verificationState, admission)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimPersistenceState{}, fmt.Errorf(
			"cannot derive rollback admission from durable verification evidence: %w",
			err,
		)
	}
	state, err := market.InitializeModuleUpdateJobRollbackClaimPersistence(
		verificationState,
		rollbackAdmission,
		admission,
	)
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimPersistenceState{}, err
	}

	claim, err := scanMarketUpdateJobRollbackClaim(t.tx.QueryRowContext(ctx, `
SELECT claim_id,rollback_admission_id,record_id,job_id,source_admission_id,
       verification_receipt_id,worker_id,expected_state_version,
       expected_journal_sequence,claim_sequence,single_use,
       atomic_persistence_required,fresh_revalidation_required,preserve_user_data,
       preserve_secret_material,execution_authorized,production_mutation_allowed
FROM cc_market_update_job_rollback_claims
WHERE record_id=$1`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return market.ModuleUpdateJobRollbackClaimPersistenceState{}, err
	}
	state.Claim = claim
	state.HasClaim = true
	if _, err := market.ReopenModuleUpdateJobRollbackClaimPersistence(state, admission); err != nil {
		return market.ModuleUpdateJobRollbackClaimPersistenceState{}, fmt.Errorf(
			"invalid durable rollback claim: %w",
			err,
		)
	}
	return state, nil
}

func (t *sqlMarketUpdateRollbackClaimTx) PersistClaimCAS(
	ctx context.Context,
	current market.ModuleUpdateJobRollbackClaimPersistenceState,
	candidate market.ModuleUpdateJobRollbackClaimPersistenceState,
) error {
	if current.HasClaim || !candidate.HasClaim ||
		candidate.Source != current.Source || candidate.RollbackAdmission != current.RollbackAdmission {
		return fmt.Errorf("%w: invalid Market rollback claim persistence transition", ErrMarketUpdateJobRollbackClaimConflict)
	}
	claim := candidate.Claim
	result, err := t.tx.ExecContext(ctx, `
INSERT INTO cc_market_update_job_rollback_claims
  (claim_id,rollback_admission_id,record_id,job_id,source_admission_id,
   verification_receipt_id,worker_id,expected_state_version,
   expected_journal_sequence,claim_sequence,single_use,
   atomic_persistence_required,fresh_revalidation_required,preserve_user_data,
   preserve_secret_material,execution_authorized,production_mutation_allowed)
SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
FROM cc_market_update_jobs j
JOIN cc_market_update_job_verification_journal v
  ON v.record_id = j.record_id
 AND v.receipt_id = $6
WHERE j.record_id = $3
  AND j.job_id = $4
  AND j.admission_id = $5
  AND j.state = 'ROLLING_BACK'
  AND j.state_version = $8
  AND j.journal_sequence = $9
  AND j.preserve_user_data
  AND j.preserve_secret_material
  AND NOT j.execution_authorized
  AND v.job_id = j.job_id
  AND v.admission_id = j.admission_id
  AND v.outcome = 'FAILED'
  AND v.to_state = 'ROLLING_BACK'
  AND v.to_state_version = j.state_version
  AND v.journal_sequence = j.journal_sequence
  AND v.preserve_user_data
  AND v.preserve_secret_material
  AND v.rollback_required_now
  AND NOT v.terminal
  AND NOT v.execution_authorized
  AND NOT v.production_mutation_allowed`,
		claim.ClaimID,
		claim.RollbackAdmissionID,
		claim.RecordID,
		claim.JobID,
		claim.SourceAdmissionID,
		claim.VerificationReceiptID,
		claim.WorkerID,
		claim.ExpectedStateVersion,
		claim.ExpectedJournalSequence,
		claim.ClaimSequence,
		claim.SingleUse,
		claim.AtomicPersistenceRequired,
		claim.FreshRevalidationRequired,
		claim.PreserveUserData,
		claim.PreserveSecretMaterial,
		claim.ExecutionAuthorized,
		claim.ProductionMutationAllowed,
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: rollback admission already claimed", ErrMarketUpdateJobRollbackClaimConflict)
	}
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("%w: stale Market rollback claim CAS", ErrMarketUpdateJobRollbackClaimConflict)
	}
	return nil
}

func (t *sqlMarketUpdateRollbackClaimTx) Commit() error   { return t.tx.Commit() }
func (t *sqlMarketUpdateRollbackClaimTx) Rollback() error { return t.tx.Rollback() }

func scanMarketUpdateJobRollbackClaim(scanner marketClaimScanner) (market.ModuleUpdateJobRollbackClaim, error) {
	var claim market.ModuleUpdateJobRollbackClaim
	if err := scanner.Scan(
		&claim.ClaimID,
		&claim.RollbackAdmissionID,
		&claim.RecordID,
		&claim.JobID,
		&claim.SourceAdmissionID,
		&claim.VerificationReceiptID,
		&claim.WorkerID,
		&claim.ExpectedStateVersion,
		&claim.ExpectedJournalSequence,
		&claim.ClaimSequence,
		&claim.SingleUse,
		&claim.AtomicPersistenceRequired,
		&claim.FreshRevalidationRequired,
		&claim.PreserveUserData,
		&claim.PreserveSecretMaterial,
		&claim.ExecutionAuthorized,
		&claim.ProductionMutationAllowed,
	); err != nil {
		return market.ModuleUpdateJobRollbackClaim{}, err
	}
	return claim, nil
}
