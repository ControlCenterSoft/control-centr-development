package networkplanning

import (
	"errors"
	"testing"
)

func TestValidatePreflightPlanRejectsEveryForbiddenAuthorityFlag(t *testing.T) {
	base, err := BuildPreflightPlan(validRequest())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*PreflightPlan)
	}{
		{name: "forwarding", mutate: func(p *PreflightPlan) { p.ForwardingAuthorized = true }},
		{name: "nat", mutate: func(p *PreflightPlan) { p.NATAuthorized = true }},
		{name: "external publication", mutate: func(p *PreflightPlan) { p.ExternalPublicationAuthorized = true }},
		{name: "support remote access", mutate: func(p *PreflightPlan) { p.SupportRemoteAccessAuthorized = true }},
		{name: "connectivity verification disabled", mutate: func(p *PreflightPlan) { p.ConnectivityVerificationRequired = false }},
		{name: "rollback confirmation disabled", mutate: func(p *PreflightPlan) { p.Rollback.OperatorConfirmationRequired = false }},
		{name: "post rollback verification disabled", mutate: func(p *PreflightPlan) { p.Rollback.PostRollbackVerificationRequired = false }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := base
			plan.Probes = append([]ConnectivityProbe(nil), base.Probes...)
			tc.mutate(&plan)
			if err := ValidatePreflightPlan(plan); !errors.Is(err, ErrInvalidPreflightPlan) {
				t.Fatalf("ValidatePreflightPlan() error = %v, want ErrInvalidPreflightPlan", err)
			}
		})
	}
}

func TestValidatePreflightPlanRejectsProbeTargetTamperingEvenWithOldPlanID(t *testing.T) {
	plan, err := BuildPreflightPlan(validRequest())
	if err != nil {
		t.Fatal(err)
	}
	plan.Probes = append([]ConnectivityProbe(nil), plan.Probes...)
	plan.Probes[0].Target = "evil.example.com"
	if err := ValidatePreflightPlan(plan); !errors.Is(err, ErrInvalidPreflightPlan) {
		t.Fatalf("ValidatePreflightPlan() error = %v, want ErrInvalidPreflightPlan", err)
	}
}
