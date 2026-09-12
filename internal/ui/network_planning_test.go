package ui

import (
	"errors"
	"testing"
	"time"

	"control-center/internal/networkpolicy"
)

func TestBuildNetworkPlanningViewIsReadOnlyAndExactBound(t *testing.T) {
	plan := mustNetworkPlanningPlan(t, []networkpolicy.InterfaceIntent{
		{InterfaceID: "lan-01", Zone: networkpolicy.ZoneLAN, Changed: true},
		{InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
	}, []networkpolicy.ConnectivityProbe{
		{ID: "probe-lan", Kind: networkpolicy.ProbeZoneReachability, InterfaceID: "lan-01", Zone: networkpolicy.ZoneLAN},
		{ID: "probe-mgmt", Kind: networkpolicy.ProbeControlPlane, InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
	})

	view, err := BuildNetworkPlanningView(NetworkPlanningRequest{
		Plan: plan,
		CurrentInterfaces: []CurrentNetworkInterface{
			{InterfaceID: "lan-01", Name: "eth1", CurrentZone: "UNASSIGNED", OperationalState: "up"},
			{InterfaceID: "mgmt-01", Name: "eth0", CurrentZone: "MANAGEMENT", OperationalState: "up"},
		},
		Rollback: rollbackFor(plan),
	})
	if err != nil {
		t.Fatalf("BuildNetworkPlanningView() error = %v", err)
	}
	if view.Schema != NetworkPlanningSchemaV1 || view.PlanID != plan.PlanID || view.NodeID != plan.NodeID || view.RevisionID != plan.RevisionID {
		t.Fatalf("unexpected identity: %#v", view)
	}
	if view.MutationAuthorized || view.ExternalPublishAllowed {
		t.Fatalf("planning projection granted authority: %#v", view)
	}
	if !view.ForwardingDefaultDeny {
		t.Fatal("forwarding default deny was lost")
	}
	if view.SupportGatewayChannels.Required || view.SupportGatewayChannels.TransitAuthorized {
		t.Fatalf("ordinary plan unexpectedly became support-gateway plan: %#v", view.SupportGatewayChannels)
	}
	if len(view.Interfaces) != 2 || view.Interfaces[0].InterfaceID != "lan-01" || view.Interfaces[1].InterfaceID != "mgmt-01" {
		t.Fatalf("interfaces are not canonical: %#v", view.Interfaces)
	}
	if view.Interfaces[0].CurrentZone != "UNASSIGNED" || view.Interfaces[0].TargetZone != networkpolicy.ZoneLAN || !view.Interfaces[0].Changed {
		t.Fatalf("current/target projection mismatch: %#v", view.Interfaces[0])
	}
}

func TestBuildNetworkPlanningViewRequiresSeparatedSupportGatewayChannels(t *testing.T) {
	plan := mustNetworkPlanningPlan(t, []networkpolicy.InterfaceIntent{
		{InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
		{InterfaceID: "support-lan-01", Zone: networkpolicy.ZoneSupportLAN, Changed: true},
		{InterfaceID: "support-wan-01", Zone: networkpolicy.ZoneSupportWAN, Changed: true},
	}, []networkpolicy.ConnectivityProbe{
		{ID: "probe-mgmt", Kind: networkpolicy.ProbeControlPlane, InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
		{ID: "probe-support-lan", Kind: networkpolicy.ProbeLinkState, InterfaceID: "support-lan-01", Zone: networkpolicy.ZoneSupportLAN},
		{ID: "probe-support-wan", Kind: networkpolicy.ProbeLinkState, InterfaceID: "support-wan-01", Zone: networkpolicy.ZoneSupportWAN},
	})

	view, err := BuildNetworkPlanningView(NetworkPlanningRequest{
		Plan: plan,
		CurrentInterfaces: []CurrentNetworkInterface{
			{InterfaceID: "mgmt-01", Name: "eth0", CurrentZone: "MANAGEMENT", OperationalState: "up"},
			{InterfaceID: "support-lan-01", Name: "eth1", CurrentZone: "UNASSIGNED", OperationalState: "up"},
			{InterfaceID: "support-wan-01", Name: "eth2", CurrentZone: "UNASSIGNED", OperationalState: "up"},
		},
		Rollback: rollbackFor(plan),
	})
	if err != nil {
		t.Fatalf("BuildNetworkPlanningView() error = %v", err)
	}
	channels := view.SupportGatewayChannels
	if !channels.Required || !channels.Separated || channels.TransitAuthorized {
		t.Fatalf("unexpected support channel state: %#v", channels)
	}
	if channels.LANInterfaceID != "support-lan-01" || channels.WANInterfaceID != "support-wan-01" {
		t.Fatalf("unexpected support channel identities: %#v", channels)
	}
}

func TestBuildNetworkPlanningViewRejectsIncompleteSupportGatewayChannels(t *testing.T) {
	plan := mustNetworkPlanningPlan(t, []networkpolicy.InterfaceIntent{
		{InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
		{InterfaceID: "support-lan-01", Zone: networkpolicy.ZoneSupportLAN, Changed: true},
	}, []networkpolicy.ConnectivityProbe{
		{ID: "probe-mgmt", Kind: networkpolicy.ProbeControlPlane, InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
		{ID: "probe-support-lan", Kind: networkpolicy.ProbeLinkState, InterfaceID: "support-lan-01", Zone: networkpolicy.ZoneSupportLAN},
	})

	_, err := BuildNetworkPlanningView(NetworkPlanningRequest{
		Plan: plan,
		CurrentInterfaces: []CurrentNetworkInterface{
			{InterfaceID: "mgmt-01", Name: "eth0", CurrentZone: "MANAGEMENT", OperationalState: "up"},
			{InterfaceID: "support-lan-01", Name: "eth1", CurrentZone: "UNASSIGNED", OperationalState: "up"},
		},
		Rollback: rollbackFor(plan),
	})
	if !errors.Is(err, ErrInvalidNetworkPlanning) {
		t.Fatalf("BuildNetworkPlanningView() error = %v, want ErrInvalidNetworkPlanning", err)
	}
}

func TestBuildNetworkPlanningViewRejectsTamperedCanonicalPlan(t *testing.T) {
	plan := mustNetworkPlanningPlan(t, []networkpolicy.InterfaceIntent{
		{InterfaceID: "lan-01", Zone: networkpolicy.ZoneLAN, Changed: true},
		{InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
	}, []networkpolicy.ConnectivityProbe{
		{ID: "probe-lan", Kind: networkpolicy.ProbeZoneReachability, InterfaceID: "lan-01", Zone: networkpolicy.ZoneLAN},
		{ID: "probe-mgmt", Kind: networkpolicy.ProbeControlPlane, InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
	})
	plan.Steps[0].Action = networkpolicy.ActionCommitOrRollback

	_, err := BuildNetworkPlanningView(NetworkPlanningRequest{
		Plan: plan,
		CurrentInterfaces: []CurrentNetworkInterface{
			{InterfaceID: "lan-01", Name: "eth1", CurrentZone: "LAN", OperationalState: "up"},
			{InterfaceID: "mgmt-01", Name: "eth0", CurrentZone: "MANAGEMENT", OperationalState: "up"},
		},
		Rollback: rollbackFor(plan),
	})
	if !errors.Is(err, ErrInvalidNetworkPlanning) {
		t.Fatalf("BuildNetworkPlanningView() error = %v, want ErrInvalidNetworkPlanning", err)
	}
}

func TestBuildNetworkPlanningViewRejectsWeakRollbackContract(t *testing.T) {
	plan := mustNetworkPlanningPlan(t, []networkpolicy.InterfaceIntent{
		{InterfaceID: "lan-01", Zone: networkpolicy.ZoneLAN, Changed: true},
		{InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
	}, []networkpolicy.ConnectivityProbe{
		{ID: "probe-lan", Kind: networkpolicy.ProbeZoneReachability, InterfaceID: "lan-01", Zone: networkpolicy.ZoneLAN},
		{ID: "probe-mgmt", Kind: networkpolicy.ProbeControlPlane, InterfaceID: "mgmt-01", Zone: networkpolicy.ZoneManagement},
	})

	rollback := rollbackFor(plan)
	rollback.AutomaticOnProbeFailure = false
	_, err := BuildNetworkPlanningView(NetworkPlanningRequest{
		Plan: plan,
		CurrentInterfaces: []CurrentNetworkInterface{
			{InterfaceID: "lan-01", Name: "eth1", CurrentZone: "LAN", OperationalState: "up"},
			{InterfaceID: "mgmt-01", Name: "eth0", CurrentZone: "MANAGEMENT", OperationalState: "up"},
		},
		Rollback: rollback,
	})
	if !errors.Is(err, ErrInvalidNetworkPlanning) {
		t.Fatalf("BuildNetworkPlanningView() error = %v, want ErrInvalidNetworkPlanning", err)
	}
}

func mustNetworkPlanningPlan(t *testing.T, interfaces []networkpolicy.InterfaceIntent, probes []networkpolicy.ConnectivityProbe) networkpolicy.ChangePlan {
	t.Helper()
	plan, err := networkpolicy.BuildChangePlan(networkpolicy.ChangePlanRequest{
		NodeID:     "node-a",
		RevisionID: "revision-034-a",
		Interfaces: interfaces,
		Probes:     probes,
		Timeouts: networkpolicy.TimeoutPolicy{
			Snapshot:    time.Minute,
			Preflight:   time.Minute,
			ApplyWindow: 5 * time.Minute,
			Probe:       time.Minute,
			Rollback:    2 * time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("BuildChangePlan() error = %v", err)
	}
	return plan
}

func rollbackFor(plan networkpolicy.ChangePlan) NetworkPlanningRollback {
	return NetworkPlanningRollback{
		SnapshotRequired:            true,
		AutomaticOnProbeFailure:     true,
		AutomaticOnTimeout:          true,
		RollbackTimeoutMilliseconds: plan.Timeouts.Rollback.Milliseconds(),
	}
}
