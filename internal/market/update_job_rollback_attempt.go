package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ModuleUpdateJobRollbackAttempt is a bounded, typed execution admission for
// exactly one rollback attempt. It is derived only from a freshly loaded,
// durable rollback claim and never accepts caller-selected commands, paths or
// restore targets.
type ModuleUpdateJobRollbackAttempt struct {
	AttemptID                   string `json:"attempt_id"`
	ClaimID                     string `json:"claim_id"`
	RollbackAdmissionID         string `json:"rollback_admission_id"`
	RecordID                    string `json:"record_id"`
	JobID                       string `json:"job_id"`
	SourceAdmissionID           string `json:"source_admission_id"`
	VerificationReceiptID       string `json:"verification_receipt_id"`
	WorkerID                    string `json:"worker_id"`
	ModuleID                    string `json:"module_id"`
	RestoreVersion              string `json:"restore_version"`
	FailedTargetVersion         string `json:"failed_target_version"`
	Generation                  uint64 `json:"generation"`
	RollbackMode                string `json:"rollback_mode"`
	PreMigrationSnapshotID      string `json:"pre_migration_snapshot_id,omitempty"`
	ExpectedStateVersion        uint64 `json:"expected_state_version"`
	ExpectedJournalSequence     uint64 `json:"expected_journal_sequence"`
	ClaimSequence               uint64 `json:"claim_sequence"`
	AttemptSequence             uint64 `json:"attempt_sequence"`
	SingleUse                   bool   `json:"single_use"`
	FreshStateReadRequired      bool   `json:"fresh_state_read_required"`
	PreserveUserData            bool   `json:"preserve_user_data"`
	PreserveSecretMaterial      bool   `json:"preserve_secret_material"`
	RollbackExecutionAuthorized bool   `json:"rollback_execution_authorized"`
	GenericCommandAuthorized    bool   `json:"generic_command_authorized"`
	MigrationDowngradeAllowed   bool   `json:"migration_downgrade_allowed"`
	AutomaticRetryAuthorized    bool   `json:"automatic_retry_authorized"`
	ProductionMutationAllowed   bool   `json:"production_mutation_allowed"`
}

// AdmitModuleUpdateJobRollbackAttempt mints one typed rollback capability from
// a freshly loaded durable claim. The restore target, rollback mode and optional
// snapshot are inherited from sealed admission evidence; a caller cannot select
// any executable material. A persistence/executor layer must still consume the
// attempt with an exact CAS on the advertised state and journal revision.
func AdmitModuleUpdateJobRollbackAttempt(
	fresh ModuleUpdateJobRollbackClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobRollbackAttempt, error) {
	claim, err := ReopenModuleUpdateJobRollbackClaimPersistence(fresh, admission)
	if err != nil {
		return ModuleUpdateJobRollbackAttempt{}, fmt.Errorf("rollback attempt requires durable claim: %w", err)
	}
	if fresh.Source.Record.State != ModuleJobStateRollingBack {
		return ModuleUpdateJobRollbackAttempt{}, fmt.Errorf("rollback attempt requires fresh ROLLING_BACK state")
	}
	if fresh.Source.Record.StateVersion != claim.ExpectedStateVersion ||
		fresh.Source.Record.JournalSequence != claim.ExpectedJournalSequence {
		return ModuleUpdateJobRollbackAttempt{}, fmt.Errorf("rollback attempt source revision drift")
	}
	if claim.ClaimSequence == ^uint64(0) {
		return ModuleUpdateJobRollbackAttempt{}, fmt.Errorf("rollback attempt sequence overflow")
	}

	rb := fresh.RollbackAdmission
	attempt := ModuleUpdateJobRollbackAttempt{
		ClaimID:                     claim.ClaimID,
		RollbackAdmissionID:         claim.RollbackAdmissionID,
		RecordID:                    claim.RecordID,
		JobID:                       claim.JobID,
		SourceAdmissionID:           claim.SourceAdmissionID,
		VerificationReceiptID:       claim.VerificationReceiptID,
		WorkerID:                    claim.WorkerID,
		ModuleID:                    rb.ModuleID,
		RestoreVersion:              rb.RestoreVersion,
		FailedTargetVersion:         rb.FailedTargetVersion,
		Generation:                  rb.Generation,
		RollbackMode:                rb.RollbackMode,
		PreMigrationSnapshotID:      rb.PreMigrationSnapshotID,
		ExpectedStateVersion:        claim.ExpectedStateVersion,
		ExpectedJournalSequence:     claim.ExpectedJournalSequence,
		ClaimSequence:               claim.ClaimSequence,
		AttemptSequence:             claim.ClaimSequence + 1,
		SingleUse:                   true,
		FreshStateReadRequired:      true,
		PreserveUserData:            true,
		PreserveSecretMaterial:      true,
		RollbackExecutionAuthorized: true,
		GenericCommandAuthorized:    false,
		MigrationDowngradeAllowed:   false,
		AutomaticRetryAuthorized:    false,
		ProductionMutationAllowed:   false,
	}
	attempt.AttemptID = moduleUpdateJobRollbackAttemptID(attempt)
	if err := ValidateModuleUpdateJobRollbackAttempt(attempt, fresh, admission); err != nil {
		return ModuleUpdateJobRollbackAttempt{}, err
	}
	return attempt, nil
}

// ValidateModuleUpdateJobRollbackAttempt deterministically rebuilds the attempt
// from fresh durable state and rejects widened authority or changed lineage.
func ValidateModuleUpdateJobRollbackAttempt(
	attempt ModuleUpdateJobRollbackAttempt,
	fresh ModuleUpdateJobRollbackClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	claim, err := ReopenModuleUpdateJobRollbackClaimPersistence(fresh, admission)
	if err != nil {
		return err
	}
	if fresh.Source.Record.State != ModuleJobStateRollingBack ||
		fresh.Source.Record.StateVersion != claim.ExpectedStateVersion ||
		fresh.Source.Record.JournalSequence != claim.ExpectedJournalSequence {
		return fmt.Errorf("rollback attempt is not bound to fresh ROLLING_BACK revision")
	}
	if attempt.ClaimID != claim.ClaimID ||
		attempt.RollbackAdmissionID != claim.RollbackAdmissionID ||
		attempt.RecordID != claim.RecordID ||
		attempt.JobID != claim.JobID ||
		attempt.SourceAdmissionID != claim.SourceAdmissionID ||
		attempt.VerificationReceiptID != claim.VerificationReceiptID ||
		attempt.WorkerID != claim.WorkerID {
		return fmt.Errorf("rollback attempt claim lineage mismatch")
	}
	rb := fresh.RollbackAdmission
	if attempt.ModuleID != rb.ModuleID ||
		attempt.RestoreVersion != rb.RestoreVersion ||
		attempt.FailedTargetVersion != rb.FailedTargetVersion ||
		attempt.Generation != rb.Generation ||
		attempt.RollbackMode != rb.RollbackMode ||
		attempt.PreMigrationSnapshotID != rb.PreMigrationSnapshotID {
		return fmt.Errorf("rollback attempt restore lineage mismatch")
	}
	if attempt.ExpectedStateVersion != claim.ExpectedStateVersion ||
		attempt.ExpectedJournalSequence != claim.ExpectedJournalSequence ||
		attempt.ClaimSequence != claim.ClaimSequence ||
		attempt.AttemptSequence != claim.ClaimSequence+1 {
		return fmt.Errorf("rollback attempt revision lineage mismatch")
	}
	if !attempt.SingleUse || !attempt.FreshStateReadRequired ||
		!attempt.PreserveUserData || !attempt.PreserveSecretMaterial ||
		!attempt.RollbackExecutionAuthorized || attempt.GenericCommandAuthorized ||
		attempt.MigrationDowngradeAllowed || attempt.AutomaticRetryAuthorized ||
		attempt.ProductionMutationAllowed {
		return fmt.Errorf("rollback attempt violates bounded execution contract")
	}
	if attempt.AttemptID != moduleUpdateJobRollbackAttemptID(attempt) {
		return fmt.Errorf("rollback attempt identity mismatch")
	}
	return nil
}

func moduleUpdateJobRollbackAttemptID(attempt ModuleUpdateJobRollbackAttempt) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		attempt.ClaimID,
		attempt.RollbackAdmissionID,
		attempt.RecordID,
		attempt.JobID,
		attempt.SourceAdmissionID,
		attempt.VerificationReceiptID,
		attempt.WorkerID,
		attempt.ModuleID,
		attempt.RestoreVersion,
		attempt.FailedTargetVersion,
		strconv.FormatUint(attempt.Generation, 10),
		attempt.RollbackMode,
		attempt.PreMigrationSnapshotID,
		strconv.FormatUint(attempt.ExpectedStateVersion, 10),
		strconv.FormatUint(attempt.ExpectedJournalSequence, 10),
		strconv.FormatUint(attempt.ClaimSequence, 10),
		strconv.FormatUint(attempt.AttemptSequence, 10),
		strconv.FormatBool(attempt.SingleUse),
		strconv.FormatBool(attempt.FreshStateReadRequired),
		strconv.FormatBool(attempt.PreserveUserData),
		strconv.FormatBool(attempt.PreserveSecretMaterial),
		strconv.FormatBool(attempt.RollbackExecutionAuthorized),
		strconv.FormatBool(attempt.GenericCommandAuthorized),
		strconv.FormatBool(attempt.MigrationDowngradeAllowed),
		strconv.FormatBool(attempt.AutomaticRetryAuthorized),
		strconv.FormatBool(attempt.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-rollback-attempt:" + hex.EncodeToString(digest[:])
}
