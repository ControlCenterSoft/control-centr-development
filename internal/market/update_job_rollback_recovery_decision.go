package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type ModuleUpdateJobRollbackRecoveryDecision string

const (
	ModuleRollbackRecoveryClose      ModuleUpdateJobRollbackRecoveryDecision = "CLOSE_RECOVERY"
	ModuleRollbackRecoveryRevalidate ModuleUpdateJobRollbackRecoveryDecision = "REVALIDATE_FRESH_ROLLBACK_ADMISSION"
)

// ModuleUpdateJobRollbackRecoveryDecisionReceipt is immutable operator evidence
// produced only after an uncertain rollback completion was reconciled as not
// committed or ambiguous. It never reuses the old attempt/admission and never
// grants execution, retry, generic-command, or production-mutation authority.
type ModuleUpdateJobRollbackRecoveryDecisionReceipt struct {
	DecisionID                       string                                  `json:"decision_id"`
	ReconciliationID                 string                                  `json:"reconciliation_id"`
	AttemptID                        string                                  `json:"attempt_id"`
	RollbackAdmissionID              string                                  `json:"rollback_admission_id"`
	RecordID                         string                                  `json:"record_id"`
	JobID                            string                                  `json:"job_id"`
	ModuleID                         string                                  `json:"module_id"`
	OperatorID                       string                                  `json:"operator_id"`
	DecisionReason                   string                                  `json:"decision_reason"`
	Decision                         ModuleUpdateJobRollbackRecoveryDecision `json:"decision"`
	NextAction                       string                                  `json:"next_action"`
	RecoveryClosed                   bool                                    `json:"recovery_closed"`
	RequiresFreshAdmissionLineage    bool                                    `json:"requires_fresh_admission_lineage"`
	PreviousAttemptReusable          bool                                    `json:"previous_attempt_reusable"`
	FreshRollbackAdmissionAuthorized bool                                    `json:"fresh_rollback_admission_authorized"`
	FurtherAttemptAuthorized         bool                                    `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized         bool                                    `json:"automatic_retry_authorized"`
	ExecutionAuthorized              bool                                    `json:"execution_authorized"`
	GenericCommandAuthorized         bool                                    `json:"generic_command_authorized"`
	PreserveUserData                 bool                                    `json:"preserve_user_data"`
	PreserveSecretMaterial           bool                                    `json:"preserve_secret_material"`
	ProductionMutationAllowed        bool                                    `json:"production_mutation_allowed"`
}

// DecideModuleUpdateJobRollbackRecovery records a bounded operator choice after
// lost-response reconciliation. REVALIDATE_FRESH_ROLLBACK_ADMISSION only asks a
// later gate to build new lineage; this function cannot authorize or execute it.
func DecideModuleUpdateJobRollbackRecovery(
	reconciliation ModuleUpdateJobRollbackReconciliationReceipt,
	operatorID string,
	decisionReason string,
	decision ModuleUpdateJobRollbackRecoveryDecision,
) (ModuleUpdateJobRollbackRecoveryDecisionReceipt, error) {
	if err := validateRollbackReconciliationForRecoveryDecision(reconciliation); err != nil {
		return ModuleUpdateJobRollbackRecoveryDecisionReceipt{}, err
	}
	if err := validateRollbackRecoveryAuditText("operator id", operatorID, 128); err != nil {
		return ModuleUpdateJobRollbackRecoveryDecisionReceipt{}, err
	}
	if err := validateRollbackRecoveryAuditText("decision reason", decisionReason, 512); err != nil {
		return ModuleUpdateJobRollbackRecoveryDecisionReceipt{}, err
	}

	receipt := ModuleUpdateJobRollbackRecoveryDecisionReceipt{
		ReconciliationID:                 reconciliation.ReconciliationID,
		AttemptID:                        reconciliation.AttemptID,
		RollbackAdmissionID:              reconciliation.RollbackAdmissionID,
		RecordID:                         reconciliation.RecordID,
		JobID:                            reconciliation.JobID,
		ModuleID:                         reconciliation.ModuleID,
		OperatorID:                       operatorID,
		DecisionReason:                   decisionReason,
		Decision:                         decision,
		PreviousAttemptReusable:          false,
		FreshRollbackAdmissionAuthorized: false,
		FurtherAttemptAuthorized:         false,
		AutomaticRetryAuthorized:         false,
		ExecutionAuthorized:              false,
		GenericCommandAuthorized:         false,
		PreserveUserData:                 true,
		PreserveSecretMaterial:           true,
		ProductionMutationAllowed:        false,
	}

	switch decision {
	case ModuleRollbackRecoveryClose:
		receipt.NextAction = "none"
		receipt.RecoveryClosed = true
		receipt.RequiresFreshAdmissionLineage = false
	case ModuleRollbackRecoveryRevalidate:
		receipt.NextAction = "revalidate-fresh-rollback-admission"
		receipt.RecoveryClosed = false
		receipt.RequiresFreshAdmissionLineage = true
	default:
		return ModuleUpdateJobRollbackRecoveryDecisionReceipt{}, fmt.Errorf(
			"unsupported rollback recovery decision %q",
			decision,
		)
	}

	receipt.DecisionID = moduleUpdateJobRollbackRecoveryDecisionID(receipt)
	return receipt, nil
}

func validateRollbackReconciliationForRecoveryDecision(
	reconciliation ModuleUpdateJobRollbackReconciliationReceipt,
) error {
	if reconciliation.ReconciliationID == "" ||
		reconciliation.ReconciliationID != moduleUpdateJobRollbackReconciliationID(reconciliation) {
		return fmt.Errorf("rollback reconciliation identity mismatch")
	}
	if !reconciliation.PreserveUserData || !reconciliation.PreserveSecretMaterial {
		return fmt.Errorf("rollback reconciliation does not preserve protected module material")
	}
	if reconciliation.FurtherAttemptAuthorized || reconciliation.AutomaticRetryAuthorized ||
		reconciliation.ExecutionAuthorized || reconciliation.GenericCommandAuthorized ||
		reconciliation.ProductionMutationAllowed {
		return fmt.Errorf("rollback reconciliation widened recovery authority")
	}
	if reconciliation.TerminalEvidenceConfirmed || !reconciliation.FreshDecisionRequired {
		return fmt.Errorf("rollback reconciliation does not require a fresh recovery decision")
	}
	switch reconciliation.Outcome {
	case ModuleUpdateJobRollbackReconciliationDefinitelyNotCommitted,
		ModuleUpdateJobRollbackReconciliationAmbiguous:
		return nil
	case ModuleUpdateJobRollbackReconciliationCommitted:
		return fmt.Errorf("committed rollback is terminal and cannot enter recovery decision")
	default:
		return fmt.Errorf("unsupported rollback reconciliation outcome %q", reconciliation.Outcome)
	}
}

func validateRollbackRecoveryAuditText(name, value string, maxLen int) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be non-empty and whitespace-normalized", name)
	}
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds %d bytes", name, maxLen)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func moduleUpdateJobRollbackRecoveryDecisionID(
	receipt ModuleUpdateJobRollbackRecoveryDecisionReceipt,
) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		receipt.ReconciliationID,
		receipt.AttemptID,
		receipt.RollbackAdmissionID,
		receipt.RecordID,
		receipt.JobID,
		receipt.ModuleID,
		receipt.OperatorID,
		receipt.DecisionReason,
		string(receipt.Decision),
		receipt.NextAction,
		strconv.FormatBool(receipt.RecoveryClosed),
		strconv.FormatBool(receipt.RequiresFreshAdmissionLineage),
		strconv.FormatBool(receipt.PreviousAttemptReusable),
		strconv.FormatBool(receipt.FreshRollbackAdmissionAuthorized),
		strconv.FormatBool(receipt.FurtherAttemptAuthorized),
		strconv.FormatBool(receipt.AutomaticRetryAuthorized),
		strconv.FormatBool(receipt.ExecutionAuthorized),
		strconv.FormatBool(receipt.GenericCommandAuthorized),
		strconv.FormatBool(receipt.PreserveUserData),
		strconv.FormatBool(receipt.PreserveSecretMaterial),
		strconv.FormatBool(receipt.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-rollback-recovery-decision:" + hex.EncodeToString(digest[:])
}
