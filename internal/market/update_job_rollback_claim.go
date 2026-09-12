package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ModuleUpdateJobRollbackClaim is immutable evidence that one worker claimed
// the exact rollback admission. It does not grant permission to execute
// rollback or mutate production; persistence must treat it as single-use.
type ModuleUpdateJobRollbackClaim struct {
	ClaimID                   string `json:"claim_id"`
	RollbackAdmissionID       string `json:"rollback_admission_id"`
	RecordID                  string `json:"record_id"`
	JobID                     string `json:"job_id"`
	SourceAdmissionID         string `json:"source_admission_id"`
	VerificationReceiptID     string `json:"verification_receipt_id"`
	WorkerID                  string `json:"worker_id"`
	ExpectedStateVersion      uint64 `json:"expected_state_version"`
	ExpectedJournalSequence   uint64 `json:"expected_journal_sequence"`
	ClaimSequence             uint64 `json:"claim_sequence"`
	SingleUse                 bool   `json:"single_use"`
	AtomicPersistenceRequired bool   `json:"atomic_persistence_required"`
	FreshRevalidationRequired bool   `json:"fresh_revalidation_required"`
	PreserveUserData          bool   `json:"preserve_user_data"`
	PreserveSecretMaterial    bool   `json:"preserve_secret_material"`
	ExecutionAuthorized       bool   `json:"execution_authorized"`
	ProductionMutationAllowed bool   `json:"production_mutation_allowed"`
}

// ModuleUpdateJobRollbackClaimPersistenceState is the durable rollback-claim
// boundary. Source verification and rollback admission remain immutable; Claim
// is appended exactly once under the admission's exact state/journal revision.
type ModuleUpdateJobRollbackClaimPersistenceState struct {
	Source            ModuleUpdateJobVerificationPersistenceState `json:"source"`
	RollbackAdmission ModuleUpdateJobRollbackAdmission            `json:"rollback_admission"`
	Claim             ModuleUpdateJobRollbackClaim                `json:"claim"`
	HasClaim          bool                                        `json:"has_claim"`
}

// ModuleUpdateJobRollbackClaimCommitResult is evidence only. Neither first
// claim nor replay authorizes rollback execution or production mutation.
type ModuleUpdateJobRollbackClaimCommitResult struct {
	State               ModuleUpdateJobRollbackClaimPersistenceState `json:"state"`
	Claim               ModuleUpdateJobRollbackClaim                 `json:"claim"`
	Replay              bool                                         `json:"replay"`
	ClaimAppendRequired bool                                         `json:"claim_append_required"`
	ExecutionAuthorized bool                                         `json:"execution_authorized"`
	ProductionMutation  bool                                         `json:"production_mutation"`
}

func InitializeModuleUpdateJobRollbackClaimPersistence(
	source ModuleUpdateJobVerificationPersistenceState,
	rollbackAdmission ModuleUpdateJobRollbackAdmission,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobRollbackClaimPersistenceState, error) {
	if err := ValidateModuleUpdateJobRollbackAdmission(rollbackAdmission, source, admission); err != nil {
		return ModuleUpdateJobRollbackClaimPersistenceState{}, fmt.Errorf("invalid rollback admission: %w", err)
	}
	state := ModuleUpdateJobRollbackClaimPersistenceState{
		Source:            source,
		RollbackAdmission: rollbackAdmission,
	}
	if err := validateModuleUpdateJobRollbackClaimPersistenceState(state, admission); err != nil {
		return ModuleUpdateJobRollbackClaimPersistenceState{}, err
	}
	return state, nil
}

// CommitModuleUpdateJobRollbackClaimCAS seals one worker claim against the
// exact ROLLING_BACK revision admitted earlier. The returned state must be
// persisted atomically using RollbackAdmissionID as the single-use CAS key.
func CommitModuleUpdateJobRollbackClaimCAS(
	current ModuleUpdateJobRollbackClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
	workerID string,
	expectedStateVersion uint64,
	expectedJournalSequence uint64,
) (ModuleUpdateJobRollbackClaimCommitResult, error) {
	if err := validateModuleUpdateJobRollbackClaimPersistenceState(current, admission); err != nil {
		return ModuleUpdateJobRollbackClaimCommitResult{}, err
	}
	workerID = strings.TrimSpace(workerID)
	if !moduleUpdateWorkerIDPattern.MatchString(workerID) {
		return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("invalid rollback worker id %q", workerID)
	}
	if expectedStateVersion == 0 || expectedJournalSequence == 0 {
		return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("rollback claim requires non-zero expected revisions")
	}
	if expectedStateVersion != current.RollbackAdmission.ExpectedStateVersion ||
		expectedJournalSequence != current.RollbackAdmission.ExpectedJournalSequence {
		return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("stale rollback claim revision")
	}
	if current.RollbackAdmission.ExpectedJournalSequence == ^uint64(0) {
		return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("rollback claim sequence overflow")
	}

	planned := buildModuleUpdateJobRollbackClaim(current.RollbackAdmission, workerID)
	if current.HasClaim {
		if current.Claim.WorkerID != workerID {
			return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("rollback admission already claimed by a different worker")
		}
		if current.Claim != planned {
			return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("persisted rollback claim evidence mismatch")
		}
		return ModuleUpdateJobRollbackClaimCommitResult{
			State:               current,
			Claim:               current.Claim,
			Replay:              true,
			ClaimAppendRequired: false,
			ExecutionAuthorized: false,
			ProductionMutation:  false,
		}, nil
	}

	candidate := current
	candidate.Claim = planned
	candidate.HasClaim = true
	if err := validateModuleUpdateJobRollbackClaimPersistenceState(candidate, admission); err != nil {
		return ModuleUpdateJobRollbackClaimCommitResult{}, fmt.Errorf("invalid atomic rollback claim candidate: %w", err)
	}
	return ModuleUpdateJobRollbackClaimCommitResult{
		State:               candidate,
		Claim:               planned,
		Replay:              false,
		ClaimAppendRequired: true,
		ExecutionAuthorized: false,
		ProductionMutation:  false,
	}, nil
}

// ReopenModuleUpdateJobRollbackClaimPersistence validates a durable claim after
// restart. It reconstructs no execution authority and never performs rollback.
func ReopenModuleUpdateJobRollbackClaimPersistence(
	persisted ModuleUpdateJobRollbackClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobRollbackClaim, error) {
	if err := validateModuleUpdateJobRollbackClaimPersistenceState(persisted, admission); err != nil {
		return ModuleUpdateJobRollbackClaim{}, err
	}
	if !persisted.HasClaim {
		return ModuleUpdateJobRollbackClaim{}, fmt.Errorf("rollback claim persistence has no committed claim")
	}
	return persisted.Claim, nil
}

func validateModuleUpdateJobRollbackClaimPersistenceState(
	state ModuleUpdateJobRollbackClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	if err := ValidateModuleUpdateJobRollbackAdmission(state.RollbackAdmission, state.Source, admission); err != nil {
		return fmt.Errorf("rollback claim source is not revalidatable: %w", err)
	}
	if state.RollbackAdmission.ExpectedState != ModuleJobStateRollingBack ||
		state.Source.Record.State != ModuleJobStateRollingBack {
		return fmt.Errorf("rollback claim requires exact ROLLING_BACK source state")
	}
	if state.Source.Record.StateVersion != state.RollbackAdmission.ExpectedStateVersion ||
		state.Source.Record.JournalSequence != state.RollbackAdmission.ExpectedJournalSequence {
		return fmt.Errorf("rollback claim source revision drift")
	}

	if !state.HasClaim {
		if state.Claim != (ModuleUpdateJobRollbackClaim{}) {
			return fmt.Errorf("rollback claim marker and evidence disagree")
		}
		return nil
	}

	claim := state.Claim
	if claim.RollbackAdmissionID != state.RollbackAdmission.RollbackAdmissionID ||
		claim.RecordID != state.RollbackAdmission.RecordID ||
		claim.JobID != state.RollbackAdmission.JobID ||
		claim.SourceAdmissionID != state.RollbackAdmission.SourceAdmissionID ||
		claim.VerificationReceiptID != state.RollbackAdmission.VerificationReceiptID {
		return fmt.Errorf("rollback claim identity lineage is invalid")
	}
	if !moduleUpdateWorkerIDPattern.MatchString(claim.WorkerID) || strings.TrimSpace(claim.WorkerID) != claim.WorkerID {
		return fmt.Errorf("rollback claim worker id is invalid")
	}
	if claim.ExpectedStateVersion != state.RollbackAdmission.ExpectedStateVersion ||
		claim.ExpectedJournalSequence != state.RollbackAdmission.ExpectedJournalSequence ||
		claim.ClaimSequence != state.RollbackAdmission.ExpectedJournalSequence+1 {
		return fmt.Errorf("rollback claim revision lineage is invalid")
	}
	if !claim.SingleUse || !claim.AtomicPersistenceRequired || !claim.FreshRevalidationRequired ||
		!claim.PreserveUserData || !claim.PreserveSecretMaterial || claim.ExecutionAuthorized ||
		claim.ProductionMutationAllowed {
		return fmt.Errorf("rollback claim violates safety contract")
	}
	if claim.ClaimID != moduleUpdateJobRollbackClaimID(claim) {
		return fmt.Errorf("rollback claim identity mismatch")
	}
	return nil
}

func buildModuleUpdateJobRollbackClaim(
	rollbackAdmission ModuleUpdateJobRollbackAdmission,
	workerID string,
) ModuleUpdateJobRollbackClaim {
	claim := ModuleUpdateJobRollbackClaim{
		RollbackAdmissionID:       rollbackAdmission.RollbackAdmissionID,
		RecordID:                  rollbackAdmission.RecordID,
		JobID:                     rollbackAdmission.JobID,
		SourceAdmissionID:         rollbackAdmission.SourceAdmissionID,
		VerificationReceiptID:     rollbackAdmission.VerificationReceiptID,
		WorkerID:                  workerID,
		ExpectedStateVersion:      rollbackAdmission.ExpectedStateVersion,
		ExpectedJournalSequence:   rollbackAdmission.ExpectedJournalSequence,
		ClaimSequence:             rollbackAdmission.ExpectedJournalSequence + 1,
		SingleUse:                 true,
		AtomicPersistenceRequired: true,
		FreshRevalidationRequired: true,
		PreserveUserData:          true,
		PreserveSecretMaterial:    true,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
	claim.ClaimID = moduleUpdateJobRollbackClaimID(claim)
	return claim
}

func moduleUpdateJobRollbackClaimID(claim ModuleUpdateJobRollbackClaim) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		claim.RollbackAdmissionID,
		claim.RecordID,
		claim.JobID,
		claim.SourceAdmissionID,
		claim.VerificationReceiptID,
		claim.WorkerID,
		strconv.FormatUint(claim.ExpectedStateVersion, 10),
		strconv.FormatUint(claim.ExpectedJournalSequence, 10),
		strconv.FormatUint(claim.ClaimSequence, 10),
		strconv.FormatBool(claim.SingleUse),
		strconv.FormatBool(claim.AtomicPersistenceRequired),
		strconv.FormatBool(claim.FreshRevalidationRequired),
		strconv.FormatBool(claim.PreserveUserData),
		strconv.FormatBool(claim.PreserveSecretMaterial),
		strconv.FormatBool(claim.ExecutionAuthorized),
		strconv.FormatBool(claim.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-rollback-claim:" + hex.EncodeToString(digest[:])
}
