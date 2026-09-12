package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type ModuleUpdateJobAdmission struct {
	AdmissionID                 string `json:"admission_id"`
	JobID                       string `json:"job_id"`
	LifecycleIdempotencyKey     string `json:"lifecycle_idempotency_key"`
	BundleID                    string `json:"bundle_id"`
	BundleIdempotencyKey        string `json:"bundle_idempotency_key"`
	ModuleID                    string `json:"module_id"`
	CurrentVersion              string `json:"current_version"`
	TargetVersion               string `json:"target_version"`
	Generation                  uint64 `json:"generation"`
	BundleSHA256                string `json:"bundle_sha256"`
	BundleSizeBytes             uint64 `json:"bundle_size_bytes"`
	MigrationCount              int    `json:"migration_count"`
	RollbackMode                string `json:"rollback_mode"`
	RequirePreMigrationSnapshot bool   `json:"require_pre_migration_snapshot"`
	PreserveUserData            bool   `json:"preserve_user_data"`
	PreserveSecretMaterial      bool   `json:"preserve_secret_material"`
	ExecutionAuthorized         bool   `json:"execution_authorized"`
}

func AdmitModuleUpdateJob(
	request ModuleLifecycleRequest,
	lifecycle ModuleLifecyclePlan,
	job ModuleLifecycleJobPlan,
	bundle ModuleUpdateBundlePlan,
	bundlePayload []byte,
	migrationPayloads map[string][]byte,
) (ModuleUpdateJobAdmission, error) {
	if request.Action != ModuleActionUpdate {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("update job admission requires update action")
	}
	if request.Generation == 0 || request.Generation != bundle.Generation {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("lifecycle and update bundle generation mismatch")
	}

	expectedLifecycle, err := PlanModuleLifecycle(request)
	if err != nil {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("invalid lifecycle request: %w", err)
	}
	if lifecycle != expectedLifecycle {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("lifecycle plan does not match request")
	}
	if lifecycle.Noop {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("no-op lifecycle must not create an update worker admission")
	}

	expectedJob, err := PlanModuleLifecycleJob(lifecycle)
	if err != nil {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("invalid lifecycle job: %w", err)
	}
	if job != expectedJob {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("lifecycle job does not match lifecycle plan")
	}
	if job.InitialState != ModuleJobStatePrepared || job.ExecutionAuthorized || !job.RequiresVerification || !job.RequiresRollbackOnFailure {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("lifecycle job is not safe for update admission")
	}

	if err := ValidateModuleUpdateBundlePlan(bundle); err != nil {
		return ModuleUpdateJobAdmission{}, err
	}
	if bundle.ModuleID != lifecycle.ModuleID || bundle.CurrentVersion != lifecycle.CurrentVersion || bundle.TargetVersion != lifecycle.TargetVersion {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("update bundle does not match lifecycle module/version intent")
	}
	if err := VerifyUpdateBundlePayload(bundle, bundlePayload); err != nil {
		return ModuleUpdateJobAdmission{}, err
	}
	if len(migrationPayloads) != len(bundle.Migrations) {
		return ModuleUpdateJobAdmission{}, fmt.Errorf("migration payload set size mismatch: got %d want %d", len(migrationPayloads), len(bundle.Migrations))
	}
	for _, migration := range bundle.Migrations {
		payload, ok := migrationPayloads[migration.ID]
		if !ok {
			return ModuleUpdateJobAdmission{}, fmt.Errorf("migration payload %q is required", migration.ID)
		}
		if err := VerifyMigrationArtifactPayload(bundle, migration.ID, payload); err != nil {
			return ModuleUpdateJobAdmission{}, err
		}
	}

	admission := ModuleUpdateJobAdmission{
		JobID:                       job.JobID,
		LifecycleIdempotencyKey:     lifecycle.IdempotencyKey,
		BundleID:                    bundle.BundleID,
		BundleIdempotencyKey:        bundle.IdempotencyKey,
		ModuleID:                    bundle.ModuleID,
		CurrentVersion:              bundle.CurrentVersion,
		TargetVersion:               bundle.TargetVersion,
		Generation:                  bundle.Generation,
		BundleSHA256:                bundle.BundleSHA256,
		BundleSizeBytes:             bundle.BundleSizeBytes,
		MigrationCount:              len(bundle.Migrations),
		RollbackMode:                bundle.RollbackMode,
		RequirePreMigrationSnapshot: bundle.RequirePreMigrationSnapshot,
		PreserveUserData:            true,
		PreserveSecretMaterial:      true,
		ExecutionAuthorized:         false,
	}
	admission.AdmissionID = moduleUpdateJobAdmissionID(admission)
	return admission, nil
}

func ValidateModuleUpdateJobAdmission(
	admission ModuleUpdateJobAdmission,
	request ModuleLifecycleRequest,
	lifecycle ModuleLifecyclePlan,
	job ModuleLifecycleJobPlan,
	bundle ModuleUpdateBundlePlan,
	bundlePayload []byte,
	migrationPayloads map[string][]byte,
) error {
	if !admission.PreserveUserData || !admission.PreserveSecretMaterial {
		return fmt.Errorf("update job admission must preserve user data and secret material")
	}
	if admission.ExecutionAuthorized {
		return fmt.Errorf("update job admission must not authorize execution")
	}

	rebuilt, err := AdmitModuleUpdateJob(request, lifecycle, job, bundle, bundlePayload, migrationPayloads)
	if err != nil {
		return err
	}
	if admission != rebuilt {
		return fmt.Errorf("update job admission evidence mismatch")
	}
	return nil
}

func moduleUpdateJobAdmissionID(admission ModuleUpdateJobAdmission) string {
	material := strings.Join([]string{
		admission.JobID,
		admission.LifecycleIdempotencyKey,
		admission.BundleID,
		admission.BundleIdempotencyKey,
		admission.ModuleID,
		admission.CurrentVersion,
		admission.TargetVersion,
		strconv.FormatUint(admission.Generation, 10),
		admission.BundleSHA256,
		strconv.FormatUint(admission.BundleSizeBytes, 10),
		strconv.Itoa(admission.MigrationCount),
		admission.RollbackMode,
		strconv.FormatBool(admission.RequirePreMigrationSnapshot),
		strconv.FormatBool(admission.PreserveUserData),
		strconv.FormatBool(admission.PreserveSecretMaterial),
		strconv.FormatBool(admission.ExecutionAuthorized),
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return "market-update-admission:" + hex.EncodeToString(digest[:])
}
