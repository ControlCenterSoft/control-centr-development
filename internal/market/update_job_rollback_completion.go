package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type ModuleUpdateJobRollbackOutcome string

const (
	ModuleUpdateJobRollbackSucceeded ModuleUpdateJobRollbackOutcome = "SUCCEEDED"
	ModuleUpdateJobRollbackFailed    ModuleUpdateJobRollbackOutcome = "FAILED"
)

// ModuleUpdateJobRollbackReceipt is immutable evidence that exactly one typed
// rollback attempt reached a terminal lifecycle outcome. It is not reusable as
// rollback, retry, command, or production-mutation authority.
type ModuleUpdateJobRollbackReceipt struct {
	ReceiptID                 string                         `json:"receipt_id"`
	AttemptID                 string                         `json:"attempt_id"`
	ClaimID                   string                         `json:"claim_id"`
	RollbackAdmissionID       string                         `json:"rollback_admission_id"`
	RecordID                  string                         `json:"record_id"`
	JobID                     string                         `json:"job_id"`
	SourceAdmissionID         string                         `json:"source_admission_id"`
	VerificationReceiptID     string                         `json:"verification_receipt_id"`
	WorkerID                  string                         `json:"worker_id"`
	ModuleID                  string                         `json:"module_id"`
	RestoreVersion            string                         `json:"restore_version"`
	FailedTargetVersion       string                         `json:"failed_target_version"`
	Generation                uint64                         `json:"generation"`
	RollbackMode              string                         `json:"rollback_mode"`
	PreMigrationSnapshotID    string                         `json:"pre_migration_snapshot_id,omitempty"`
	Outcome                   ModuleUpdateJobRollbackOutcome `json:"outcome"`
	FromState                 ModuleLifecycleJobState        `json:"from_state"`
	ToState                   ModuleLifecycleJobState        `json:"to_state"`
	FromStateVersion          uint64                         `json:"from_state_version"`
	ToStateVersion            uint64                         `json:"to_state_version"`
	JournalSequence           uint64                         `json:"journal_sequence"`
	AttemptConsumed           bool                           `json:"attempt_consumed"`
	AtomicPersistenceRequired bool                           `json:"atomic_persistence_required"`
	PreserveUserData          bool                           `json:"preserve_user_data"`
	PreserveSecretMaterial    bool                           `json:"preserve_secret_material"`
	Terminal                  bool                           `json:"terminal"`
	RecoveryRequired          bool                           `json:"recovery_required"`
	FurtherAttemptAuthorized  bool                           `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized  bool                           `json:"automatic_retry_authorized"`
	ExecutionAuthorized       bool                           `json:"execution_authorized"`
	GenericCommandAuthorized  bool                           `json:"generic_command_authorized"`
	ProductionMutationAllowed bool                           `json:"production_mutation_allowed"`
}

// ModuleUpdateJobRollbackPersistenceState retains the immutable pre-rollback
// evidence separately from the authoritative terminal job record. Receipt is
// appended exactly once together with consuming Attempt.
type ModuleUpdateJobRollbackPersistenceState struct {
	Source     ModuleUpdateJobRollbackClaimPersistenceState `json:"source"`
	Attempt    ModuleUpdateJobRollbackAttempt               `json:"attempt"`
	Record     ModuleUpdateJobRecord                        `json:"record"`
	Receipt    ModuleUpdateJobRollbackReceipt               `json:"receipt"`
	HasReceipt bool                                         `json:"has_receipt"`
}

type ModuleUpdateJobRollbackCommitResult struct {
	State                      ModuleUpdateJobRollbackPersistenceState `json:"state"`
	Receipt                    ModuleUpdateJobRollbackReceipt          `json:"receipt"`
	Replay                     bool                                    `json:"replay"`
	AttemptConsumptionRequired bool                                    `json:"attempt_consumption_required"`
	JournalAppendRequired      bool                                    `json:"journal_append_required"`
	RecoveryRequired           bool                                    `json:"recovery_required"`
	FurtherAttemptAuthorized   bool                                    `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized   bool                                    `json:"automatic_retry_authorized"`
	ExecutionAuthorized        bool                                    `json:"execution_authorized"`
	ProductionMutationAllowed  bool                                    `json:"production_mutation_allowed"`
}

func InitializeModuleUpdateJobRollbackPersistence(
	source ModuleUpdateJobRollbackClaimPersistenceState,
	attempt ModuleUpdateJobRollbackAttempt,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobRollbackPersistenceState, error) {
	if err := ValidateModuleUpdateJobRollbackAttempt(attempt, source, admission); err != nil {
		return ModuleUpdateJobRollbackPersistenceState{}, fmt.Errorf("invalid rollback attempt: %w", err)
	}
	state := ModuleUpdateJobRollbackPersistenceState{
		Source:  source,
		Attempt: attempt,
		Record:  source.Source.Record,
	}
	if err := validateModuleUpdateJobRollbackPersistenceState(state, admission); err != nil {
		return ModuleUpdateJobRollbackPersistenceState{}, err
	}
	return state, nil
}

// CommitModuleUpdateJobRollbackCAS consumes the exact typed rollback attempt and
// proposes one atomic terminal state+receipt write. Persistence must CAS using
// the original ROLLING_BACK state/journal revision and append the receipt in the
// same transaction. Exact replay is read-only and cannot mint another attempt.
func CommitModuleUpdateJobRollbackCAS(
	current ModuleUpdateJobRollbackPersistenceState,
	admission ModuleUpdateJobAdmission,
	expectedStateVersion uint64,
	expectedJournalSequence uint64,
	outcome ModuleUpdateJobRollbackOutcome,
) (ModuleUpdateJobRollbackCommitResult, error) {
	if err := validateModuleUpdateJobRollbackPersistenceState(current, admission); err != nil {
		return ModuleUpdateJobRollbackCommitResult{}, err
	}
	if expectedStateVersion == 0 || expectedJournalSequence == 0 {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback commit requires non-zero expected revisions")
	}
	if expectedStateVersion != current.Attempt.ExpectedStateVersion ||
		expectedJournalSequence != current.Attempt.ExpectedJournalSequence {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("stale rollback commit revision")
	}

	event, err := moduleUpdateRollbackEvent(outcome)
	if err != nil {
		return ModuleUpdateJobRollbackCommitResult{}, err
	}

	if current.HasReceipt {
		rebuilt, err := buildModuleUpdateJobRollbackReceipt(current.Source, current.Attempt, outcome)
		if err != nil {
			return ModuleUpdateJobRollbackCommitResult{}, err
		}
		if current.Receipt != rebuilt {
			return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("persisted rollback receipt evidence mismatch")
		}
		return ModuleUpdateJobRollbackCommitResult{
			State:                      current,
			Receipt:                    current.Receipt,
			Replay:                     true,
			AttemptConsumptionRequired: false,
			JournalAppendRequired:      false,
			RecoveryRequired:           current.Receipt.RecoveryRequired,
			FurtherAttemptAuthorized:   false,
			AutomaticRetryAuthorized:   false,
			ExecutionAuthorized:        false,
			ProductionMutationAllowed:  false,
		}, nil
	}

	if current.Record != current.Source.Source.Record {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback commit source record drift")
	}
	if current.Record.State != ModuleJobStateRollingBack ||
		current.Record.StateVersion != expectedStateVersion ||
		current.Record.JournalSequence != expectedJournalSequence {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback commit requires exact fresh ROLLING_BACK revision")
	}
	if current.Record.StateVersion == ^uint64(0) {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback commit state version overflow")
	}
	if current.Attempt.AttemptSequence <= current.Attempt.ClaimSequence ||
		current.Attempt.AttemptSequence <= current.Record.JournalSequence {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback commit journal sequence is not monotonic")
	}

	transition, err := AdvanceModuleLifecycleJob(current.Record.State, event)
	if err != nil {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback terminal transition rejected: %w", err)
	}
	if !transition.Terminal || transition.From != ModuleJobStateRollingBack {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("rollback outcome must terminate from ROLLING_BACK")
	}

	receipt, err := buildModuleUpdateJobRollbackReceipt(current.Source, current.Attempt, outcome)
	if err != nil {
		return ModuleUpdateJobRollbackCommitResult{}, err
	}
	candidate := current
	candidate.Record.State = receipt.ToState
	candidate.Record.StateVersion = receipt.ToStateVersion
	candidate.Record.JournalSequence = receipt.JournalSequence
	candidate.Receipt = receipt
	candidate.HasReceipt = true
	if err := validateModuleUpdateJobRollbackPersistenceState(candidate, admission); err != nil {
		return ModuleUpdateJobRollbackCommitResult{}, fmt.Errorf("invalid rollback commit candidate: %w", err)
	}

	return ModuleUpdateJobRollbackCommitResult{
		State:                      candidate,
		Receipt:                    receipt,
		Replay:                     false,
		AttemptConsumptionRequired: true,
		JournalAppendRequired:      true,
		RecoveryRequired:           receipt.RecoveryRequired,
		FurtherAttemptAuthorized:   false,
		AutomaticRetryAuthorized:   false,
		ExecutionAuthorized:        false,
		ProductionMutationAllowed:  false,
	}, nil
}

func validateModuleUpdateJobRollbackPersistenceState(
	state ModuleUpdateJobRollbackPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	if err := ValidateModuleUpdateJobRollbackAttempt(state.Attempt, state.Source, admission); err != nil {
		return fmt.Errorf("rollback persistence attempt is not revalidatable: %w", err)
	}
	if state.Record.RecordID != state.Source.Source.Record.RecordID ||
		state.Record.JobID != state.Source.Source.Record.JobID ||
		state.Record.AdmissionID != state.Source.Source.Record.AdmissionID ||
		state.Record.ClaimID != state.Source.Source.Record.ClaimID ||
		state.Record.ClaimedBy != state.Source.Source.Record.ClaimedBy ||
		state.Record.ClaimedFromStateVersion != state.Source.Source.Record.ClaimedFromStateVersion {
		return fmt.Errorf("rollback persistence record identity lineage mismatch")
	}
	if !state.Record.PreserveUserData || !state.Record.PreserveSecretMaterial || state.Record.ExecutionAuthorized {
		return fmt.Errorf("rollback persistence record violates safety contract")
	}

	if !state.HasReceipt {
		if state.Receipt != (ModuleUpdateJobRollbackReceipt{}) {
			return fmt.Errorf("rollback receipt marker and evidence disagree")
		}
		if state.Record != state.Source.Source.Record {
			return fmt.Errorf("unconsumed rollback attempt must retain exact source record")
		}
		return nil
	}

	receipt := state.Receipt
	if !receipt.AttemptConsumed || !receipt.AtomicPersistenceRequired ||
		!receipt.PreserveUserData || !receipt.PreserveSecretMaterial || !receipt.Terminal ||
		receipt.FurtherAttemptAuthorized || receipt.AutomaticRetryAuthorized ||
		receipt.ExecutionAuthorized || receipt.GenericCommandAuthorized || receipt.ProductionMutationAllowed {
		return fmt.Errorf("rollback receipt violates terminal safety contract")
	}
	if receipt.AttemptID != state.Attempt.AttemptID ||
		receipt.ClaimID != state.Attempt.ClaimID ||
		receipt.RollbackAdmissionID != state.Attempt.RollbackAdmissionID ||
		receipt.RecordID != state.Attempt.RecordID ||
		receipt.JobID != state.Attempt.JobID ||
		receipt.SourceAdmissionID != state.Attempt.SourceAdmissionID ||
		receipt.VerificationReceiptID != state.Attempt.VerificationReceiptID ||
		receipt.WorkerID != state.Attempt.WorkerID ||
		receipt.ModuleID != state.Attempt.ModuleID {
		return fmt.Errorf("rollback receipt attempt lineage mismatch")
	}
	if receipt.RestoreVersion != state.Attempt.RestoreVersion ||
		receipt.FailedTargetVersion != state.Attempt.FailedTargetVersion ||
		receipt.Generation != state.Attempt.Generation ||
		receipt.RollbackMode != state.Attempt.RollbackMode ||
		receipt.PreMigrationSnapshotID != state.Attempt.PreMigrationSnapshotID {
		return fmt.Errorf("rollback receipt restore lineage mismatch")
	}
	if receipt.FromState != ModuleJobStateRollingBack ||
		receipt.FromStateVersion != state.Attempt.ExpectedStateVersion ||
		receipt.ToStateVersion != receipt.FromStateVersion+1 ||
		receipt.JournalSequence != state.Attempt.AttemptSequence {
		return fmt.Errorf("rollback receipt revision lineage mismatch")
	}
	transition, err := AdvanceModuleLifecycleJob(receipt.FromState, moduleUpdateRollbackEventMust(receipt.Outcome))
	if err != nil || transition.To != receipt.ToState || transition.Terminal != receipt.Terminal ||
		transition.RecoveryRequired != receipt.RecoveryRequired {
		return fmt.Errorf("rollback receipt outcome does not match lifecycle transition")
	}
	if receipt.Outcome == ModuleUpdateJobRollbackSucceeded {
		if receipt.ToState != ModuleJobStateRolledBack || receipt.RecoveryRequired {
			return fmt.Errorf("successful rollback receipt has invalid terminal semantics")
		}
	} else if receipt.Outcome == ModuleUpdateJobRollbackFailed {
		if receipt.ToState != ModuleJobStateFailed || !receipt.RecoveryRequired {
			return fmt.Errorf("failed rollback receipt must require recovery")
		}
	} else {
		return fmt.Errorf("unsupported rollback outcome %q", receipt.Outcome)
	}
	if state.Record.State != receipt.ToState ||
		state.Record.StateVersion != receipt.ToStateVersion ||
		state.Record.JournalSequence != receipt.JournalSequence {
		return fmt.Errorf("rollback durable record does not match receipt revision")
	}
	rebuilt, err := buildModuleUpdateJobRollbackReceipt(state.Source, state.Attempt, receipt.Outcome)
	if err != nil {
		return err
	}
	if receipt != rebuilt || receipt.ReceiptID != moduleUpdateJobRollbackReceiptID(receipt) {
		return fmt.Errorf("rollback receipt identity mismatch")
	}
	return nil
}

func buildModuleUpdateJobRollbackReceipt(
	source ModuleUpdateJobRollbackClaimPersistenceState,
	attempt ModuleUpdateJobRollbackAttempt,
	outcome ModuleUpdateJobRollbackOutcome,
) (ModuleUpdateJobRollbackReceipt, error) {
	transition, err := AdvanceModuleLifecycleJob(ModuleJobStateRollingBack, moduleUpdateRollbackEventMust(outcome))
	if err != nil {
		return ModuleUpdateJobRollbackReceipt{}, err
	}
	if attempt.ExpectedStateVersion == ^uint64(0) {
		return ModuleUpdateJobRollbackReceipt{}, fmt.Errorf("rollback receipt state version overflow")
	}
	if source.Source.Record.StateVersion != attempt.ExpectedStateVersion ||
		source.Source.Record.JournalSequence != attempt.ExpectedJournalSequence {
		return ModuleUpdateJobRollbackReceipt{}, fmt.Errorf("rollback receipt source revision drift")
	}
	receipt := ModuleUpdateJobRollbackReceipt{
		AttemptID:                 attempt.AttemptID,
		ClaimID:                   attempt.ClaimID,
		RollbackAdmissionID:       attempt.RollbackAdmissionID,
		RecordID:                  attempt.RecordID,
		JobID:                     attempt.JobID,
		SourceAdmissionID:         attempt.SourceAdmissionID,
		VerificationReceiptID:     attempt.VerificationReceiptID,
		WorkerID:                  attempt.WorkerID,
		ModuleID:                  attempt.ModuleID,
		RestoreVersion:            attempt.RestoreVersion,
		FailedTargetVersion:       attempt.FailedTargetVersion,
		Generation:                attempt.Generation,
		RollbackMode:              attempt.RollbackMode,
		PreMigrationSnapshotID:    attempt.PreMigrationSnapshotID,
		Outcome:                   outcome,
		FromState:                 ModuleJobStateRollingBack,
		ToState:                   transition.To,
		FromStateVersion:          attempt.ExpectedStateVersion,
		ToStateVersion:            attempt.ExpectedStateVersion + 1,
		JournalSequence:           attempt.AttemptSequence,
		AttemptConsumed:           true,
		AtomicPersistenceRequired: true,
		PreserveUserData:          true,
		PreserveSecretMaterial:    true,
		Terminal:                  true,
		RecoveryRequired:          transition.RecoveryRequired,
		FurtherAttemptAuthorized:  false,
		AutomaticRetryAuthorized:  false,
		ExecutionAuthorized:       false,
		GenericCommandAuthorized:  false,
		ProductionMutationAllowed: false,
	}
	receipt.ReceiptID = moduleUpdateJobRollbackReceiptID(receipt)
	return receipt, nil
}

func moduleUpdateRollbackEvent(outcome ModuleUpdateJobRollbackOutcome) (ModuleLifecycleJobEvent, error) {
	switch outcome {
	case ModuleUpdateJobRollbackSucceeded:
		return ModuleJobEventRollbackSucceeded, nil
	case ModuleUpdateJobRollbackFailed:
		return ModuleJobEventRollbackFailed, nil
	default:
		return "", fmt.Errorf("unsupported rollback outcome %q", outcome)
	}
}

func moduleUpdateRollbackEventMust(outcome ModuleUpdateJobRollbackOutcome) ModuleLifecycleJobEvent {
	event, _ := moduleUpdateRollbackEvent(outcome)
	return event
}

func moduleUpdateJobRollbackReceiptID(receipt ModuleUpdateJobRollbackReceipt) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
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
		strconv.FormatUint(receipt.Generation, 10),
		receipt.RollbackMode,
		receipt.PreMigrationSnapshotID,
		string(receipt.Outcome),
		string(receipt.FromState),
		string(receipt.ToState),
		strconv.FormatUint(receipt.FromStateVersion, 10),
		strconv.FormatUint(receipt.ToStateVersion, 10),
		strconv.FormatUint(receipt.JournalSequence, 10),
		strconv.FormatBool(receipt.AttemptConsumed),
		strconv.FormatBool(receipt.AtomicPersistenceRequired),
		strconv.FormatBool(receipt.PreserveUserData),
		strconv.FormatBool(receipt.PreserveSecretMaterial),
		strconv.FormatBool(receipt.Terminal),
		strconv.FormatBool(receipt.RecoveryRequired),
		strconv.FormatBool(receipt.FurtherAttemptAuthorized),
		strconv.FormatBool(receipt.AutomaticRetryAuthorized),
		strconv.FormatBool(receipt.ExecutionAuthorized),
		strconv.FormatBool(receipt.GenericCommandAuthorized),
		strconv.FormatBool(receipt.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-rollback-receipt:" + hex.EncodeToString(digest[:])
}
