package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"control-center/internal/market"
)

var (
	ErrMarketUpdateJobClaimNotFound = errors.New("market update job claim record not found")
	ErrMarketUpdateJobClaimConflict = errors.New("market update job claim persistence conflict")
)

type MarketUpdateJobClaimRepository struct {
	backend marketUpdateClaimBackend
}

func NewMarketUpdateJobClaimRepository(db *sql.DB) (*MarketUpdateJobClaimRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &MarketUpdateJobClaimRepository{backend: sqlMarketUpdateClaimBackend{db: db}}, nil
}

// Initialize persists the deterministic PREPARED record exactly once. Replays
// never reset an already RUNNING claim and always revalidate persisted evidence.
func (r *MarketUpdateJobClaimRepository) Initialize(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobClaimPersistenceState, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}
	prepared, err := market.InitializeModuleUpdateJobClaimPersistence(record, admission)
	if err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}

	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}
	defer tx.Rollback()

	if err := tx.InsertPrepared(ctx, prepared.Record); err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}
	persisted, err := tx.Load(ctx, prepared.Record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}
	switch persisted.Record.State {
	case market.ModuleJobStatePrepared:
		if _, err := market.InitializeModuleUpdateJobClaimPersistence(persisted.Record, admission); err != nil {
			return market.ModuleUpdateJobClaimPersistenceState{}, err
		}
		if persisted.HasJournal || persisted.Journal != (market.ModuleUpdateJobClaimJournalEntry{}) {
			return market.ModuleUpdateJobClaimPersistenceState{}, fmt.Errorf("prepared Market update job contains journal evidence")
		}
	case market.ModuleJobStateRunning:
		if _, err := market.ReopenModuleUpdateJobClaimPersistence(persisted, admission); err != nil {
			return market.ModuleUpdateJobClaimPersistenceState{}, err
		}
	default:
		return market.ModuleUpdateJobClaimPersistenceState{}, fmt.Errorf("unsupported persisted Market update job state %q", persisted.Record.State)
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}
	return persisted, nil
}

// Claim atomically persists the PREPARED -> RUNNING CAS and its immutable
// journal entry. It never executes a module operation or migration.
func (r *MarketUpdateJobClaimRepository) Claim(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
	workerID string,
	expectedStateVersion uint64,
) (market.ModuleUpdateJobClaimCommitResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobClaimCommitResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobClaimCommitResult{}, err
	}
	defer tx.Rollback()

	current, err := tx.Load(ctx, record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobClaimCommitResult{}, err
	}
	result, err := market.CommitModuleUpdateJobClaimCAS(current, admission, workerID, expectedStateVersion)
	if err != nil {
		return market.ModuleUpdateJobClaimCommitResult{}, err
	}
	if result.ExecutionAuthorized || result.ProductionMutation || result.Claim.ExecutionAuthorized || result.Claim.ProductionMutationAllowed {
		return market.ModuleUpdateJobClaimCommitResult{}, errors.New("Market update claim widened execution authority")
	}
	if !result.Replay {
		if !result.JournalAppendRequired || !result.State.HasJournal {
			return market.ModuleUpdateJobClaimCommitResult{}, errors.New("first Market update claim lacks atomic journal evidence")
		}
		if err := tx.PersistClaimCAS(ctx, current, result.State); err != nil {
			return market.ModuleUpdateJobClaimCommitResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobClaimCommitResult{}, err
	}
	return result, nil
}

// Reopen loads one committed record+journal snapshot and revalidates the exact
// claim after process restart. It is read-only with respect to Market state.
func (r *MarketUpdateJobClaimRepository) Reopen(
	ctx context.Context,
	admission market.ModuleUpdateJobAdmission,
) (market.ModuleUpdateJobClaimResult, error) {
	record, err := market.PrepareModuleUpdateJobRecord(admission)
	if err != nil {
		return market.ModuleUpdateJobClaimResult{}, err
	}
	tx, err := r.backend.Begin(ctx)
	if err != nil {
		return market.ModuleUpdateJobClaimResult{}, err
	}
	defer tx.Rollback()

	persisted, err := tx.Load(ctx, record.RecordID)
	if err != nil {
		return market.ModuleUpdateJobClaimResult{}, err
	}
	result, err := market.ReopenModuleUpdateJobClaimPersistence(persisted, admission)
	if err != nil {
		return market.ModuleUpdateJobClaimResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return market.ModuleUpdateJobClaimResult{}, err
	}
	return result, nil
}

type marketUpdateClaimBackend interface {
	Begin(context.Context) (marketUpdateClaimTx, error)
}

type marketUpdateClaimTx interface {
	InsertPrepared(context.Context, market.ModuleUpdateJobRecord) error
	Load(context.Context, string) (market.ModuleUpdateJobClaimPersistenceState, error)
	PersistClaimCAS(context.Context, market.ModuleUpdateJobClaimPersistenceState, market.ModuleUpdateJobClaimPersistenceState) error
	Commit() error
	Rollback() error
}

type sqlMarketUpdateClaimBackend struct{ db *sql.DB }

func (b sqlMarketUpdateClaimBackend) Begin(ctx context.Context) (marketUpdateClaimTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlMarketUpdateClaimTx{tx: tx}, nil
}

type sqlMarketUpdateClaimTx struct{ tx *sql.Tx }

func (t *sqlMarketUpdateClaimTx) InsertPrepared(ctx context.Context, record market.ModuleUpdateJobRecord) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO cc_market_update_jobs
  (record_id,job_id,admission_id,state,state_version,journal_sequence,
   preserve_user_data,preserve_secret_material,execution_authorized)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (record_id) DO NOTHING`,
		record.RecordID,
		record.JobID,
		record.AdmissionID,
		string(record.State),
		record.StateVersion,
		record.JournalSequence,
		record.PreserveUserData,
		record.PreserveSecretMaterial,
		record.ExecutionAuthorized,
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: conflicting prepared Market update record", ErrMarketUpdateJobClaimConflict)
	}
	return err
}

func (t *sqlMarketUpdateClaimTx) Load(
	ctx context.Context,
	recordID string,
) (market.ModuleUpdateJobClaimPersistenceState, error) {
	record, err := scanMarketUpdateJobRecord(t.tx.QueryRowContext(ctx, `
SELECT record_id,job_id,admission_id,state,state_version,journal_sequence,
       claim_id,claimed_by,claimed_from_state_version,
       preserve_user_data,preserve_secret_material,execution_authorized
FROM cc_market_update_jobs
WHERE record_id=$1
FOR UPDATE`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return market.ModuleUpdateJobClaimPersistenceState{}, ErrMarketUpdateJobClaimNotFound
	}
	if err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}

	state := market.ModuleUpdateJobClaimPersistenceState{Record: record}
	journal, err := scanMarketUpdateJobClaimJournal(t.tx.QueryRowContext(ctx, `
SELECT journal_id,sequence,record_id,job_id,admission_id,claim_id,worker_id,
       from_state,to_state,from_state_version,to_state_version,execution_authorized
FROM cc_market_update_job_claim_journal
WHERE record_id=$1`, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return market.ModuleUpdateJobClaimPersistenceState{}, err
	}
	state.Journal = journal
	state.HasJournal = true
	return state, nil
}

func (t *sqlMarketUpdateClaimTx) PersistClaimCAS(
	ctx context.Context,
	current market.ModuleUpdateJobClaimPersistenceState,
	candidate market.ModuleUpdateJobClaimPersistenceState,
) error {
	result, err := t.tx.ExecContext(ctx, `
UPDATE cc_market_update_jobs
SET state=$1,state_version=$2,journal_sequence=$3,claim_id=$4,claimed_by=$5,
    claimed_from_state_version=$6
WHERE record_id=$7 AND job_id=$8 AND admission_id=$9
  AND state=$10 AND state_version=$11 AND journal_sequence=$12
  AND preserve_user_data AND preserve_secret_material AND NOT execution_authorized`,
		string(candidate.Record.State),
		candidate.Record.StateVersion,
		candidate.Record.JournalSequence,
		candidate.Record.ClaimID,
		candidate.Record.ClaimedBy,
		candidate.Record.ClaimedFromStateVersion,
		current.Record.RecordID,
		current.Record.JobID,
		current.Record.AdmissionID,
		string(current.Record.State),
		current.Record.StateVersion,
		current.Record.JournalSequence,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("%w: stale Market update claim CAS", ErrMarketUpdateJobClaimConflict)
	}

	journal := candidate.Journal
	_, err = t.tx.ExecContext(ctx, `
INSERT INTO cc_market_update_job_claim_journal
  (journal_id,sequence,record_id,job_id,admission_id,claim_id,worker_id,
   from_state,to_state,from_state_version,to_state_version,execution_authorized)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		journal.JournalID,
		journal.Sequence,
		journal.RecordID,
		journal.JobID,
		journal.AdmissionID,
		journal.ClaimID,
		journal.WorkerID,
		string(journal.FromState),
		string(journal.ToState),
		journal.FromStateVersion,
		journal.ToStateVersion,
		journal.ExecutionAuthorized,
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: duplicate Market update claim journal", ErrMarketUpdateJobClaimConflict)
	}
	return err
}

func (t *sqlMarketUpdateClaimTx) Commit() error   { return t.tx.Commit() }
func (t *sqlMarketUpdateClaimTx) Rollback() error { return t.tx.Rollback() }

type marketClaimScanner interface{ Scan(...any) error }

func scanMarketUpdateJobRecord(scanner marketClaimScanner) (market.ModuleUpdateJobRecord, error) {
	var record market.ModuleUpdateJobRecord
	var state string
	var claimID, claimedBy sql.NullString
	var claimedFrom sql.NullInt64
	if err := scanner.Scan(
		&record.RecordID,
		&record.JobID,
		&record.AdmissionID,
		&state,
		&record.StateVersion,
		&record.JournalSequence,
		&claimID,
		&claimedBy,
		&claimedFrom,
		&record.PreserveUserData,
		&record.PreserveSecretMaterial,
		&record.ExecutionAuthorized,
	); err != nil {
		return market.ModuleUpdateJobRecord{}, err
	}
	if claimedFrom.Valid && claimedFrom.Int64 < 0 {
		return market.ModuleUpdateJobRecord{}, errors.New("negative persisted Market update state version")
	}
	record.State = market.ModuleLifecycleJobState(state)
	if claimID.Valid {
		record.ClaimID = claimID.String
	}
	if claimedBy.Valid {
		record.ClaimedBy = claimedBy.String
	}
	if claimedFrom.Valid {
		record.ClaimedFromStateVersion = uint64(claimedFrom.Int64)
	}
	return record, nil
}

func scanMarketUpdateJobClaimJournal(scanner marketClaimScanner) (market.ModuleUpdateJobClaimJournalEntry, error) {
	var entry market.ModuleUpdateJobClaimJournalEntry
	var fromState, toState string
	if err := scanner.Scan(
		&entry.JournalID,
		&entry.Sequence,
		&entry.RecordID,
		&entry.JobID,
		&entry.AdmissionID,
		&entry.ClaimID,
		&entry.WorkerID,
		&fromState,
		&toState,
		&entry.FromStateVersion,
		&entry.ToStateVersion,
		&entry.ExecutionAuthorized,
	); err != nil {
		return market.ModuleUpdateJobClaimJournalEntry{}, err
	}
	entry.FromState = market.ModuleLifecycleJobState(fromState)
	entry.ToState = market.ModuleLifecycleJobState(toState)
	return entry, nil
}
