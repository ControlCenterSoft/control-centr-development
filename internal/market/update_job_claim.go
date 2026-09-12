package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const initialModuleUpdateJobStateVersion uint64 = 1

var moduleUpdateWorkerIDPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._:/-]{0,126}[A-Za-z0-9])?$`)

type ModuleUpdateJobRecord struct {
	RecordID                string                  `json:"record_id"`
	JobID                   string                  `json:"job_id"`
	AdmissionID             string                  `json:"admission_id"`
	State                   ModuleLifecycleJobState `json:"state"`
	StateVersion            uint64                  `json:"state_version"`
	JournalSequence         uint64                  `json:"journal_sequence"`
	ClaimID                 string                  `json:"claim_id,omitempty"`
	ClaimedBy               string                  `json:"claimed_by,omitempty"`
	ClaimedFromStateVersion uint64                  `json:"claimed_from_state_version,omitempty"`
	PreserveUserData        bool                    `json:"preserve_user_data"`
	PreserveSecretMaterial  bool                    `json:"preserve_secret_material"`
	ExecutionAuthorized     bool                    `json:"execution_authorized"`
}

type ModuleUpdateJobClaim struct {
	ClaimID                   string `json:"claim_id"`
	RecordID                  string `json:"record_id"`
	JobID                     string `json:"job_id"`
	AdmissionID               string `json:"admission_id"`
	WorkerID                  string `json:"worker_id"`
	ExpectedStateVersion      uint64 `json:"expected_state_version"`
	ClaimedStateVersion       uint64 `json:"claimed_state_version"`
	JournalSequence           uint64 `json:"journal_sequence"`
	AtomicPersistenceRequired bool   `json:"atomic_persistence_required"`
	ExecutionAuthorized       bool   `json:"execution_authorized"`
	ProductionMutationAllowed bool   `json:"production_mutation_allowed"`
}

type ModuleUpdateJobClaimJournalEntry struct {
	JournalID           string                  `json:"journal_id"`
	Sequence            uint64                  `json:"sequence"`
	RecordID            string                  `json:"record_id"`
	JobID               string                  `json:"job_id"`
	AdmissionID         string                  `json:"admission_id"`
	ClaimID             string                  `json:"claim_id"`
	WorkerID            string                  `json:"worker_id"`
	FromState           ModuleLifecycleJobState `json:"from_state"`
	ToState             ModuleLifecycleJobState `json:"to_state"`
	FromStateVersion    uint64                  `json:"from_state_version"`
	ToStateVersion      uint64                  `json:"to_state_version"`
	ExecutionAuthorized bool                    `json:"execution_authorized"`
}

type ModuleUpdateJobClaimResult struct {
	Record                ModuleUpdateJobRecord            `json:"record"`
	Claim                 ModuleUpdateJobClaim             `json:"claim"`
	Journal               ModuleUpdateJobClaimJournalEntry `json:"journal"`
	JournalAppendRequired bool                             `json:"journal_append_required"`
	Replay                bool                             `json:"replay"`
}

func PrepareModuleUpdateJobRecord(admission ModuleUpdateJobAdmission) (ModuleUpdateJobRecord, error) {
	if err := validateModuleUpdateJobAdmissionEnvelope(admission); err != nil {
		return ModuleUpdateJobRecord{}, err
	}

	record := ModuleUpdateJobRecord{
		RecordID:               moduleUpdateJobRecordID(admission),
		JobID:                  admission.JobID,
		AdmissionID:            admission.AdmissionID,
		State:                  ModuleJobStatePrepared,
		StateVersion:           initialModuleUpdateJobStateVersion,
		JournalSequence:        0,
		PreserveUserData:       true,
		PreserveSecretMaterial: true,
		ExecutionAuthorized:    false,
	}
	return record, nil
}

func ClaimModuleUpdateJob(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
	workerID string,
	expectedStateVersion uint64,
) (ModuleUpdateJobClaimResult, error) {
	workerID = strings.TrimSpace(workerID)
	if !moduleUpdateWorkerIDPattern.MatchString(workerID) {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("invalid update worker id %q", workerID)
	}
	if expectedStateVersion == 0 {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("expected state version must be greater than zero")
	}
	if err := validateModuleUpdateJobRecord(record, admission); err != nil {
		return ModuleUpdateJobClaimResult{}, err
	}

	if record.State == ModuleJobStateRunning {
		return replayModuleUpdateJobClaim(record, admission, workerID, expectedStateVersion)
	}
	if record.State != ModuleJobStatePrepared {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job state %s cannot be claimed", record.State)
	}
	if expectedStateVersion != record.StateVersion {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("stale update job state version: got %d want %d", expectedStateVersion, record.StateVersion)
	}
	if record.StateVersion == ^uint64(0) || record.JournalSequence == ^uint64(0) {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job revision overflow")
	}

	transition, err := AdvanceModuleLifecycleJob(record.State, ModuleJobEventStart)
	if err != nil {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job start transition rejected: %w", err)
	}
	if transition.From != ModuleJobStatePrepared || transition.To != ModuleJobStateRunning || transition.Terminal || transition.RecoveryRequired {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job start transition is not a safe PREPARED to RUNNING transition")
	}

	toStateVersion := record.StateVersion + 1
	journalSequence := record.JournalSequence + 1
	claim := buildModuleUpdateJobClaim(record, admission, workerID, expectedStateVersion, toStateVersion, journalSequence)
	journal := buildModuleUpdateJobClaimJournal(record, claim, transition)

	updated := record
	updated.State = transition.To
	updated.StateVersion = toStateVersion
	updated.JournalSequence = journalSequence
	updated.ClaimID = claim.ClaimID
	updated.ClaimedBy = workerID
	updated.ClaimedFromStateVersion = expectedStateVersion

	if err := validateModuleUpdateJobRecord(updated, admission); err != nil {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("invalid claimed update job record: %w", err)
	}
	return ModuleUpdateJobClaimResult{
		Record:                updated,
		Claim:                 claim,
		Journal:               journal,
		JournalAppendRequired: true,
		Replay:                false,
	}, nil
}

func replayModuleUpdateJobClaim(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
	workerID string,
	expectedStateVersion uint64,
) (ModuleUpdateJobClaimResult, error) {
	if expectedStateVersion != record.ClaimedFromStateVersion {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job already claimed at state version %d", record.ClaimedFromStateVersion)
	}
	if workerID != record.ClaimedBy {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("update job already claimed by a different worker")
	}
	claim := buildModuleUpdateJobClaim(
		ModuleUpdateJobRecord{
			RecordID:        record.RecordID,
			JobID:           record.JobID,
			AdmissionID:     record.AdmissionID,
			State:           ModuleJobStatePrepared,
			StateVersion:    record.ClaimedFromStateVersion,
			JournalSequence: record.JournalSequence - 1,
		},
		admission,
		workerID,
		record.ClaimedFromStateVersion,
		record.StateVersion,
		record.JournalSequence,
	)
	if claim.ClaimID != record.ClaimID {
		return ModuleUpdateJobClaimResult{}, fmt.Errorf("persisted update job claim evidence mismatch")
	}
	transition := ModuleLifecycleJobTransition{From: ModuleJobStatePrepared, To: ModuleJobStateRunning}
	journal := buildModuleUpdateJobClaimJournal(ModuleUpdateJobRecord{
		RecordID:        record.RecordID,
		JobID:           record.JobID,
		AdmissionID:     record.AdmissionID,
		State:           ModuleJobStatePrepared,
		StateVersion:    record.ClaimedFromStateVersion,
		JournalSequence: record.JournalSequence - 1,
	}, claim, transition)
	return ModuleUpdateJobClaimResult{
		Record:                record,
		Claim:                 claim,
		Journal:               journal,
		JournalAppendRequired: false,
		Replay:                true,
	}, nil
}

func validateModuleUpdateJobAdmissionEnvelope(admission ModuleUpdateJobAdmission) error {
	if strings.TrimSpace(admission.AdmissionID) == "" || strings.TrimSpace(admission.JobID) == "" {
		return fmt.Errorf("update job admission and job identities are required")
	}
	if admission.Generation == 0 {
		return fmt.Errorf("update job admission generation must be greater than zero")
	}
	if !admission.PreserveUserData || !admission.PreserveSecretMaterial {
		return fmt.Errorf("update job admission must preserve user data and secret material")
	}
	if admission.ExecutionAuthorized {
		return fmt.Errorf("update job admission must not authorize execution")
	}
	if admission.AdmissionID != moduleUpdateJobAdmissionID(admission) {
		return fmt.Errorf("update job admission identity mismatch")
	}
	return nil
}

func validateModuleUpdateJobRecord(record ModuleUpdateJobRecord, admission ModuleUpdateJobAdmission) error {
	if err := validateModuleUpdateJobAdmissionEnvelope(admission); err != nil {
		return err
	}
	if record.RecordID != moduleUpdateJobRecordID(admission) || record.JobID != admission.JobID || record.AdmissionID != admission.AdmissionID {
		return fmt.Errorf("update job durable record does not match admission evidence")
	}
	if !record.PreserveUserData || !record.PreserveSecretMaterial || record.ExecutionAuthorized {
		return fmt.Errorf("update job durable record violates safety contract")
	}

	switch record.State {
	case ModuleJobStatePrepared:
		if record.StateVersion != initialModuleUpdateJobStateVersion || record.JournalSequence != 0 {
			return fmt.Errorf("PREPARED update job durable record has invalid revision")
		}
		if record.ClaimID != "" || record.ClaimedBy != "" || record.ClaimedFromStateVersion != 0 {
			return fmt.Errorf("PREPARED update job durable record must not contain claim evidence")
		}
	case ModuleJobStateRunning:
		if record.StateVersion != initialModuleUpdateJobStateVersion+1 || record.JournalSequence != 1 || record.ClaimedFromStateVersion != initialModuleUpdateJobStateVersion {
			return fmt.Errorf("RUNNING update job durable record has invalid claim revision")
		}
		if strings.TrimSpace(record.ClaimID) == "" || strings.TrimSpace(record.ClaimedBy) == "" {
			return fmt.Errorf("RUNNING update job durable record requires claim evidence")
		}
	default:
		return fmt.Errorf("unsupported update job durable record state %q", record.State)
	}
	return nil
}

func buildModuleUpdateJobClaim(
	record ModuleUpdateJobRecord,
	admission ModuleUpdateJobAdmission,
	workerID string,
	fromStateVersion uint64,
	toStateVersion uint64,
	journalSequence uint64,
) ModuleUpdateJobClaim {
	claim := ModuleUpdateJobClaim{
		RecordID:                  record.RecordID,
		JobID:                     record.JobID,
		AdmissionID:               admission.AdmissionID,
		WorkerID:                  workerID,
		ExpectedStateVersion:      fromStateVersion,
		ClaimedStateVersion:       toStateVersion,
		JournalSequence:           journalSequence,
		AtomicPersistenceRequired: true,
		ExecutionAuthorized:       false,
		ProductionMutationAllowed: false,
	}
	claim.ClaimID = moduleUpdateJobClaimID(claim)
	return claim
}

func buildModuleUpdateJobClaimJournal(
	record ModuleUpdateJobRecord,
	claim ModuleUpdateJobClaim,
	transition ModuleLifecycleJobTransition,
) ModuleUpdateJobClaimJournalEntry {
	entry := ModuleUpdateJobClaimJournalEntry{
		Sequence:            claim.JournalSequence,
		RecordID:            record.RecordID,
		JobID:               record.JobID,
		AdmissionID:         claim.AdmissionID,
		ClaimID:             claim.ClaimID,
		WorkerID:            claim.WorkerID,
		FromState:           transition.From,
		ToState:             transition.To,
		FromStateVersion:    claim.ExpectedStateVersion,
		ToStateVersion:      claim.ClaimedStateVersion,
		ExecutionAuthorized: false,
	}
	entry.JournalID = moduleUpdateJobClaimJournalID(entry)
	return entry
}

func moduleUpdateJobRecordID(admission ModuleUpdateJobAdmission) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		admission.JobID,
		admission.AdmissionID,
		admission.BundleID,
		admission.BundleIdempotencyKey,
		strconv.FormatUint(admission.Generation, 10),
	}, "\x00")))
	return "market-update-record:" + hex.EncodeToString(digest[:])
}

func moduleUpdateJobClaimID(claim ModuleUpdateJobClaim) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		claim.RecordID,
		claim.JobID,
		claim.AdmissionID,
		claim.WorkerID,
		strconv.FormatUint(claim.ExpectedStateVersion, 10),
		strconv.FormatUint(claim.ClaimedStateVersion, 10),
		strconv.FormatUint(claim.JournalSequence, 10),
		strconv.FormatBool(claim.AtomicPersistenceRequired),
		strconv.FormatBool(claim.ExecutionAuthorized),
		strconv.FormatBool(claim.ProductionMutationAllowed),
	}, "\x00")))
	return "market-update-claim:" + hex.EncodeToString(digest[:])
}

func moduleUpdateJobClaimJournalID(entry ModuleUpdateJobClaimJournalEntry) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		entry.RecordID,
		entry.JobID,
		entry.AdmissionID,
		entry.ClaimID,
		entry.WorkerID,
		string(entry.FromState),
		string(entry.ToState),
		strconv.FormatUint(entry.FromStateVersion, 10),
		strconv.FormatUint(entry.ToStateVersion, 10),
		strconv.FormatUint(entry.Sequence, 10),
		strconv.FormatBool(entry.ExecutionAuthorized),
	}, "\x00")))
	return "market-update-claim-journal:" + hex.EncodeToString(digest[:])
}
