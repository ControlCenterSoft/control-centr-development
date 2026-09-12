package market

import "testing"

func validInstallLifecyclePlan() ModuleLifecyclePlan {
	return ModuleLifecyclePlan{
		ModuleID:               "dns-tools",
		Action:                 ModuleActionInstall,
		FromState:              ModuleStateAbsent,
		InProgressState:        ModuleStateInstalling,
		SuccessState:           ModuleStateActive,
		RollbackState:          ModuleStateAbsent,
		TargetVersion:          "1.2.3",
		IdempotencyKey:         "market-lifecycle:abc",
		PreserveUserData:       true,
		PreserveSecretMaterial: true,
	}
}

func TestPlanModuleLifecycleJobDeterministicAndNonAuthorizing(t *testing.T) {
	input := validInstallLifecyclePlan()
	first, err := PlanModuleLifecycleJob(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanModuleLifecycleJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("job plan is not deterministic: %#v != %#v", first, second)
	}
	if first.ExecutionAuthorized {
		t.Fatal("job planning must never authorize execution")
	}
	if first.InitialState != ModuleJobStatePrepared || !first.RequiresVerification || !first.RequiresRollbackOnFailure {
		t.Fatalf("unexpected install job policy: %#v", first)
	}
}

func TestPlanModuleLifecycleJobNoopStartsSucceeded(t *testing.T) {
	input := ModuleLifecyclePlan{
		ModuleID: "dns-tools", Action: ModuleActionUpdate,
		FromState: ModuleStateActive, InProgressState: ModuleStateActive, SuccessState: ModuleStateActive, RollbackState: ModuleStateActive,
		CurrentVersion: "1.2.3", TargetVersion: "1.2.3", RollbackVersion: "1.2.3",
		IdempotencyKey: "market-lifecycle:no-op", Noop: true, PreserveUserData: true, PreserveSecretMaterial: true,
	}
	job, err := PlanModuleLifecycleJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.InitialState != ModuleJobStateSucceeded || job.RequiresVerification || job.RequiresRollbackOnFailure {
		t.Fatalf("unexpected no-op job plan: %#v", job)
	}
}

func TestPlanModuleLifecycleJobRejectsUnsafePreservation(t *testing.T) {
	input := validInstallLifecyclePlan()
	input.PreserveSecretMaterial = false
	if _, err := PlanModuleLifecycleJob(input); err == nil {
		t.Fatal("expected preservation rejection")
	}
}

func TestPlanModuleLifecycleJobRejectsMalformedUpdateRollback(t *testing.T) {
	input := ModuleLifecyclePlan{
		ModuleID: "dns-tools", Action: ModuleActionUpdate,
		FromState: ModuleStateActive, InProgressState: ModuleStateUpdating, SuccessState: ModuleStateActive, RollbackState: ModuleStateActive,
		CurrentVersion: "1.2.3", TargetVersion: "1.3.0", RollbackVersion: "1.2.2",
		IdempotencyKey: "market-lifecycle:update", PreserveUserData: true, PreserveSecretMaterial: true,
	}
	if _, err := PlanModuleLifecycleJob(input); err == nil {
		t.Fatal("expected rollback version rejection")
	}
}

func TestModuleLifecycleJobSuccessPath(t *testing.T) {
	state := ModuleJobStatePrepared
	for _, event := range []ModuleLifecycleJobEvent{ModuleJobEventStart, ModuleJobEventApplySucceeded, ModuleJobEventVerifySucceeded} {
		transition, err := AdvanceModuleLifecycleJob(state, event)
		if err != nil {
			t.Fatal(err)
		}
		state = transition.To
	}
	if state != ModuleJobStateSucceeded {
		t.Fatalf("got %s", state)
	}
}

func TestModuleLifecycleJobPostMutationFailureForcesRollback(t *testing.T) {
	transition, err := AdvanceModuleLifecycleJob(ModuleJobStateRunning, ModuleJobEventApplyFailedAfterMutation)
	if err != nil {
		t.Fatal(err)
	}
	if transition.To != ModuleJobStateRollingBack || transition.Terminal {
		t.Fatalf("unexpected transition: %#v", transition)
	}
}

func TestModuleLifecycleJobPreMutationFailureCanFailClosed(t *testing.T) {
	transition, err := AdvanceModuleLifecycleJob(ModuleJobStateRunning, ModuleJobEventApplyFailedBeforeMutation)
	if err != nil {
		t.Fatal(err)
	}
	if transition.To != ModuleJobStateFailed || !transition.Terminal || transition.RecoveryRequired {
		t.Fatalf("unexpected transition: %#v", transition)
	}
}

func TestModuleLifecycleJobVerifyFailureForcesRollback(t *testing.T) {
	transition, err := AdvanceModuleLifecycleJob(ModuleJobStateVerifying, ModuleJobEventVerifyFailed)
	if err != nil {
		t.Fatal(err)
	}
	if transition.To != ModuleJobStateRollingBack || transition.Terminal {
		t.Fatalf("unexpected transition: %#v", transition)
	}
}

func TestModuleLifecycleJobRollbackFailureRequiresRecovery(t *testing.T) {
	transition, err := AdvanceModuleLifecycleJob(ModuleJobStateRollingBack, ModuleJobEventRollbackFailed)
	if err != nil {
		t.Fatal(err)
	}
	if transition.To != ModuleJobStateFailed || !transition.Terminal || !transition.RecoveryRequired {
		t.Fatalf("unexpected transition: %#v", transition)
	}
}

func TestModuleLifecycleJobTerminalStateCannotReopen(t *testing.T) {
	for _, state := range []ModuleLifecycleJobState{ModuleJobStateSucceeded, ModuleJobStateRolledBack, ModuleJobStateFailed} {
		if _, err := AdvanceModuleLifecycleJob(state, ModuleJobEventStart); err == nil {
			t.Fatalf("expected terminal rejection for %s", state)
		}
	}
}
