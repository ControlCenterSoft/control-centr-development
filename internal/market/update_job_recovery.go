package market

import "fmt"

// RecoverPersistedModuleUpdateJobClaim revalidates the durable RUNNING record
// together with its immutable PREPARED -> RUNNING claim journal entry after a
// process restart. Recovery never appends a second journal entry and never
// widens execution authority.
func RecoverPersistedModuleUpdateJobClaim(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
	journal ModuleUpdateJobClaimJournalEntry,
) (ModuleUpdateJobClaimResult, error) {
	if err := validateModuleUpdateJobRecord(record, admission); err != nil {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("invalid persisted update job record: %w", err)
	}
	if record.State != ModuleJobStateRunning {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job claim recovery requires RUNNING state, got %s", record.State)
	}

	replayed, err := replayModuleUpdateJobClaim(
		record,
		admission,
		record.ClaimedBy,
		record.ClaimedFromStateVersion,
	)
	if err != nil {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("persisted update job claim cannot be reconstructed: %w", err)
	}
	if journal != replayed.Journal {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("persisted update job claim journal evidence mismatch")
	}
	if journal.JournalID != moduleUpdateJobClaimJournalID(journal) {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("persisted update job claim journal identity mismatch")
	}
	if replayed.Claim.ExecutionAuthorized || replayed.Claim.ProductionMutationAllowed || journal.ExecutionAuthorized {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("persisted update job claim recovery widened execution authority")
	}
	if replayed.JournalAppendRequired || !replayed.Replay {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("persisted update job claim recovery must be replay-only")
	}
	return replayed, nil
}
