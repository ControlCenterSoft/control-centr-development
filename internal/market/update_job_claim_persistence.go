package market

import "fmt"

// ModuleUpdateJobClaimPersistenceState is the complete durable state at the
// PREPARED -> RUNNING claim boundary. Record and journal must be committed as
// one CAS unit; a torn record-only or journal-only write is invalid.
type ModuleUpdateJobClaimPersistenceState struct {
	Record     ModuleUpdateJobRecord            `json:"record"`
	Journal    ModuleUpdateJobClaimJournalEntry `json:"journal"`
	HasJournal bool                             `json:"has_journal"`
}

// ModuleUpdateJobClaimCommitResult describes one atomic claim persistence
// decision. It is evidence only and never authorizes module execution.
type ModuleUpdateJobClaimCommitResult struct {
	State                 ModuleUpdateJobClaimPersistenceState `json:"state"`
	Claim                 ModuleUpdateJobClaim                 `json:"claim"`
	Replay                bool                                 `json:"replay"`
	JournalAppendRequired bool                                 `json:"journal_append_required"`
	ExecutionAuthorized   bool                                 `json:"execution_authorized"`
	ProductionMutation    bool                                 `json:"production_mutation"`
}

func InitializeModuleUpdateJobClaimPersistence(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobClaimPersistenceState, error) {
	if err := validateModuleUpdateJobRecord(record, admission); err != nil {
		return ModuleUpdateJobClaimPersistenceState{}, fmt.Errorf("invalid prepared update job record: %w", err)
	}
	if record.State != ModuleJobStatePrepared {
		return ModuleUpdateJobClaimPersistenceState{}, fmt.Errorf("claim persistence initialization requires PREPARED state, got %s", record.State)
	}
	return ModuleUpdateJobClaimPersistenceState{Record: record}, nil
}

// CommitModuleUpdateJobClaimCAS performs the persistence-neutral reference
// transaction for a worker claim. A persistence adapter must commit the
// returned Record and Journal atomically under ExpectedStateVersion CAS.
func CommitModuleUpdateJobClaimCAS(
	current ModuleUpdateJobClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
	workerID string,
	expectedStateVersion uint64,
) (ModuleUpdateJobClaimCommitResult, error) {
	if err := validateModuleUpdateJobClaimPersistenceState(current, admission); err != nil {
		return ModuleUpdateJobClaimCommitResult{}, err
	}

	claimResult, err := ClaimModuleUpdateJob(current.Record, admission, workerID, expectedStateVersion)
	if err != nil {
		return ModuleUpdateJobClaimCommitResult{}, err
	}
	if claimResult.Claim.ExecutionAuthorized || claimResult.Claim.ProductionMutationAllowed || claimResult.Journal.ExecutionAuthorized {
		return ModuleUpdateJobClaimCommitResult{}, fmt.Errorf("update job claim persistence widened execution authority")
	}

	if current.Record.State == ModuleJobStateRunning {
		if !claimResult.Replay || claimResult.JournalAppendRequired {
			return ModuleUpdateJobClaimCommitResult{}, fmt.Errorf("persisted RUNNING claim must replay without journal append")
		}
		if claimResult.Record != current.Record || claimResult.Journal != current.Journal {
			return ModuleUpdateJobClaimCommitResult{}, fmt.Errorf("persisted RUNNING claim replay evidence mismatch")
		}
		return ModuleUpdateJobClaimCommitResult{
			State:                 current,
			Claim:                 claimResult.Claim,
			Replay:                true,
			JournalAppendRequired: false,
			ExecutionAuthorized:   false,
			ProductionMutation:    false,
		}, nil
	}

	if claimResult.Replay || !claimResult.JournalAppendRequired {
		return ModuleUpdateJobClaimCommitResult{}, fmt.Errorf("first PREPARED claim must require one journal append")
	}
	candidate := ModuleUpdateJobClaimPersistenceState{
		Record:     claimResult.Record,
		Journal:    claimResult.Journal,
		HasJournal: true,
	}
	if err := validateModuleUpdateJobClaimPersistenceState(candidate, admission); err != nil {
		return ModuleUpdateJobClaimCommitResult{}, fmt.Errorf("invalid atomic update job claim candidate: %w", err)
	}

	return ModuleUpdateJobClaimCommitResult{
		State:                 candidate,
		Claim:                 claimResult.Claim,
		Replay:                false,
		JournalAppendRequired: true,
		ExecutionAuthorized:   false,
		ProductionMutation:    false,
	}, nil
}

// ReopenModuleUpdateJobClaimPersistence revalidates an atomically persisted
// claim after restart and reconstructs its exact replay evidence.
func ReopenModuleUpdateJobClaimPersistence(
	persisted ModuleUpdateJobClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
) (ModuleUpdateJobClaimResult, error) {
	if err := validateModuleUpdateJobClaimPersistenceState(persisted, admission); err != nil {
		return ModuleUpdateJobClaimResult{}, err
	}
	if persisted.Record.State != ModuleJobStateRunning {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("claim persistence reopen requires RUNNING state, got %s", persisted.Record.State)
	}
	return RecoverPersistedModuleUpdateJobClaim(persisted.Record, admission, persisted.Journal)
}

func validateModuleUpdateJobClaimPersistenceState(
	state ModuleUpdateJobClaimPersistenceState,
	admission ModuleUpdateJobAdmission,
) error {
	if err := validateModuleUpdateJobRecord(state.Record, admission); err != nil {
		return fmt.Errorf("invalid update job claim persistence record: %w", err)
	}

	switch state.Record.State {
	case ModuleJobStatePrepared:
		if state.HasJournal || state.Journal != (ModuleUpdateJobClaimJournalEntry{}) {
			return fmt.Errorf("PREPARED update job persistence must not contain claim journal evidence")
		}
	case ModuleJobStateRunning:
		if !state.HasJournal {
			return fmt.Errorf("RUNNING update job persistence requires claim journal evidence")
		}
		if _, err := RecoverPersistedModuleUpdateJobClaim(state.Record, admission, state.Journal); err != nil {
			return fmt.Errorf("RUNNING update job persistence is not recoverable: %w", err)
		}
	default:
		return fmt.Errorf("unsupported update job claim persistence state %q", state.Record.State)
	}
	return nil
}
