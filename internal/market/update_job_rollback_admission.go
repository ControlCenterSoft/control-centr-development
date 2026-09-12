package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ModuleUpdateJobRollbackAdmission is immutable, non-authorizing evidence that
// an exact failed verification result is eligible to enter the rollback claim
// path. It never grants permission to execute rollback or mutate production.
type ModuleUpdateJobRollbackAdmission struct {
	RollbackAdmissionID       string                  `json:"rollback_admission_id"`
	RecordID                  string                  `json:"record_id"`
	JobID                     string                  `json:"job_id"`
	SourceAdmissionID         string                  `json:"source_admission_id"`
	ClaimID                   string                  `json:"claim_id"`
	WorkerID                  string                  `json:"worker_id"`
	ApplyReceiptID            string                  `json:"apply_receipt_id"`
	VerificationReceiptID     string                  `json:"verification_receipt_id"`
	VerificationEvidenceID    string                  `json:"verification_evidence_id"`
	ModuleID                  string                  `json:"module_id"`
	RestoreVersion            string                  `json:"restore_version"`
	FailedTargetVersion       string                  `json:"failed_target_version"`
	Generation                uint64                  `json:"generation"`
	RollbackMode              string                  `json:"rollback_mode"`
	PreMigrationSnapshotID    string                  `json:"pre_migration_snapshot_id,omitempty"`
	ExpectedState             ModuleLifecycleJobState `json:"expected_state"`
	ExpectedStateVersion      uint64                  `json:"expected_state_version"`
	ExpectedJournalSequence   uint64                  `json:"expected_journal_sequence"`
	SingleUse                 bool                    `json:"single_use"`
	AtomicPersistenceRequired bool                    `json:"atomic_persistence_required"`
	FreshRevalidationRequired bool                    `json:"fresh_revalidation_required"`
	PreserveUserData          bool                    `json:"preserve_user_data"`
	PreserveSecretMaterial    bool                    `json:"preserve_secret_material"`
	ExecutionAuthorized       bool                    `json:"execution_authorized"`
	ProductionMutationAllowed bool                    `json:"production_mutation_allowed"`
}

// PlanModuleUpdateJobRollbackAdmission seals the exact failed-verification
// lineage into a single-use rollback admission. The snapshot identity, when
// required, is inherited from the immutable apply receipt rather than accepted
// from a caller.
func PlanModuleUpdateJobRollbackAdmission(
	state ModuleUpdateJobVerificationPersistenceState,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobRollbackAdmission, error) {
	if err := validateModuleUpdateJobVerificationPersistenceState(state, admission); err != nil {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("invalid rollback source state: %w", err)
	}
	if !state.HasVerificationReceipt {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("rollback admission requires committed verification evidence")
	}

	verification := state.VerificationReceipt
	if state.Record.State != ModuleJobStateRollingBack ||
		verification.Outcome != ModuleUpdateJobVerificationFailed ||
		!verification.RollbackRequiredNow ||
		verification.Terminal {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("rollback admission requires failed verification in ROLLING_BACK state")
	}
	if state.Record.StateVersion == 0 || state.Record.JournalSequence == 0 {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("rollback admission requires a committed lifecycle revision")
	}

	if _, err := parseUpdateBundleVersion(admission.CurrentVersion); err != nil {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("invalid rollback restore version: %w", err)
	}
	if _, err := parseUpdateBundleVersion(admission.TargetVersion); err != nil {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("invalid failed target version: %w", err)
	}
	comparison, err := compareUpdateBundleVersions(admission.CurrentVersion, admission.TargetVersion)
	if err != nil || comparison >= 0 {
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("rollback admission requires an upgrade from current to target version")
	}

	snapshotID := state.ApplyReceipt.PreMigrationSnapshotID
	switch admission.RollbackMode {
	case RollbackPreviousVersion:
		if admission.RequirePreMigrationSnapshot || admission.MigrationCount != 0 || snapshotID != "" {
			return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("application-only rollback must restore only the previous version")
		}
	case RollbackDataSnapshot:
		if !admission.RequirePreMigrationSnapshot || admission.MigrationCount <= 0 {
			return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("schema rollback requires migration snapshot evidence")
		}
		if _, err := validateModuleUpdateApplySnapshot(admission, snapshotID); err != nil {
			return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("invalid rollback snapshot lineage: %w", err)
		}
	default:
		return ModuleUpdateJobRollbackAdmission{}, fmt.Errorf("unsupported rollback mode %q", admission.RollbackMode)
	}

	planned := ModuleUpdateJobRollbackAdmission{
		RecordID:                  state.Record.RecordID,
		JobID:                     state.Record.JobID,
		SourceAdmissionID:         admission.AdmissionID,
		ClaimID:                   state.Record.ClaimID,
		WorkerID:                  state.Record.ClaimedBy,
		ApplyReceiptID:            state.ApplyReceipt.ReceiptID,
		VerificationReceiptID:     verification.ReceiptID,
		VerificationEvidenceID:    verification.VerificationEvidenceID,
		ModuleID:                  admission.ModuleID,
		RestoreVersion:            admission.CurrentVersion,
		FailedTargetVersion:       admission.TargetVersion,
		Generation:                admission.Generation,
		RollbackMode:              admission.RollbackMode,
		PreMigrationSnapshotID:    snapshotID,
		ExpectedState:             ModuleJobStateRollingBack,
		ExpectedStateVersion:      state.Record.StateVersion,
		ExpectedJournalSequence:   state.Record.JournalSequence,
		SingleUse:                 true,
		AtomicPersistenceRequired: true,
		FreshRevalidationRequired: true,
		PreserveUserData:          true,
		PreserveSecretMaterial:    true,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
	planned.RollbackAdmissionID = moduleUpdateJobRollbackAdmissionID(planned)
	return planned, nil
}

// ValidateModuleUpdateJobRollbackAdmission replays planning from the exact
// durable evidence and rejects any changed rollback authority or lineage.
func ValidateModuleUpdateJobRollbackAdmission(
	planned ModuleUpdateJobRollbackAdmission,
	state ModuleUpdateJobVerificationPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	rebuilt, err := PlanModuleUpdateJobRollbackAdmission(state, admission)
	if err != nil {
		return err
	}
	if planned != rebuilt {
		return fmt.Errorf("rollback admission evidence mismatch")
	}
	if planned.ExecutionAuthorized || planned.ProductionMutationAllowed ||
		!planned.SingleUse || !planned.AtomicPersistenceRequired || !planned.FreshRevalidationRequired ||
		!planned.PreserveUserData || !planned.PreserveSecretMaterial {
		return fmt.Errorf("rollback admission violates safety contract")
	}
	return nil
}

func moduleUpdateJobRollbackAdmissionID(planned ModuleUpdateJobRollbackAdmission) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		planned.RecordID,
		planned.JobID,
		planned.SourceAdmissionID,
		planned.ClaimID,
		planned.WorkerID,
		planned.ApplyReceiptID,
		planned.VerificationReceiptID,
		planned.VerificationEvidenceID,
		planned.ModuleID,
		planned.RestoreVersion,
		planned.FailedTargetVersion,
		strconv.FormatUint(planned.Generation, 10),
		planned.RollbackMode,
		planned.PreMigrationSnapshotID,
		string(planned.ExpectedState),
		strconv.FormatUint(planned.ExpectedStateVersion, 10),
		strconv.FormatUint(planned.ExpectedJournalSequence, 10),
		strconv.FormatBool(planned.SingleUse),
		strconv.FormatBool(planned.AtomicPersistenceRequired),
		strconv.FormatBool(planned.FreshRevalidationRequired),
		strconv.FormatBool(planned.PreserveUserData),
		strconv.FormatBool(planned.PreserveSecretMaterial),
		strconv.FormatBool(planned.ExecutionAuthorized),
		strconv.FormatBool(planned.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-rollback-admission:" + hex.EncodeToString(digest[:])
}
