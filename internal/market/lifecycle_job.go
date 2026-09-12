package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type ModuleLifecycleJobState string

const (
	ModuleJobStatePrepared    ModuleLifecycleJobState = "PREPARED"
	ModuleJobStateRunning     ModuleLifecycleJobState = "RUNNING"
	ModuleJobStateVerifying   ModuleLifecycleJobState = "VERIFYING"
	ModuleJobStateSucceeded   ModuleLifecycleJobState = "SUCCEEDED"
	ModuleJobStateRollingBack ModuleLifecycleJobState = "ROLLING_BACK"
	ModuleJobStateRolledBack  ModuleLifecycleJobState = "ROLLED_BACK"
	ModuleJobStateFailed      ModuleLifecycleJobState = "FAILED"
)

type ModuleLifecycleJobEvent string

const (
	ModuleJobEventStart                     ModuleLifecycleJobEvent = "start"
	ModuleJobEventApplySucceeded            ModuleLifecycleJobEvent = "apply_succeeded"
	ModuleJobEventApplyFailedBeforeMutation ModuleLifecycleJobEvent = "apply_failed_before_mutation"
	ModuleJobEventApplyFailedAfterMutation  ModuleLifecycleJobEvent = "apply_failed_after_mutation"
	ModuleJobEventVerifySucceeded           ModuleLifecycleJobEvent = "verify_succeeded"
	ModuleJobEventVerifyFailed              ModuleLifecycleJobEvent = "verify_failed"
	ModuleJobEventRollbackSucceeded         ModuleLifecycleJobEvent = "rollback_succeeded"
	ModuleJobEventRollbackFailed            ModuleLifecycleJobEvent = "rollback_failed"
)

type ModuleLifecycleJobPlan struct {
	JobID                     string
	LifecycleIdempotencyKey   string
	ModuleID                  string
	Action                    ModuleLifecycleAction
	TargetVersion             string
	InitialState              ModuleLifecycleJobState
	Noop                      bool
	PreserveUserData          bool
	PreserveSecretMaterial    bool
	RequiresVerification      bool
	RequiresRollbackOnFailure bool
	ExecutionAuthorized       bool
}

type ModuleLifecycleJobTransition struct {
	From             ModuleLifecycleJobState
	To               ModuleLifecycleJobState
	Terminal         bool
	RecoveryRequired bool
}

var lifecycleJobModuleIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$`)

func PlanModuleLifecycleJob(lifecycle ModuleLifecyclePlan) (ModuleLifecycleJobPlan, error) {
	if !lifecycleJobModuleIDPattern.MatchString(strings.TrimSpace(lifecycle.ModuleID)) {
		return ModuleLifecycleJobPlan{}, fmt.Errorf("invalid lifecycle module id %q", lifecycle.ModuleID)
	}
	if strings.TrimSpace(lifecycle.IdempotencyKey) == "" {
		return ModuleLifecycleJobPlan{}, fmt.Errorf("lifecycle idempotency key is required")
	}
	if !lifecycle.PreserveUserData || !lifecycle.PreserveSecretMaterial {
		return ModuleLifecycleJobPlan{}, fmt.Errorf("lifecycle plan must preserve user data and secret material")
	}
	if err := validateLifecycleJobShape(lifecycle); err != nil {
		return ModuleLifecycleJobPlan{}, err
	}

	job := ModuleLifecycleJobPlan{
		JobID:                     lifecycleJobID(lifecycle),
		LifecycleIdempotencyKey:   lifecycle.IdempotencyKey,
		ModuleID:                  lifecycle.ModuleID,
		Action:                    lifecycle.Action,
		TargetVersion:             lifecycle.TargetVersion,
		InitialState:              ModuleJobStatePrepared,
		Noop:                      lifecycle.Noop,
		PreserveUserData:          true,
		PreserveSecretMaterial:    true,
		RequiresVerification:      true,
		RequiresRollbackOnFailure: true,
		ExecutionAuthorized:       false,
	}
	if lifecycle.Noop {
		job.InitialState = ModuleJobStateSucceeded
		job.RequiresVerification = false
		job.RequiresRollbackOnFailure = false
	}
	return job, nil
}

func validateLifecycleJobShape(lifecycle ModuleLifecyclePlan) error {
	if lifecycle.Noop {
		if lifecycle.FromState != lifecycle.InProgressState || lifecycle.FromState != lifecycle.SuccessState || lifecycle.FromState != lifecycle.RollbackState {
			return fmt.Errorf("noop lifecycle plan must remain in state %s", lifecycle.FromState)
		}
		if lifecycle.RollbackVersion != lifecycle.CurrentVersion {
			return fmt.Errorf("noop lifecycle rollback version must equal current version")
		}
		return nil
	}

	switch lifecycle.Action {
	case ModuleActionInstall:
		if lifecycle.FromState != ModuleStateAbsent || lifecycle.InProgressState != ModuleStateInstalling || lifecycle.SuccessState != ModuleStateActive || lifecycle.RollbackState != ModuleStateAbsent {
			return fmt.Errorf("invalid install lifecycle state shape")
		}
		if lifecycle.CurrentVersion != "" || lifecycle.RollbackVersion != "" || strings.TrimSpace(lifecycle.TargetVersion) == "" {
			return fmt.Errorf("invalid install lifecycle version shape")
		}
	case ModuleActionUpdate:
		if lifecycle.FromState != ModuleStateActive || lifecycle.InProgressState != ModuleStateUpdating || lifecycle.SuccessState != ModuleStateActive || lifecycle.RollbackState != ModuleStateActive {
			return fmt.Errorf("invalid update lifecycle state shape")
		}
		if strings.TrimSpace(lifecycle.CurrentVersion) == "" || strings.TrimSpace(lifecycle.TargetVersion) == "" || lifecycle.RollbackVersion != lifecycle.CurrentVersion {
			return fmt.Errorf("invalid update lifecycle version shape")
		}
	case ModuleActionDisable:
		if lifecycle.FromState != ModuleStateActive || lifecycle.InProgressState != ModuleStateDisabling || lifecycle.SuccessState != ModuleStateDisabled || lifecycle.RollbackState != ModuleStateActive {
			return fmt.Errorf("invalid disable lifecycle state shape")
		}
		if strings.TrimSpace(lifecycle.CurrentVersion) == "" || lifecycle.TargetVersion != lifecycle.CurrentVersion || lifecycle.RollbackVersion != lifecycle.CurrentVersion {
			return fmt.Errorf("invalid disable lifecycle version shape")
		}
	default:
		return fmt.Errorf("unsupported lifecycle action %q", lifecycle.Action)
	}
	return nil
}

func lifecycleJobID(lifecycle ModuleLifecyclePlan) string {
	material := strings.Join([]string{
		lifecycle.IdempotencyKey,
		lifecycle.ModuleID,
		string(lifecycle.Action),
		string(lifecycle.FromState),
		string(lifecycle.InProgressState),
		string(lifecycle.SuccessState),
		string(lifecycle.RollbackState),
		lifecycle.CurrentVersion,
		lifecycle.TargetVersion,
		lifecycle.RollbackVersion,
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return "market-job:" + hex.EncodeToString(digest[:])
}

func AdvanceModuleLifecycleJob(state ModuleLifecycleJobState, event ModuleLifecycleJobEvent) (ModuleLifecycleJobTransition, error) {
	transition := ModuleLifecycleJobTransition{From: state}

	switch state {
	case ModuleJobStatePrepared:
		if event != ModuleJobEventStart {
			return ModuleLifecycleJobTransition{}, fmt.Errorf("PREPARED job requires start event")
		}
		transition.To = ModuleJobStateRunning
	case ModuleJobStateRunning:
		switch event {
		case ModuleJobEventApplySucceeded:
			transition.To = ModuleJobStateVerifying
		case ModuleJobEventApplyFailedBeforeMutation:
			transition.To = ModuleJobStateFailed
			transition.Terminal = true
		case ModuleJobEventApplyFailedAfterMutation:
			transition.To = ModuleJobStateRollingBack
		default:
			return ModuleLifecycleJobTransition{}, fmt.Errorf("RUNNING job does not accept event %q", event)
		}
	case ModuleJobStateVerifying:
		switch event {
		case ModuleJobEventVerifySucceeded:
			transition.To = ModuleJobStateSucceeded
			transition.Terminal = true
		case ModuleJobEventVerifyFailed:
			transition.To = ModuleJobStateRollingBack
		default:
			return ModuleLifecycleJobTransition{}, fmt.Errorf("VERIFYING job does not accept event %q", event)
		}
	case ModuleJobStateRollingBack:
		switch event {
		case ModuleJobEventRollbackSucceeded:
			transition.To = ModuleJobStateRolledBack
			transition.Terminal = true
		case ModuleJobEventRollbackFailed:
			transition.To = ModuleJobStateFailed
			transition.Terminal = true
			transition.RecoveryRequired = true
		default:
			return ModuleLifecycleJobTransition{}, fmt.Errorf("ROLLING_BACK job does not accept event %q", event)
		}
	case ModuleJobStateSucceeded, ModuleJobStateRolledBack, ModuleJobStateFailed:
		return ModuleLifecycleJobTransition{}, fmt.Errorf("terminal job state %s cannot transition", state)
	default:
		return ModuleLifecycleJobTransition{}, fmt.Errorf("unsupported job state %q", state)
	}

	return transition, nil
}
