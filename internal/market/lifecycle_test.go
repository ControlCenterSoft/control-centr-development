package market

import "testing"

func TestPlanModuleLifecycleInstall(t *testing.T) {
	request := ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionInstall, CurrentState: ModuleStateAbsent, TargetVersion: "1.2.3", Generation: 7}
	plan, err := PlanModuleLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InProgressState != ModuleStateInstalling || plan.SuccessState != ModuleStateActive || plan.RollbackState != ModuleStateAbsent {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if !plan.PreserveUserData || !plan.PreserveSecretMaterial {
		t.Fatalf("preservation flags must be true: %#v", plan)
	}
	second, err := PlanModuleLifecycle(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("idempotency key changed: %q != %q", plan.IdempotencyKey, second.IdempotencyKey)
	}
}

func TestPlanModuleLifecycleInstallAlreadySatisfiedIsNoop(t *testing.T) {
	plan, err := PlanModuleLifecycle(ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionInstall, CurrentState: ModuleStateActive, CurrentVersion: "1.2.3", TargetVersion: "1.2.3", Generation: 8})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Noop || plan.SuccessState != ModuleStateActive {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanModuleLifecycleUpdate(t *testing.T) {
	plan, err := PlanModuleLifecycle(ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionUpdate, CurrentState: ModuleStateActive, CurrentVersion: "1.2.3", TargetVersion: "1.3.0", Generation: 9})
	if err != nil {
		t.Fatal(err)
	}
	if plan.InProgressState != ModuleStateUpdating || plan.SuccessState != ModuleStateActive || plan.RollbackState != ModuleStateActive || plan.RollbackVersion != "1.2.3" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanModuleLifecycleUpdateSameVersionIsNoop(t *testing.T) {
	plan, err := PlanModuleLifecycle(ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionUpdate, CurrentState: ModuleStateActive, CurrentVersion: "1.2.3", TargetVersion: "1.2.3", Generation: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Noop {
		t.Fatalf("expected noop: %#v", plan)
	}
}

func TestPlanModuleLifecycleRejectsDowngrade(t *testing.T) {
	_, err := PlanModuleLifecycle(ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionUpdate, CurrentState: ModuleStateActive, CurrentVersion: "2.0.0", TargetVersion: "1.9.9", Generation: 11})
	if err == nil {
		t.Fatal("expected downgrade rejection")
	}
}

func TestPlanModuleLifecycleDisable(t *testing.T) {
	plan, err := PlanModuleLifecycle(ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionDisable, CurrentState: ModuleStateActive, CurrentVersion: "1.2.3", Generation: 12})
	if err != nil {
		t.Fatal(err)
	}
	if plan.InProgressState != ModuleStateDisabling || plan.SuccessState != ModuleStateDisabled || plan.RollbackState != ModuleStateActive || plan.RollbackVersion != "1.2.3" || plan.TargetVersion != "1.2.3" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanModuleLifecycleDisableAlreadySatisfiedIsNoop(t *testing.T) {
	plan, err := PlanModuleLifecycle(ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionDisable, CurrentState: ModuleStateDisabled, CurrentVersion: "1.2.3", Generation: 13})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Noop || plan.TargetVersion != "1.2.3" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanModuleLifecycleRejectsInvalidTransitions(t *testing.T) {
	tests := []ModuleLifecycleRequest{
		{ModuleID: "monitoring", Action: ModuleActionUpdate, CurrentState: ModuleStateDisabled, CurrentVersion: "1.2.3", TargetVersion: "1.3.0", Generation: 1},
		{ModuleID: "monitoring", Action: ModuleActionDisable, CurrentState: ModuleStateAbsent, Generation: 1},
		{ModuleID: "Monitoring", Action: ModuleActionInstall, CurrentState: ModuleStateAbsent, TargetVersion: "1.0.0", Generation: 1},
		{ModuleID: "monitoring", Action: ModuleActionInstall, CurrentState: ModuleStateAbsent, TargetVersion: "1.0.0", Generation: 0},
	}
	for _, request := range tests {
		if _, err := PlanModuleLifecycle(request); err == nil {
			t.Fatalf("expected rejection for %#v", request)
		}
	}
}

func TestPlanModuleLifecycleIdempotencyKeyChangesWithIntent(t *testing.T) {
	base := ModuleLifecycleRequest{ModuleID: "monitoring", Action: ModuleActionUpdate, CurrentState: ModuleStateActive, CurrentVersion: "1.2.3", TargetVersion: "1.3.0", Generation: 20}
	first, err := PlanModuleLifecycle(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Generation++
	second, err := PlanModuleLifecycle(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.IdempotencyKey == second.IdempotencyKey {
		t.Fatal("generation change must change idempotency key")
	}
	base.Generation--
	base.TargetVersion = "1.4.0"
	third, err := PlanModuleLifecycle(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.IdempotencyKey == third.IdempotencyKey {
		t.Fatal("target version change must change idempotency key")
	}
}
