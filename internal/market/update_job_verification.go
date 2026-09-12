package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var moduleUpdateVerificationEvidenceIDPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._:/-]{0,126}[A-Za-z0-9])?$`)

type ModuleUpdateJobVerificationOutcome string

const (
	ModuleUpdateJobVerificationPassed ModuleUpdateJobVerificationOutcome = "PASSED"
	ModuleUpdateJobVerificationFailed ModuleUpdateJobVerificationOutcome = "FAILED"
)

// ModuleUpdateJobVerificationReceipt is immutable evidence that verification of
// the exact applied update completed. It records the resulting state transition
// but never grants permission to execute verification, rollback, or module code.
type ModuleUpdateJobVerificationReceipt struct {
	ReceiptID                 string                             `json:"receipt_id"`
	RecordID                  string                             `json:"record_id"`
	JobID                     string                             `json:"job_id"`
	AdmissionID               string                             `json:"admission_id"`
	ClaimID                   string                             `json:"claim_id"`
	WorkerID                  string                             `json:"worker_id"`
	ApplyReceiptID            string                             `json:"apply_receipt_id"`
	VerificationEvidenceID    string                             `json:"verification_evidence_id"`
	Outcome                   ModuleUpdateJobVerificationOutcome `json:"outcome"`
	FromState                 ModuleLifecycleJobState            `json:"from_state"`
	ToState                   ModuleLifecycleJobState            `json:"to_state"`
	FromStateVersion          uint64                             `json:"from_state_version"`
	ToStateVersion            uint64                             `json:"to_state_version"`
	JournalSequence           uint64                             `json:"journal_sequence"`
	PreserveUserData          bool                               `json:"preserve_user_data"`
	PreserveSecretMaterial    bool                               `json:"preserve_secret_material"`
	RollbackRequiredNow       bool                               `json:"rollback_required_now"`
	Terminal                  bool                               `json:"terminal"`
	ExecutionAuthorized       bool                               `json:"execution_authorized"`
	ProductionMutationAllowed bool                               `json:"production_mutation_allowed"`
}

type ModuleUpdateJobVerificationPersistenceState struct {
	Record                 ModuleUpdateJobRecord              `json:"record"`
	ClaimJournal           ModuleUpdateJobClaimJournalEntry   `json:"claim_journal"`
	ApplyReceipt           ModuleUpdateJobApplyReceipt        `json:"apply_receipt"`
	VerificationReceipt    ModuleUpdateJobVerificationReceipt `json:"verification_receipt"`
	HasVerificationReceipt bool                               `json:"has_verification_receipt"`
}

type ModuleUpdateJobVerificationCommitResult struct {
	State                 ModuleUpdateJobVerificationPersistenceState `json:"state"`
	Receipt               ModuleUpdateJobVerificationReceipt          `json:"receipt"`
	Replay                bool                                        `json:"replay"`
	JournalAppendRequired bool                                        `json:"journal_append_required"`
	RollbackRequired      bool                                        `json:"rollback_required"`
	ExecutionAuthorized   bool                                        `json:"execution_authorized"`
	ProductionMutation    bool                                        `json:"production_mutation"`
}

func InitializeModuleUpdateJobVerificationPersistence(
	applyState ModuleUpdateJobApplyPersistenceState,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobVerificationPersistenceState, error) {
	if applyState.Record.State != ModuleJobStateVerifying || !applyState.HasApplyReceipt {
		return ModuleUpdateJobVerificationPersistenceState{}, fmt.Errorf("verification requires committed VERIFYING apply state")
	}
	if err := validateModuleUpdateJobApplyPersistenceState(applyState, admission); err != nil {
		return ModuleUpdateJobVerificationPersistenceState{}, fmt.Errorf("invalid VERIFYING apply state: %w", err)
	}
	return ModuleUpdateJobVerificationPersistenceState{
		Record:       applyState.Record,
		ClaimJournal: applyState.ClaimJournal,
		ApplyReceipt: applyState.ApplyReceipt,
	}, nil
}

// CommitModuleUpdateJobVerificationCAS records the bounded verification result
// against the exact apply receipt. A persistence adapter must atomically CAS the
// durable job record and append Receipt. Failed verification enters
// ROLLING_BACK; it does not execute rollback itself.
func CommitModuleUpdateJobVerificationCAS(
	current ModuleUpdateJobVerificationPersistenceState,
	admission ModuleUpdateJobAdmission,
	expectedStateVersion uint64,
	outcome ModuleUpdateJobVerificationOutcome,
	verificationEvidenceID string,
) (ModuleUpdateJobVerificationCommitResult, error) {
	if err := validateModuleUpdateJobVerificationPersistenceState(current, admission); err != nil {
		return ModuleUpdateJobVerificationCommitResult{}, err
	}
	if expectedStateVersion == 0 {
		return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("expected state version must be greater than zero")
	}
	if !moduleUpdateVerificationEvidenceIDPattern.MatchString(verificationEvidenceID) || strings.TrimSpace(verificationEvidenceID) != verificationEvidenceID {
		return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("valid canonical verification evidence id is required")
	}

	event, err := moduleUpdateVerificationEvent(outcome)
	if err != nil {
		return ModuleUpdateJobVerificationCommitResult{}, err
	}

	switch current.Record.State {
	case ModuleJobStateVerifying:
		if expectedStateVersion != current.Record.StateVersion {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf(
				"stale update verification state version: got %d want %d",
				expectedStateVersion,
				current.Record.StateVersion,
			)
		}
		if current.Record.StateVersion == ^uint64(0) || current.Record.JournalSequence == ^uint64(0) {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("update verification revision overflow")
		}
		transition, err := AdvanceModuleLifecycleJob(current.Record.State, event)
		if err != nil {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("update verification transition rejected: %w", err)
		}
		if transition.From != ModuleJobStateVerifying || transition.RecoveryRequired {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("update verification transition violates recovery contract")
		}
		if outcome == ModuleUpdateJobVerificationPassed && (transition.To != ModuleJobStateSucceeded || !transition.Terminal) {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("successful verification must terminate as SUCCEEDED")
		}
		if outcome == ModuleUpdateJobVerificationFailed && (transition.To != ModuleJobStateRollingBack || transition.Terminal) {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("failed verification must enter ROLLING_BACK")
		}

		receipt := buildModuleUpdateJobVerificationReceipt(
			current,
			outcome,
			verificationEvidenceID,
			transition,
			current.Record.StateVersion+1,
			current.Record.JournalSequence+1,
		)
		candidate := current
		candidate.Record.State = transition.To
		candidate.Record.StateVersion = receipt.ToStateVersion
		candidate.Record.JournalSequence = receipt.JournalSequence
		candidate.VerificationReceipt = receipt
		candidate.HasVerificationReceipt = true
		if err := validateModuleUpdateJobVerificationPersistenceState(candidate, admission); err != nil {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("invalid update verification candidate: %w", err)
		}
		return ModuleUpdateJobVerificationCommitResult{
			State:                 candidate,
			Receipt:               receipt,
			Replay:                false,
			JournalAppendRequired: true,
			RollbackRequired:      receipt.RollbackRequiredNow,
			ExecutionAuthorized:   false,
			ProductionMutation:    false,
		}, nil

	case ModuleJobStateSucceeded, ModuleJobStateRollingBack:
		if expectedStateVersion != current.VerificationReceipt.FromStateVersion {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf(
				"update verification already committed from state version %d",
				current.VerificationReceipt.FromStateVersion,
			)
		}
		transition, err := AdvanceModuleLifecycleJob(ModuleJobStateVerifying, event)
		if err != nil {
			return ModuleUpdateJobVerificationCommitResult{}, err
		}
		verifying := current
		verifying.Record.State = ModuleJobStateVerifying
		verifying.Record.StateVersion = current.VerificationReceipt.FromStateVersion
		verifying.Record.JournalSequence = current.ApplyReceipt.JournalSequence
		verifying.VerificationReceipt = ModuleUpdateJobVerificationReceipt{}
		verifying.HasVerificationReceipt = false
		rebuilt := buildModuleUpdateJobVerificationReceipt(
			verifying,
			outcome,
			verificationEvidenceID,
			transition,
			current.VerificationReceipt.ToStateVersion,
			current.VerificationReceipt.JournalSequence,
		)
		if rebuilt != current.VerificationReceipt {
			return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("persisted update verification receipt evidence mismatch")
		}
		return ModuleUpdateJobVerificationCommitResult{
			State:                 current,
			Receipt:               current.VerificationReceipt,
			Replay:                true,
			JournalAppendRequired: false,
			RollbackRequired:      current.VerificationReceipt.RollbackRequiredNow,
			ExecutionAuthorized:   false,
			ProductionMutation:    false,
		}, nil
	default:
		return ModuleUpdateJobVerificationCommitResult{}, fmt.Errorf("update job state %s cannot record verification", current.Record.State)
	}
}

func validateModuleUpdateJobVerificationPersistenceState(
	state ModuleUpdateJobVerificationPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	if !state.Record.PreserveUserData || !state.Record.PreserveSecretMaterial || state.Record.ExecutionAuthorized {
		return fmt.Errorf("update verification durable record violates safety contract")
	}
	if state.ApplyReceipt.ExecutionAuthorized || state.ApplyReceipt.ProductionMutationAllowed ||
		!state.ApplyReceipt.PreserveUserData || !state.ApplyReceipt.PreserveSecretMaterial ||
		!state.ApplyReceipt.VerificationRequired || !state.ApplyReceipt.RollbackRequiredOnFailure {
		return fmt.Errorf("update verification apply receipt violates safety contract")
	}

	if !state.HasVerificationReceipt {
		if state.VerificationReceipt != (ModuleUpdateJobVerificationReceipt{}) {
			return fmt.Errorf("verification receipt marker and evidence disagree")
		}
		if state.Record.State != ModuleJobStateVerifying {
			return fmt.Errorf("uncompleted verification must remain VERIFYING")
		}
		return validateModuleUpdateJobApplyPersistenceState(ModuleUpdateJobApplyPersistenceState{
			Record:          state.Record,
			ClaimJournal:    state.ClaimJournal,
			ApplyReceipt:    state.ApplyReceipt,
			HasApplyReceipt: true,
		}, admission)
	}

	receipt := state.VerificationReceipt
	if receipt.ExecutionAuthorized || receipt.ProductionMutationAllowed ||
		!receipt.PreserveUserData || !receipt.PreserveSecretMaterial {
		return fmt.Errorf("update verification receipt violates safety contract")
	}
	if receipt.FromState != ModuleJobStateVerifying || receipt.FromStateVersion != state.ApplyReceipt.ToStateVersion ||
		receipt.JournalSequence != state.ApplyReceipt.JournalSequence+1 || receipt.ToStateVersion != receipt.FromStateVersion+1 {
		return fmt.Errorf("update verification receipt transition lineage is invalid")
	}
	if state.Record.State != receipt.ToState || state.Record.StateVersion != receipt.ToStateVersion ||
		state.Record.JournalSequence != receipt.JournalSequence {
		return fmt.Errorf("update verification durable record does not match receipt revision")
	}
	if receipt.RecordID != state.Record.RecordID || receipt.JobID != state.Record.JobID ||
		receipt.AdmissionID != state.Record.AdmissionID || receipt.ClaimID != state.Record.ClaimID ||
		receipt.WorkerID != state.Record.ClaimedBy || receipt.ApplyReceiptID != state.ApplyReceipt.ReceiptID {
		return fmt.Errorf("update verification receipt identity lineage is invalid")
	}
	if !moduleUpdateVerificationEvidenceIDPattern.MatchString(receipt.VerificationEvidenceID) || strings.TrimSpace(receipt.VerificationEvidenceID) != receipt.VerificationEvidenceID {
		return fmt.Errorf("update verification receipt evidence id is invalid")
	}

	transition, err := AdvanceModuleLifecycleJob(ModuleJobStateVerifying, moduleUpdateVerificationEventMust(receipt.Outcome))
	if err != nil || transition.To != receipt.ToState || transition.Terminal != receipt.Terminal {
		return fmt.Errorf("update verification receipt outcome does not match state transition")
	}
	if receipt.Outcome == ModuleUpdateJobVerificationPassed {
		if receipt.RollbackRequiredNow || receipt.ToState != ModuleJobStateSucceeded || !receipt.Terminal {
			return fmt.Errorf("successful verification receipt has invalid rollback semantics")
		}
	} else if receipt.Outcome == ModuleUpdateJobVerificationFailed {
		if !receipt.RollbackRequiredNow || receipt.ToState != ModuleJobStateRollingBack || receipt.Terminal {
			return fmt.Errorf("failed verification receipt must require rollback")
		}
	} else {
		return fmt.Errorf("unsupported verification outcome %q", receipt.Outcome)
	}

	verifyingRecord := state.Record
	verifyingRecord.State = ModuleJobStateVerifying
	verifyingRecord.StateVersion = receipt.FromStateVersion
	verifyingRecord.JournalSequence = state.ApplyReceipt.JournalSequence
	if err := validateModuleUpdateJobApplyPersistenceState(ModuleUpdateJobApplyPersistenceState{
		Record:          verifyingRecord,
		ClaimJournal:    state.ClaimJournal,
		ApplyReceipt:    state.ApplyReceipt,
		HasApplyReceipt: true,
	}, admission); err != nil {
		return fmt.Errorf("update verification apply lineage is not recoverable: %w", err)
	}
	expected := buildModuleUpdateJobVerificationReceipt(
		ModuleUpdateJobVerificationPersistenceState{
			Record:       verifyingRecord,
			ClaimJournal: state.ClaimJournal,
			ApplyReceipt: state.ApplyReceipt,
		},
		receipt.Outcome,
		receipt.VerificationEvidenceID,
		transition,
		receipt.ToStateVersion,
		receipt.JournalSequence,
	)
	if expected != receipt || receipt.ReceiptID != moduleUpdateJobVerificationReceiptID(receipt) {
		return fmt.Errorf("update verification receipt identity mismatch")
	}
	return nil
}

func moduleUpdateVerificationEvent(outcome ModuleUpdateJobVerificationOutcome) (ModuleLifecycleJobEvent, error) {
	switch outcome {
	case ModuleUpdateJobVerificationPassed:
		return ModuleJobEventVerifySucceeded, nil
	case ModuleUpdateJobVerificationFailed:
		return ModuleJobEventVerifyFailed, nil
	default:
		return "", fmt.Errorf("unsupported verification outcome %q", outcome)
	}
}

func moduleUpdateVerificationEventMust(outcome ModuleUpdateJobVerificationOutcome) ModuleLifecycleJobEvent {
	event, _ := moduleUpdateVerificationEvent(outcome)
	return event
}

func buildModuleUpdateJobVerificationReceipt(
	state ModuleUpdateJobVerificationPersistenceState,
	outcome ModuleUpdateJobVerificationOutcome,
	verificationEvidenceID string,
	transition ModuleLifecycleJobTransition,
	toStateVersion uint64,
	journalSequence uint64,
) ModuleUpdateJobVerificationReceipt {
	receipt := ModuleUpdateJobVerificationReceipt{
		RecordID:                  state.Record.RecordID,
		JobID:                     state.Record.JobID,
		AdmissionID:               state.Record.AdmissionID,
		ClaimID:                   state.Record.ClaimID,
		WorkerID:                  state.Record.ClaimedBy,
		ApplyReceiptID:            state.ApplyReceipt.ReceiptID,
		VerificationEvidenceID:    verificationEvidenceID,
		Outcome:                   outcome,
		FromState:                 ModuleJobStateVerifying,
		ToState:                   transition.To,
		FromStateVersion:          state.Record.StateVersion,
		ToStateVersion:            toStateVersion,
		JournalSequence:           journalSequence,
		PreserveUserData:          true,
		PreserveSecretMaterial:    true,
		RollbackRequiredNow:       outcome == ModuleUpdateJobVerificationFailed,
		Terminal:                  transition.Terminal,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
	receipt.ReceiptID = moduleUpdateJobVerificationReceiptID(receipt)
	return receipt
}

func moduleUpdateJobVerificationReceiptID(receipt ModuleUpdateJobVerificationReceipt) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
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
		strconv.FormatUint(receipt.FromStateVersion, 10),
		strconv.FormatUint(receipt.ToStateVersion, 10),
		strconv.FormatUint(receipt.JournalSequence, 10),
		strconv.FormatBool(receipt.PreserveUserData),
		strconv.FormatBool(receipt.PreserveSecretMaterial),
		strconv.FormatBool(receipt.RollbackRequiredNow),
		strconv.FormatBool(receipt.Terminal),
		strconv.FormatBool(receipt.ExecutionAuthorized),
		strconv.FormatBool(receipt.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-verification:" + hex.EncodeToString(digest[:])
}
