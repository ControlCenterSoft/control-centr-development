package networkplanning

import (
	"errors"
	"testing"
)

func validRequest() PreflightRequest {
	return PreflightRequest{
		NodeID:            "node-01",
		BaseRevisionID:    "rev-100",
		PlannedRevisionID: "rev-101",
		Probes: []ConnectivityProbe{
			{ID: "ntp", Kind: ProbeNTPReachability, SourceInterfaceID: "eth1", Target: "NTP.EXAMPLE.COM"},
			{ID: "mgmt", Kind: ProbeManagementReachability, SourceInterfaceID: "eth0", Target: "192.0.2.10"},
			{ID: "dns", Kind: ProbeDNSResolution, SourceInterfaceID: "eth0", Target: "CONTROL.EXAMPLE.COM"},
			{ID: "gw", Kind: ProbeGatewayReachability, SourceInterfaceID: "eth1", Target: "2001:db8::1"},
		},
	}
}

func TestBuildPreflightPlanCanonicalizesAndFailsClosed(t *testing.T) {
	plan, err := BuildPreflightPlan(validRequest())
	if err != nil {
		t.Fatalf("BuildPreflightPlan() error = %v", err)
	}
	if err := ValidatePreflightPlan(plan); err != nil {
		t.Fatalf("ValidatePreflightPlan() error = %v", err)
	}
	if plan.PlanID == "" || plan.Schema != PreflightSchemaVersion {
		t.Fatalf("unexpected plan identity: %#v", plan)
	}
	if plan.Probes[0].Kind != ProbeDNSResolution || plan.Probes[0].Target != "control.example.com" {
		t.Fatalf("probes are not canonical: %#v", plan.Probes)
	}
	if !plan.ConnectivityVerificationRequired || !plan.Rollback.OperatorConfirmationRequired || !plan.Rollback.PostRollbackVerificationRequired {
		t.Fatalf("required safety gates are missing: %#v", plan)
	}
	if plan.MutationAuthorized || plan.ApplyAuthorized || plan.AutomaticRollbackAuthorized || plan.ForwardingAuthorized || plan.NATAuthorized || plan.ExternalPublicationAuthorized || plan.SupportRemoteAccessAuthorized || plan.Rollback.ExecutionAuthorized {
		t.Fatalf("plan unexpectedly grants runtime authority: %#v", plan)
	}
}

func TestBuildPreflightPlanIsDeterministic(t *testing.T) {
	request := validRequest()
	first, err := BuildPreflightPlan(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Probes[0], request.Probes[3] = request.Probes[3], request.Probes[0]
	second, err := BuildPreflightPlan(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.PlanID != second.PlanID {
		t.Fatalf("plan IDs differ: %s != %s", first.PlanID, second.PlanID)
	}
	if !equalProbes(first.Probes, second.Probes) {
		t.Fatalf("canonical probes differ: %#v != %#v", first.Probes, second.Probes)
	}
}

func TestBuildPreflightPlanRequiresManagementProbe(t *testing.T) {
	request := validRequest()
	request.Probes = request.Probes[0:1]
	_, err := BuildPreflightPlan(request)
	if !errors.Is(err, ErrInvalidPreflightPlan) {
		t.Fatalf("error = %v, want ErrInvalidPreflightPlan", err)
	}
}

func TestBuildPreflightPlanRejectsUnsafeTargets(t *testing.T) {
	tests := []struct {
		name   string
		kind   ProbeKind
		target string
	}{
		{name: "URL", kind: ProbeManagementReachability, target: "https://192.0.2.1/check"},
		{name: "path", kind: ProbeManagementReachability, target: "192.0.2.1/path"},
		{name: "control", kind: ProbeManagementReachability, target: "192.0.2.1\n"},
		{name: "gateway hostname", kind: ProbeGatewayReachability, target: "gw.example.com"},
		{name: "DNS IP", kind: ProbeDNSResolution, target: "192.0.2.53"},
		{name: "short hostname", kind: ProbeDNSResolution, target: "resolver"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			request.Probes[1] = ConnectivityProbe{ID: "mgmt", Kind: tc.kind, SourceInterfaceID: "eth0", Target: tc.target}
			if tc.kind != ProbeManagementReachability {
				request.Probes = append(request.Probes, ConnectivityProbe{ID: "mgmt-required", Kind: ProbeManagementReachability, SourceInterfaceID: "eth0", Target: "192.0.2.44"})
			}
			_, err := BuildPreflightPlan(request)
			if !errors.Is(err, ErrInvalidPreflightPlan) {
				t.Fatalf("error = %v, want ErrInvalidPreflightPlan", err)
			}
		})
	}
}

func TestBuildPreflightPlanRejectsDuplicateProbeIdentityAndBinding(t *testing.T) {
	request := validRequest()
	request.Probes = append(request.Probes, request.Probes[1])
	if _, err := BuildPreflightPlan(request); !errors.Is(err, ErrInvalidPreflightPlan) {
		t.Fatalf("duplicate ID error = %v", err)
	}

	request = validRequest()
	duplicate := request.Probes[1]
	duplicate.ID = "mgmt-copy"
	request.Probes = append(request.Probes, duplicate)
	if _, err := BuildPreflightPlan(request); !errors.Is(err, ErrInvalidPreflightPlan) {
		t.Fatalf("duplicate binding error = %v", err)
	}
}

func TestValidatePreflightPlanRejectsAuthorityAndContentTampering(t *testing.T) {
	base, err := BuildPreflightPlan(validRequest())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*PreflightPlan)
	}{
		{name: "mutation authority", mutate: func(p *PreflightPlan) { p.MutationAuthorized = true }},
		{name: "apply authority", mutate: func(p *PreflightPlan) { p.ApplyAuthorized = true }},
		{name: "automatic rollback", mutate: func(p *PreflightPlan) { p.AutomaticRollbackAuthorized = true }},
		{name: "rollback execution", mutate: func(p *PreflightPlan) { p.Rollback.ExecutionAuthorized = true }},
		{name: "rollback revision", mutate: func(p *PreflightPlan) { p.Rollback.PreviousRevisionID = "rev-other" }},
		{name: "plan id", mutate: func(p *PreflightPlan) { p.PlanID = "sha256:deadbeef" }},
		{name: "noncanonical probe order", mutate: func(p *PreflightPlan) { p.Probes[0], p.Probes[1] = p.Probes[1], p.Probes[0] }},
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

func TestBuildPreflightPlanRejectsSameRevision(t *testing.T) {
	request := validRequest()
	request.PlannedRevisionID = request.BaseRevisionID
	if _, err := BuildPreflightPlan(request); !errors.Is(err, ErrInvalidPreflightPlan) {
		t.Fatalf("error = %v, want ErrInvalidPreflightPlan", err)
	}
}
