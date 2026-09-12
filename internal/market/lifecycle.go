package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type ModuleLifecycleState string

const (
	ModuleStateAbsent     ModuleLifecycleState = "ABSENT"
	ModuleStateInstalling ModuleLifecycleState = "INSTALLING"
	ModuleStateActive     ModuleLifecycleState = "ACTIVE"
	ModuleStateUpdating   ModuleLifecycleState = "UPDATING"
	ModuleStateDisabling  ModuleLifecycleState = "DISABLING"
	ModuleStateDisabled   ModuleLifecycleState = "DISABLED"
)

type ModuleLifecycleAction string

const (
	ModuleActionInstall ModuleLifecycleAction = "install"
	ModuleActionUpdate  ModuleLifecycleAction = "update"
	ModuleActionDisable ModuleLifecycleAction = "disable"
)

type ModuleLifecycleRequest struct {
	ModuleID       string
	Action         ModuleLifecycleAction
	CurrentState   ModuleLifecycleState
	CurrentVersion string
	TargetVersion  string
	Generation     uint64
}

type ModuleLifecyclePlan struct {
	ModuleID               string
	Action                 ModuleLifecycleAction
	FromState              ModuleLifecycleState
	InProgressState        ModuleLifecycleState
	SuccessState           ModuleLifecycleState
	RollbackState          ModuleLifecycleState
	CurrentVersion         string
	TargetVersion          string
	RollbackVersion        string
	IdempotencyKey         string
	Noop                   bool
	PreserveUserData       bool
	PreserveSecretMaterial bool
}

var (
	lifecycleModuleIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$`)
	lifecycleVersionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

func PlanModuleLifecycle(request ModuleLifecycleRequest) (ModuleLifecyclePlan, error) {
	request.ModuleID = strings.TrimSpace(request.ModuleID)
	request.CurrentVersion = strings.TrimSpace(request.CurrentVersion)
	request.TargetVersion = strings.TrimSpace(request.TargetVersion)

	if !lifecycleModuleIDPattern.MatchString(request.ModuleID) {
		return ModuleLifecyclePlan{}, fmt.Errorf("invalid module id %q", request.ModuleID)
	}
	if request.Generation == 0 {
		return ModuleLifecyclePlan{}, fmt.Errorf("generation must be greater than zero")
	}
	if !validModuleLifecycleState(request.CurrentState) {
		return ModuleLifecyclePlan{}, fmt.Errorf("unsupported current state %q", request.CurrentState)
	}

	plan := ModuleLifecyclePlan{
		ModuleID:               request.ModuleID,
		Action:                 request.Action,
		FromState:              request.CurrentState,
		CurrentVersion:         request.CurrentVersion,
		TargetVersion:          request.TargetVersion,
		PreserveUserData:       true,
		PreserveSecretMaterial: true,
	}

	switch request.Action {
	case ModuleActionInstall:
		if !lifecycleVersionPattern.MatchString(request.TargetVersion) {
			return ModuleLifecyclePlan{}, fmt.Errorf("install target version must be semantic")
		}
		if request.CurrentState == ModuleStateActive && request.CurrentVersion == request.TargetVersion {
			return finalizeNoopLifecyclePlan(plan, request), nil
		}
		if request.CurrentState != ModuleStateAbsent {
			return ModuleLifecyclePlan{}, fmt.Errorf("install requires ABSENT state, got %s", request.CurrentState)
		}
		if request.CurrentVersion != "" {
			return ModuleLifecyclePlan{}, fmt.Errorf("ABSENT module must not have a current version")
		}
		plan.InProgressState = ModuleStateInstalling
		plan.SuccessState = ModuleStateActive
		plan.RollbackState = ModuleStateAbsent

	case ModuleActionUpdate:
		if request.CurrentState != ModuleStateActive {
			return ModuleLifecyclePlan{}, fmt.Errorf("update requires ACTIVE state, got %s", request.CurrentState)
		}
		if !lifecycleVersionPattern.MatchString(request.CurrentVersion) || !lifecycleVersionPattern.MatchString(request.TargetVersion) {
			return ModuleLifecyclePlan{}, fmt.Errorf("update current and target versions must be semantic")
		}
		comparison, err := compareLifecycleVersions(request.CurrentVersion, request.TargetVersion)
		if err != nil {
			return ModuleLifecyclePlan{}, err
		}
		if comparison == 0 {
			return finalizeNoopLifecyclePlan(plan, request), nil
		}
		if comparison > 0 {
			return ModuleLifecyclePlan{}, fmt.Errorf("update target %s is older than current %s", request.TargetVersion, request.CurrentVersion)
		}
		plan.InProgressState = ModuleStateUpdating
		plan.SuccessState = ModuleStateActive
		plan.RollbackState = ModuleStateActive
		plan.RollbackVersion = request.CurrentVersion

	case ModuleActionDisable:
		if request.TargetVersion != "" && request.TargetVersion != request.CurrentVersion {
			return ModuleLifecyclePlan{}, fmt.Errorf("disable must not change module version")
		}
		if request.CurrentState == ModuleStateDisabled {
			if !lifecycleVersionPattern.MatchString(request.CurrentVersion) {
				return ModuleLifecyclePlan{}, fmt.Errorf("disabled module current version must be semantic")
			}
			plan.TargetVersion = request.CurrentVersion
			return finalizeNoopLifecyclePlan(plan, request), nil
		}
		if request.CurrentState != ModuleStateActive {
			return ModuleLifecyclePlan{}, fmt.Errorf("disable requires ACTIVE state, got %s", request.CurrentState)
		}
		if !lifecycleVersionPattern.MatchString(request.CurrentVersion) {
			return ModuleLifecyclePlan{}, fmt.Errorf("active module current version must be semantic")
		}
		plan.TargetVersion = request.CurrentVersion
		plan.InProgressState = ModuleStateDisabling
		plan.SuccessState = ModuleStateDisabled
		plan.RollbackState = ModuleStateActive
		plan.RollbackVersion = request.CurrentVersion

	default:
		return ModuleLifecyclePlan{}, fmt.Errorf("unsupported lifecycle action %q", request.Action)
	}

	plan.IdempotencyKey = lifecycleIdempotencyKey(request, plan.TargetVersion)
	return plan, nil
}

func finalizeNoopLifecyclePlan(plan ModuleLifecyclePlan, request ModuleLifecycleRequest) ModuleLifecyclePlan {
	plan.Noop = true
	plan.InProgressState = request.CurrentState
	plan.SuccessState = request.CurrentState
	plan.RollbackState = request.CurrentState
	plan.RollbackVersion = request.CurrentVersion
	if plan.TargetVersion == "" {
		plan.TargetVersion = request.CurrentVersion
	}
	plan.IdempotencyKey = lifecycleIdempotencyKey(request, plan.TargetVersion)
	return plan
}

func validModuleLifecycleState(state ModuleLifecycleState) bool {
	switch state {
	case ModuleStateAbsent, ModuleStateInstalling, ModuleStateActive, ModuleStateUpdating, ModuleStateDisabling, ModuleStateDisabled:
		return true
	default:
		return false
	}
}

func lifecycleIdempotencyKey(request ModuleLifecycleRequest, targetVersion string) string {
	material := strings.Join([]string{
		request.ModuleID,
		string(request.Action),
		strconv.FormatUint(request.Generation, 10),
		string(request.CurrentState),
		request.CurrentVersion,
		targetVersion,
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return "market-lifecycle:" + hex.EncodeToString(digest[:])
}

func compareLifecycleVersions(current, target string) (int, error) {
	currentParts, err := parseLifecycleVersion(current)
	if err != nil {
		return 0, err
	}
	targetParts, err := parseLifecycleVersion(target)
	if err != nil {
		return 0, err
	}
	for index := 0; index < len(currentParts); index++ {
		if currentParts[index] < targetParts[index] {
			return -1, nil
		}
		if currentParts[index] > targetParts[index] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseLifecycleVersion(version string) ([3]uint64, error) {
	var parsed [3]uint64
	if !lifecycleVersionPattern.MatchString(version) {
		return parsed, fmt.Errorf("invalid semantic version %q", version)
	}
	parts := strings.Split(version, ".")
	for index, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return parsed, fmt.Errorf("invalid semantic version %q: %w", version, err)
		}
		parsed[index] = value
	}
	return parsed, nil
}
