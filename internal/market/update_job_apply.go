package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var moduleUpdateSnapshotIDPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._:/-]{0,126}[A-Za-z0-9])?$`)

// ModuleUpdateJobApplyReceipt is immutable evidence that the exact claimed
// update apply step completed and the job may enter VERIFYING. It never grants
// execution authority for a module operation or a subsequent state change.
type ModuleUpdateJobApplyReceipt struct {
	ReceiptID                   string                  `json:"receipt_id"`
	RecordID                    string                  `json:"record_id"`
	JobID                       string                  `json:"job_id"`
	AdmissionID                 string                  `json:"admission_id"`
	ClaimID                     string                  `json:"claim_id"`
	WorkerID                    string                  `json:"worker_id"`
	BundleID                    string                  `json:"bundle_id"`
	BundleSHA256                string                  `json:"bundle_sha256"`
	BundleSizeBytes             uint64                  `json:"bundle_size_bytes"`
	CurrentVersion              string                  `json:"current_version"`
	TargetVersion               string                  `json:"target_version"`
	Generation                  uint64                  `json:"generation"`
	MigrationCount              int                     `json:"migration_count"`
	RollbackMode                string                  `json:"rollback_mode"`
	RequirePreMigrationSnapshot bool                    `json:"require_pre_migration_snapshot"`
	PreMigrationSnapshotID      string                  `json:"pre_migration_snapshot_id,omitempty"`
	FromState                   ModuleLifecycleJobState `json:"from_state"`
	ToState                     ModuleLifecycleJobState `json:"to_state"`
	FromStateVersion            uint64                  `json:"from_state_version"`
	ToStateVersion              uint64                  `json:"to_state_version"`
	JournalSequence             uint64                  `json:"journal_sequence"`
	PreserveUserData            bool                    `json:"preserve_user_data"`
	PreserveSecretMaterial      bool                    `json:"preserve_secret_material"`
	VerificationRequired        bool                    `json:"verification_required"`
	RollbackRequiredOnFailure   bool                    `json:"rollback_required_on_failure"`
	ExecutionAuthorized         bool                    `json:"execution_authorized"`
	ProductionMutationAllowed   bool                    `json:"production_mutation_allowed"`
}

// ModuleUpdateJobApplyPersistenceState is the exact persistence-neutral state
// around RUNNING -> VERIFYING. ClaimJournal anchors the original worker claim;
// ApplyReceipt is appended exactly once after a successful bounded apply.
type ModuleUpdateJobApplyPersistenceState struct {
	Record          ModuleUpdateJobRecord            `json:"record"`
	ClaimJournal    ModuleUpdateJobClaimJournalEntry `json:"claim_journal"`
	ApplyReceipt    ModuleUpdateJobApplyReceipt      `json:"apply_receipt"`
	HasApplyReceipt bool                             `json:"has_apply_receipt"`
}

type ModuleUpdateJobApplyCommitResult struct {
	State                 ModuleUpdateJobApplyPersistenceState `json:"state"`
	Receipt               ModuleUpdateJobApplyReceipt          `json:"receipt"`
	Replay                bool                                 `json:"replay"`
	JournalAppendRequired bool                                 `json:"journal_append_required"`
	ExecutionAuthorized   bool                                 `json:"execution_authorized"`
	ProductionMutation    bool                                 `json:"production_mutation"`
}

func InitializeModuleUpdateJobApplyPersistence(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
	claimJournal ModuleUpdateJobClaimJournalEntry,
) (ModuleUpdateJobApplyPersistenceState, error) {
	if _, err := RecoverPersistedModuleUpdateJobClaim(record, admission, claimJournal); err != nil {
		return ModuleUpdateJobApplyPersistenceState{}, fmt.Errorf("invalid claimed update job: %w", err)
	}
	return ModuleUpdateJobApplyPersistenceState{
		Record:       record,
		ClaimJournal: claimJournal,
	}, nil
}

// CommitModuleUpdateJobApplySuccessCAS records only successful bounded apply
// evidence. A persistence adapter must atomically CAS Record and append Receipt.
// The receipt is not permission to execute an apply, verification, or rollback.
func CommitModuleUpdateJobApplySuccessCAS(
	current ModuleUpdateJobApplyPersistenceState,
	admission ModuleUpdateJobAdmission,
	expectedStateVersion uint64,
	preMigrationSnapshotID string,
) (ModuleUpdateJobApplyCommitResult, error) {
	if err := validateModuleUpdateJobApplyPersistenceState(current, admission); err != nil {
		return ModuleUpdateJobApplyCommitResult{}, err
	}
	if expectedStateVersion == 0 {
		return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf("expected state version must be greater than zero")
	}
	preMigrationSnapshotID, err := validateModuleUpdateApplySnapshot(admission, preMigrationSnapshotID)
	if err != nil {
		return ModuleUpdateJobApplyCommitResult{}, err
	}

	switch current.Record.State {
	case ModuleJobStateRunning:
		if expectedStateVersion != current.Record.StateVersion {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf(
				"stale update apply state version: got %d want %d",
				expectedStateVersion,
				current.Record.StateVersion,
			)
		}
		if current.Record.StateVersion == ^uint64(0) || current.Record.JournalSequence == ^uint64(0) {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf("update apply revision overflow")
		}
		transition, err := AdvanceModuleLifecycleJob(current.Record.State, ModuleJobEventApplySucceeded)
		if err != nil {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf("update apply transition rejected: %w", err)
		}
		if transition.From != ModuleJobStateRunning ||
			transition.To != ModuleJobStateVerifying ||
			transition.Terminal ||
			transition.RecoveryRequired {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf(
				"update apply transition is not a safe RUNNING to VERIFYING transition",
			)
		}

		receipt := buildModuleUpdateJobApplyReceipt(
			current.Record,
			admission,
			preMigrationSnapshotID,
			current.Record.StateVersion+1,
			current.Record.JournalSequence+1,
		)
		candidate := current
		candidate.Record.State = ModuleJobStateVerifying
		candidate.Record.StateVersion = receipt.ToStateVersion
		candidate.Record.JournalSequence = receipt.JournalSequence
		candidate.ApplyReceipt = receipt
		candidate.HasApplyReceipt = true
		if err := validateModuleUpdateJobApplyPersistenceState(candidate, admission); err != nil {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf("invalid update apply candidate: %w", err)
		}
		return ModuleUpdateJobApplyCommitResult{
			State:                 candidate,
			Receipt:               receipt,
			Replay:                false,
			JournalAppendRequired: true,
			ExecutionAuthorized:   false,
			ProductionMutation:    false,
		}, nil

	case ModuleJobStateVerifying:
		if expectedStateVersion != current.ApplyReceipt.FromStateVersion {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf(
				"update apply already committed from state version %d",
				current.ApplyReceipt.FromStateVersion,
			)
		}
		rebuilt, err := rebuildModuleUpdateJobApplyReceipt(current, admission, preMigrationSnapshotID)
		if err != nil {
			return ModuleUpdateJobApplyCommitResult{}, err
		}
		if rebuilt != current.ApplyReceipt {
			return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf("persisted update apply receipt evidence mismatch")
		}
		return ModuleUpdateJobApplyCommitResult{
			State:                 current,
			Receipt:               current.ApplyReceipt,
			Replay:                true,
			JournalAppendRequired: false,
			ExecutionAuthorized:   false,
			ProductionMutation:    false,
		}, nil
	default:
		return ModuleUpdateJobApplyCommitResult{}, fmt.Errorf(
			"update job state %s cannot record apply success",
			current.Record.State,
		)
	}
}

func validateModuleUpdateJobApplyPersistenceState(
	state ModuleUpdateJobApplyPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	switch state.Record.State {
	case ModuleJobStateRunning:
		if state.HasApplyReceipt || state.ApplyReceipt != (ModuleUpdateJobApplyReceipt{}) {
			return fmt.Errorf("RUNNING update job must not contain apply receipt evidence")
		}
		if _, err := RecoverPersistedModuleUpdateJobClaim(state.Record, admission, state.ClaimJournal); err != nil {
			return fmt.Errorf("RUNNING update job claim evidence is not recoverable: %w", err)
		}
		return nil

	case ModuleJobStateVerifying:
		if !state.HasApplyReceipt {
			return fmt.Errorf("VERIFYING update job requires apply receipt evidence")
		}
		receipt := state.ApplyReceipt
		if receipt.ExecutionAuthorized ||
			receipt.ProductionMutationAllowed ||
			!receipt.PreserveUserData ||
			!receipt.PreserveSecretMaterial {
			return fmt.Errorf("update apply receipt violates safety contract")
		}
		if !receipt.VerificationRequired || !receipt.RollbackRequiredOnFailure {
			return fmt.Errorf("update apply receipt must require verification and rollback on failure")
		}
		if receipt.ToState != ModuleJobStateVerifying ||
			receipt.ToStateVersion != state.Record.StateVersion ||
			receipt.JournalSequence != state.Record.JournalSequence {
			return fmt.Errorf("VERIFYING update job revision does not match apply receipt")
		}
		if receipt.FromState != ModuleJobStateRunning ||
			receipt.FromStateVersion+1 != receipt.ToStateVersion ||
			receipt.JournalSequence != state.ClaimJournal.Sequence+1 {
			return fmt.Errorf("update apply receipt transition lineage is invalid")
		}
		if state.Record.RecordID != receipt.RecordID ||
			state.Record.JobID != receipt.JobID ||
			state.Record.AdmissionID != receipt.AdmissionID ||
			state.Record.ClaimID != receipt.ClaimID ||
			state.Record.ClaimedBy != receipt.WorkerID {
			return fmt.Errorf("VERIFYING update job does not match apply receipt identity")
		}
		if !state.Record.PreserveUserData || !state.Record.PreserveSecretMaterial || state.Record.ExecutionAuthorized {
			return fmt.Errorf("VERIFYING update job durable record violates safety contract")
		}

		running := state.Record
		running.State = ModuleJobStateRunning
		running.StateVersion = receipt.FromStateVersion
		running.JournalSequence = state.ClaimJournal.Sequence
		if _, err := RecoverPersistedModuleUpdateJobClaim(running, admission, state.ClaimJournal); err != nil {
			return fmt.Errorf("VERIFYING update job claim lineage is not recoverable: %w", err)
		}
		expected := buildModuleUpdateJobApplyReceipt(
			running,
			admission,
			receipt.PreMigrationSnapshotID,
			receipt.ToStateVersion,
			receipt.JournalSequence,
		)
		if receipt != expected || receipt.ReceiptID != moduleUpdateJobApplyReceiptID(receipt) {
			return fmt.Errorf("update apply receipt identity mismatch")
		}
		return nil
	default:
		return fmt.Errorf("unsupported update apply persistence state %q", state.Record.State)
	}
}

func rebuildModuleUpdateJobApplyReceipt(
	state ModuleUpdateJobApplyPersistenceState,
	admission ModuleUpdateJobAdmission,
	preMigrationSnapshotID string,
) (ModuleUpdateJobApplyReceipt, error) {
	if state.Record.State != ModuleJobStateVerifying || !state.HasApplyReceipt {
		return ModuleUpdateJobApplyReceipt{}, fmt.Errorf("update apply replay requires VERIFYING receipt state")
	}
	running := state.Record
	running.State = ModuleJobStateRunning
	running.StateVersion = state.ApplyReceipt.FromStateVersion
	running.JournalSequence = state.ClaimJournal.Sequence
	if _, err := RecoverPersistedModuleUpdateJobClaim(running, admission, state.ClaimJournal); err != nil {
		return ModuleUpdateJobApplyReceipt{}, err
	}
	return buildModuleUpdateJobApplyReceipt(
		running,
		admission,
		preMigrationSnapshotID,
		state.ApplyReceipt.ToStateVersion,
		state.ApplyReceipt.JournalSequence,
	), nil
}

func validateModuleUpdateApplySnapshot(
	admission ModuleUpdateJobAdmission,
	preMigrationSnapshotID string,
) (string, error) {
	trimmed := strings.TrimSpace(preMigrationSnapshotID)
	if trimmed != preMigrationSnapshotID {
		return "", fmt.Errorf("pre-migration snapshot id must be canonical")
	}
	if admission.RequirePreMigrationSnapshot {
		if !moduleUpdateSnapshotIDPattern.MatchString(preMigrationSnapshotID) {
			return "", fmt.Errorf("valid pre-migration snapshot id is required")
		}
		return preMigrationSnapshotID, nil
	}
	if preMigrationSnapshotID != "" {
		return "", fmt.Errorf("pre-migration snapshot evidence is not allowed for an application-only update")
	}
	return "", nil
}

func buildModuleUpdateJobApplyReceipt(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
	preMigrationSnapshotID string,
	toStateVersion uint64,
	journalSequence uint64,
) ModuleUpdateJobApplyReceipt {
	receipt := ModuleUpdateJobApplyReceipt{
		RecordID:                    record.RecordID,
		JobID:                       record.JobID,
		AdmissionID:                 admission.AdmissionID,
		ClaimID:                     record.ClaimID,
		WorkerID:                    record.ClaimedBy,
		BundleID:                    admission.BundleID,
		BundleSHA256:                admission.BundleSHA256,
		BundleSizeBytes:             admission.BundleSizeBytes,
		CurrentVersion:              admission.CurrentVersion,
		TargetVersion:               admission.TargetVersion,
		Generation:                  admission.Generation,
		MigrationCount:              admission.MigrationCount,
		RollbackMode:                admission.RollbackMode,
		RequirePreMigrationSnapshot: admission.RequirePreMigrationSnapshot,
		PreMigrationSnapshotID:      preMigrationSnapshotID,
		FromState:                   ModuleJobStateRunning,
		ToState:                     ModuleJobStateVerifying,
		FromStateVersion:            record.StateVersion,
		ToStateVersion:              toStateVersion,
		JournalSequence:             journalSequence,
		PreserveUserData:            true,
		PreserveSecretMaterial:      true,
		VerificationRequired:        true,
		RollbackRequiredOnFailure:   true,
		ExecutionAuthorized:         false,
		ProductionMutationAllowed:   false,
	}
	receipt.ReceiptID = moduleUpdateJobApplyReceiptID(receipt)
	return receipt
}

func moduleUpdateJobApplyReceiptID(receipt ModuleUpdateJobApplyReceipt) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		receipt.RecordID,
		receipt.JobID,
		receipt.AdmissionID,
		receipt.ClaimID,
		receipt.WorkerID,
		receipt.BundleID,
		receipt.BundleSHA256,
		strconv.FormatUint(receipt.BundleSizeBytes, 10),
		receipt.CurrentVersion,
		receipt.TargetVersion,
		strconv.FormatUint(receipt.Generation, 10),
		strconv.Itoa(receipt.MigrationCount),
		receipt.RollbackMode,
		strconv.FormatBool(receipt.RequirePreMigrationSnapshot),
		receipt.PreMigrationSnapshotID,
		string(receipt.FromState),
		string(receipt.ToState),
		strconv.FormatUint(receipt.FromStateVersion, 10),
		strconv.FormatUint(receipt.ToStateVersion, 10),
		strconv.FormatUint(receipt.JournalSequence, 10),
		strconv.FormatBool(receipt.PreserveUserData),
		strconv.FormatBool(receipt.PreserveSecretMaterial),
		strconv.FormatBool(receipt.VerificationRequired),
		strconv.FormatBool(receipt.RollbackRequiredOnFailure),
		strconv.FormatBool(receipt.ExecutionAuthorized),
		strconv.FormatBool(receipt.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-apply:" + hex.EncodeToString(digest[:])
}
