package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type ModuleUpdateJobApplyRecoveryNextStep string

const (
	ModuleUpdateJobApplyRecoveryRevalidateApply        ModuleUpdateJobApplyRecoveryNextStep = "REVALIDATE_BOUNDED_APPLY"
	ModuleUpdateJobApplyRecoveryRevalidateVerification ModuleUpdateJobApplyRecoveryNextStep = "REVALIDATE_VERIFICATION"
)

// ModuleUpdateJobApplyRecoveryPlan is read-only restart evidence for the exact
// persisted apply state. It never authorizes module execution or state change.
type ModuleUpdateJobApplyRecoveryPlan struct {
	RecoveryID                           string                               `json:"recovery_id"`
	RecordID                             string                               `json:"record_id"`
	JobID                                string                               `json:"job_id"`
	AdmissionID                          string                               `json:"admission_id"`
	ClaimID                              string                               `json:"claim_id"`
	WorkerID                             string                               `json:"worker_id"`
	State                                ModuleLifecycleJobState              `json:"state"`
	StateVersion                         uint64                               `json:"state_version"`
	JournalSequence                      uint64                               `json:"journal_sequence"`
	ApplyReceiptID                       string                               `json:"apply_receipt_id,omitempty"`
	NextStep                             ModuleUpdateJobApplyRecoveryNextStep `json:"next_step"`
	PreMigrationSnapshotEvidenceRequired bool                                 `json:"pre_migration_snapshot_evidence_required"`
	PreserveUserData                     bool                                 `json:"preserve_user_data"`
	PreserveSecretMaterial               bool                                 `json:"preserve_secret_material"`
	ExecutionAuthorized                  bool                                 `json:"execution_authorized"`
	ProductionMutationAllowed            bool                                 `json:"production_mutation_allowed"`
}

func RecoverModuleUpdateJobApplyPersistence(
	state ModuleUpdateJobApplyPersistenceState,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobApplyRecoveryPlan, error) {
	if err := validateModuleUpdateJobApplyPersistenceState(state, admission); err != nil {
		return ModuleUpdateJobApplyRecoveryPlan{}, err
	}

	plan := ModuleUpdateJobApplyRecoveryPlan{
		RecordID:                             state.Record.RecordID,
		JobID:                                state.Record.JobID,
		AdmissionID:                          state.Record.AdmissionID,
		ClaimID:                              state.Record.ClaimID,
		WorkerID:                             state.Record.ClaimedBy,
		State:                                state.Record.State,
		StateVersion:                         state.Record.StateVersion,
		JournalSequence:                      state.Record.JournalSequence,
		PreMigrationSnapshotEvidenceRequired: admission.RequirePreMigrationSnapshot,
		PreserveUserData:                     true,
		PreserveSecretMaterial:               true,
		ExecutionAuthorized:                  false,
		ProductionMutationAllowed:            false,
	}

	switch state.Record.State {
	case ModuleJobStateRunning:
		plan.NextStep = ModuleUpdateJobApplyRecoveryRevalidateApply
	case ModuleJobStateVerifying:
		plan.NextStep = ModuleUpdateJobApplyRecoveryRevalidateVerification
		plan.ApplyReceiptID = state.ApplyReceipt.ReceiptID
	default:
		return ModuleUpdateJobApplyRecoveryPlan{}, fmt.Errorf("unsupported persisted apply recovery state %q", state.Record.State)
	}
	plan.RecoveryID = moduleUpdateJobApplyRecoveryID(plan)
	return plan, nil
}

func moduleUpdateJobApplyRecoveryID(plan ModuleUpdateJobApplyRecoveryPlan) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		plan.RecordID,
		plan.JobID,
		plan.AdmissionID,
		plan.ClaimID,
		plan.WorkerID,
		string(plan.State),
		strconv.FormatUint(plan.StateVersion, 10),
		strconv.FormatUint(plan.JournalSequence, 10),
		plan.ApplyReceiptID,
		string(plan.NextStep),
		strconv.FormatBool(plan.PreMigrationSnapshotEvidenceRequired),
		strconv.FormatBool(plan.PreserveUserData),
		strconv.FormatBool(plan.PreserveSecretMaterial),
		strconv.FormatBool(plan.ExecutionAuthorized),
		strconv.FormatBool(plan.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-apply-recovery:" + hex.EncodeToString(digest[:])
}
