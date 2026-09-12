package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type ModuleUpdateJobRollbackReconciliationOutcome string

const (
	ModuleUpdateJobRollbackReconciliationCommitted              ModuleUpdateJobRollbackReconciliationOutcome = "COMMITTED"
	ModuleUpdateJobRollbackReconciliationDefinitelyNotCommitted ModuleUpdateJobRollbackReconciliationOutcome = "DEFINITELY_NOT_COMMITTED"
	ModuleUpdateJobRollbackReconciliationAmbiguous              ModuleUpdateJobRollbackReconciliationOutcome = "AMBIGUOUS"
)

// ModuleUpdateJobRollbackReconciliationReceipt is immutable evidence produced
// after a lost/uncertain rollback-completion acknowledgement. It never grants a
// retry, rollback, command, or production-mutation capability.
type ModuleUpdateJobRollbackReconciliationReceipt struct {
	ReconciliationID          string                                       `json:"reconciliation_id"`
	AttemptID                 string                                       `json:"attempt_id"`
	ClaimID                   string                                       `json:"claim_id"`
	RollbackAdmissionID       string                                       `json:"rollback_admission_id"`
	RecordID                  string                                       `json:"record_id"`
	JobID                     string                                       `json:"job_id"`
	WorkerID                  string                                       `json:"worker_id"`
	ModuleID                  string                                       `json:"module_id"`
	ExpectedReceiptID         string                                       `json:"expected_receipt_id"`
	ObservedReceiptID         string                                       `json:"observed_receipt_id,omitempty"`
	ExpectedOutcome           ModuleUpdateJobRollbackOutcome               `json:"expected_outcome"`
	Outcome                   ModuleUpdateJobRollbackReconciliationOutcome `json:"outcome"`
	Reason                    string                                       `json:"reason"`
	ObservedState             ModuleLifecycleJobState                      `json:"observed_state"`
	ObservedStateVersion      uint64                                       `json:"observed_state_version"`
	ObservedJournalSequence   uint64                                       `json:"observed_journal_sequence"`
	TerminalEvidenceConfirmed bool                                         `json:"terminal_evidence_confirmed"`
	FreshDecisionRequired     bool                                         `json:"fresh_decision_required"`
	PreserveUserData          bool                                         `json:"preserve_user_data"`
	PreserveSecretMaterial    bool                                         `json:"preserve_secret_material"`
	FurtherAttemptAuthorized  bool                                         `json:"further_attempt_authorized"`
	AutomaticRetryAuthorized  bool                                         `json:"automatic_retry_authorized"`
	ExecutionAuthorized       bool                                         `json:"execution_authorized"`
	GenericCommandAuthorized  bool                                         `json:"generic_command_authorized"`
	ProductionMutationAllowed bool                                         `json:"production_mutation_allowed"`
}

// ReconcileModuleUpdateJobRollbackCommit classifies fresh durable evidence after
// the caller lost the result of CommitModuleUpdateJobRollbackCAS. Only an exact
// terminal receipt proves COMMITTED. Only the byte-for-byte original pre-CAS
// record with no receipt proves DEFINITELY_NOT_COMMITTED. Every other same-lineage
// observation is AMBIGUOUS and must be resolved by a separate fresh decision.
func ReconcileModuleUpdateJobRollbackCommit(
	source ModuleUpdateJobRollbackPersistenceState,
	observed ModuleUpdateJobRollbackPersistenceState,
	admission ModuleUpdateJobAdmission,
	expectedOutcome ModuleUpdateJobRollbackOutcome,
) (ModuleUpdateJobRollbackReconciliationReceipt, error) {
	if source.HasReceipt {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"rollback reconciliation requires pre-commit source state",
		)
	}
	if err := validateModuleUpdateJobRollbackPersistenceState(source, admission); err != nil {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"invalid rollback reconciliation source: %w", err,
		)
	}
	if observed.Attempt != source.Attempt {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"rollback reconciliation attempt lineage mismatch",
		)
	}
	if err := ValidateModuleUpdateJobRollbackAttempt(observed.Attempt, observed.Source, admission); err != nil {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"rollback reconciliation observed source is not revalidatable: %w", err,
		)
	}
	if !sameRollbackCompletionRecordIdentity(source.Record, observed.Record) {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"rollback reconciliation durable record identity mismatch",
		)
	}

	expected, err := CommitModuleUpdateJobRollbackCAS(
		source,
		admission,
		source.Attempt.ExpectedStateVersion,
		source.Attempt.ExpectedJournalSequence,
		expectedOutcome,
	)
	if err != nil {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"cannot rebuild expected rollback completion: %w", err,
		)
	}
	if expected.Replay {
		return ModuleUpdateJobRollbackReconciliationReceipt{}, fmt.Errorf(
			"rollback reconciliation source unexpectedly replayed",
		)
	}

	outcome := ModuleUpdateJobRollbackReconciliationAmbiguous
	reason := "durable_state_incomplete_or_diverged"
	terminalConfirmed := false
	freshDecisionRequired := true

	if observed.HasReceipt {
		if err := validateModuleUpdateJobRollbackPersistenceState(observed, admission); err == nil &&
			observed.Receipt == expected.Receipt {
			outcome = ModuleUpdateJobRollbackReconciliationCommitted
			reason = "exact_terminal_receipt"
			terminalConfirmed = true
			freshDecisionRequired = false
		}
	} else if observed.Receipt == (ModuleUpdateJobRollbackReceipt{}) && observed.Record == source.Record {
		outcome = ModuleUpdateJobRollbackReconciliationDefinitelyNotCommitted
		reason = "exact_precommit_revision"
	}

	receipt := ModuleUpdateJobRollbackReconciliationReceipt{
		AttemptID:                 source.Attempt.AttemptID,
		ClaimID:                   source.Attempt.ClaimID,
		RollbackAdmissionID:       source.Attempt.RollbackAdmissionID,
		RecordID:                  source.Attempt.RecordID,
		JobID:                     source.Attempt.JobID,
		WorkerID:                  source.Attempt.WorkerID,
		ModuleID:                  source.Attempt.ModuleID,
		ExpectedReceiptID:         expected.Receipt.ReceiptID,
		ExpectedOutcome:           expectedOutcome,
		Outcome:                   outcome,
		Reason:                    reason,
		ObservedState:             observed.Record.State,
		ObservedStateVersion:      observed.Record.StateVersion,
		ObservedJournalSequence:   observed.Record.JournalSequence,
		TerminalEvidenceConfirmed: terminalConfirmed,
		FreshDecisionRequired:     freshDecisionRequired,
		PreserveUserData:          true,
		PreserveSecretMaterial:    true,
		FurtherAttemptAuthorized:  false,
		AutomaticRetryAuthorized:  false,
		ExecutionAuthorized:       false,
		GenericCommandAuthorized:  false,
		ProductionMutationAllowed: false,
	}
	if observed.HasReceipt {
		receipt.ObservedReceiptID = observed.Receipt.ReceiptID
	}
	receipt.ReconciliationID = moduleUpdateJobRollbackReconciliationID(receipt)
	return receipt, nil
}

func sameRollbackCompletionRecordIdentity(expected, observed ModuleUpdateJobRecord) bool {
	return observed.RecordID == expected.RecordID &&
		observed.JobID == expected.JobID &&
		observed.AdmissionID == expected.AdmissionID &&
		observed.ClaimID == expected.ClaimID &&
		observed.ClaimedBy == expected.ClaimedBy &&
		observed.ClaimedFromStateVersion == expected.ClaimedFromStateVersion
}

func moduleUpdateJobRollbackReconciliationID(receipt ModuleUpdateJobRollbackReconciliationReceipt) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		receipt.AttemptID,
		receipt.ClaimID,
		receipt.RollbackAdmissionID,
		receipt.RecordID,
		receipt.JobID,
		receipt.WorkerID,
		receipt.ModuleID,
		receipt.ExpectedReceiptID,
		receipt.ObservedReceiptID,
		string(receipt.ExpectedOutcome),
		string(receipt.Outcome),
		receipt.Reason,
		string(receipt.ObservedState),
		strconv.FormatUint(receipt.ObservedStateVersion, 10),
		strconv.FormatUint(receipt.ObservedJournalSequence, 10),
		strconv.FormatBool(receipt.TerminalEvidenceConfirmed),
		strconv.FormatBool(receipt.FreshDecisionRequired),
		strconv.FormatBool(receipt.PreserveUserData),
		strconv.FormatBool(receipt.PreserveSecretMaterial),
		strconv.FormatBool(receipt.FurtherAttemptAuthorized),
		strconv.FormatBool(receipt.AutomaticRetryAuthorized),
		strconv.FormatBool(receipt.ExecutionAuthorized),
		strconv.FormatBool(receipt.GenericCommandAuthorized),
		strconv.FormatBool(receipt.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-rollback-reconciliation:" + hex.EncodeToString(digest[:])
}
