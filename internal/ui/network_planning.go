package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"control-center/internal/networkpolicy"
)

const NetworkPlanningSchemaV1 = "ui.network-planning/v1"

var ErrInvalidNetworkPlanning = errors.New("invalid network planning view")

const (
	maxNetworkPlanningInterfaces = 64
	maxNetworkPlanningText       = 256
)

// CurrentNetworkInterface is bounded read-only inventory evidence consumed by
// the 0.34 planning projection. It deliberately carries no gateway, route,
// firewall, credential, backend payload, or mutation authority.
type CurrentNetworkInterface struct {
	InterfaceID      string `json:"interface_id"`
	Name             string `json:"name"`
	CurrentZone      string `json:"current_zone"`
	OperationalState string `json:"operational_state"`
}

// NetworkPlanningInterface presents the exact current-vs-target zone binding
// for one interface included in the canonical network ChangePlan.
type NetworkPlanningInterface struct {
	InterfaceID      string             `json:"interface_id"`
	Name             string             `json:"name"`
	CurrentZone      string             `json:"current_zone"`
	TargetZone       networkpolicy.Zone `json:"target_zone"`
	OperationalState string             `json:"operational_state"`
	Changed          bool               `json:"changed"`
}

type NetworkPlanningProbe struct {
	ID          string                  `json:"id"`
	Kind        networkpolicy.ProbeKind `json:"kind"`
	InterfaceID string                  `json:"interface_id"`
	Zone        networkpolicy.Zone      `json:"zone"`
}

type NetworkPlanningStep struct {
	Order  int                        `json:"order"`
	Stage  networkpolicy.ChangeStage  `json:"stage"`
	Action networkpolicy.ChangeAction `json:"action"`
}

type NetworkPlanningForwarding struct {
	Source              networkpolicy.Zone `json:"source"`
	Destination         networkpolicy.Zone `json:"destination"`
	ExplicitlyEnabled   bool               `json:"explicitly_enabled"`
	EdgeGatewayAssigned bool               `json:"edge_gateway_assigned"`
}

// NetworkPlanningRollback is operator-visible recovery evidence. 0.34 is
// planning-only, so the contract requires rollback intent but never executes
// it or grants authority to do so.
type NetworkPlanningRollback struct {
	SnapshotRequired            bool  `json:"snapshot_required"`
	AutomaticOnProbeFailure     bool  `json:"automatic_on_probe_failure"`
	AutomaticOnTimeout          bool  `json:"automatic_on_timeout"`
	RollbackTimeoutMilliseconds int64 `json:"rollback_timeout_ms"`
}

// SupportGatewayChannels makes the two-channel Support Gateway invariant
// explicit when SUPPORT_LAN/SUPPORT_WAN are present in a plan. Transit remains
// hard-false in this milestone.
type SupportGatewayChannels struct {
	Required          bool   `json:"required"`
	LANInterfaceID    string `json:"lan_interface_id,omitempty"`
	WANInterfaceID    string `json:"wan_interface_id,omitempty"`
	Separated         bool   `json:"separated"`
	TransitAuthorized bool   `json:"transit_authorized"`
}

// NetworkPlanningView is a side-effect-free operator projection over one exact
// content-addressed network ChangePlan and a bounded current-interface snapshot.
// MutationAuthorized is intentionally hard-false for the 0.34 planning milestone.
type NetworkPlanningView struct {
	Schema                  string                      `json:"schema"`
	PlanID                  string                      `json:"plan_id"`
	NodeID                  string                      `json:"node_id"`
	RevisionID              string                      `json:"revision_id"`
	Interfaces              []NetworkPlanningInterface  `json:"interfaces"`
	Forwarding              []NetworkPlanningForwarding `json:"forwarding"`
	ForwardingDefaultDeny   bool                        `json:"forwarding_default_deny"`
	Probes                  []NetworkPlanningProbe      `json:"probes"`
	Steps                   []NetworkPlanningStep       `json:"steps"`
	Rollback                NetworkPlanningRollback     `json:"rollback"`
	SupportGatewayChannels  SupportGatewayChannels      `json:"support_gateway_channels"`
	MutationAuthorized      bool                        `json:"mutation_authorized"`
	ExternalPublishAllowed  bool                        `json:"external_publication_authorized"`
}

type NetworkPlanningRequest struct {
	Plan              networkpolicy.ChangePlan
	CurrentInterfaces []CurrentNetworkInterface
	Rollback          NetworkPlanningRollback
}

// BuildNetworkPlanningView revalidates the exact canonical plan, binds it to
// current interface evidence and returns a deterministic read-only projection.
// Unknown/stale/tampered input fails closed instead of being rendered as a
// healthy or executable network change.
func BuildNetworkPlanningView(request NetworkPlanningRequest) (NetworkPlanningView, error) {
	canonical, err := networkpolicy.BuildChangePlan(networkpolicy.ChangePlanRequest{
		NodeID:     request.Plan.NodeID,
		RevisionID: request.Plan.RevisionID,
		Interfaces: append([]networkpolicy.InterfaceIntent(nil), request.Plan.Interfaces...),
		Forwarding: append([]networkpolicy.ForwardingIntent(nil), request.Plan.Forwarding...),
		Probes:     append([]networkpolicy.ConnectivityProbe(nil), request.Plan.Probes...),
		Timeouts:   request.Plan.Timeouts,
	})
	if err != nil {
		return NetworkPlanningView{}, fmt.Errorf("%w: canonical plan: %v", ErrInvalidNetworkPlanning, err)
	}
	if !reflect.DeepEqual(canonical, request.Plan) {
		return NetworkPlanningView{}, fmt.Errorf("%w: supplied plan does not match canonical content", ErrInvalidNetworkPlanning)
	}
	if request.Plan.PlanID == "" {
		return NetworkPlanningView{}, fmt.Errorf("%w: plan_id is required", ErrInvalidNetworkPlanning)
	}
	if err := validateRollback(request.Rollback, request.Plan); err != nil {
		return NetworkPlanningView{}, err
	}

	currentByID, err := indexCurrentInterfaces(request.CurrentInterfaces)
	if err != nil {
		return NetworkPlanningView{}, err
	}
	interfaces := make([]NetworkPlanningInterface, 0, len(request.Plan.Interfaces))
	for _, target := range request.Plan.Interfaces {
		current, ok := currentByID[target.InterfaceID]
		if !ok {
			return NetworkPlanningView{}, fmt.Errorf("%w: current interface %q is missing", ErrInvalidNetworkPlanning, target.InterfaceID)
		}
		interfaces = append(interfaces, NetworkPlanningInterface{
			InterfaceID:      target.InterfaceID,
			Name:             current.Name,
			CurrentZone:      current.CurrentZone,
			TargetZone:       target.Zone,
			OperationalState: current.OperationalState,
			Changed:          target.Changed,
		})
	}

	supportChannels, err := buildSupportGatewayChannels(request.Plan.Interfaces)
	if err != nil {
		return NetworkPlanningView{}, err
	}

	forwarding := make([]NetworkPlanningForwarding, 0, len(request.Plan.Forwarding))
	for _, intent := range request.Plan.Forwarding {
		forwarding = append(forwarding, NetworkPlanningForwarding{
			Source:              intent.Source,
			Destination:         intent.Destination,
			ExplicitlyEnabled:   intent.ExplicitlyEnabled,
			EdgeGatewayAssigned: intent.EdgeGatewayAssigned,
		})
	}
	probes := make([]NetworkPlanningProbe, 0, len(request.Plan.Probes))
	for _, probe := range request.Plan.Probes {
		probes = append(probes, NetworkPlanningProbe{
			ID:          probe.ID,
			Kind:        probe.Kind,
			InterfaceID: probe.InterfaceID,
			Zone:        probe.Zone,
		})
	}
	steps := make([]NetworkPlanningStep, 0, len(request.Plan.Steps))
	for _, step := range request.Plan.Steps {
		steps = append(steps, NetworkPlanningStep{Order: step.Order, Stage: step.Stage, Action: step.Action})
	}

	return NetworkPlanningView{
		Schema:                 NetworkPlanningSchemaV1,
		PlanID:                 request.Plan.PlanID,
		NodeID:                 request.Plan.NodeID,
		RevisionID:             request.Plan.RevisionID,
		Interfaces:             interfaces,
		Forwarding:             forwarding,
		ForwardingDefaultDeny:  request.Plan.ForwardingDefaultDeny,
		Probes:                 probes,
		Steps:                  steps,
		Rollback:               request.Rollback,
		SupportGatewayChannels: supportChannels,
		MutationAuthorized:     false,
		ExternalPublishAllowed: false,
	}, nil
}

func validateRollback(rollback NetworkPlanningRollback, plan networkpolicy.ChangePlan) error {
	if !rollback.SnapshotRequired || !rollback.AutomaticOnProbeFailure || !rollback.AutomaticOnTimeout {
		return fmt.Errorf("%w: rollback strategy must require snapshot and automatic rollback on probe failure/timeout", ErrInvalidNetworkPlanning)
	}
	expected := plan.Timeouts.Rollback.Milliseconds()
	if expected <= 0 || rollback.RollbackTimeoutMilliseconds != expected {
		return fmt.Errorf("%w: rollback timeout must match the exact canonical plan", ErrInvalidNetworkPlanning)
	}
	return nil
}

func indexCurrentInterfaces(input []CurrentNetworkInterface) (map[string]CurrentNetworkInterface, error) {
	if len(input) == 0 || len(input) > maxNetworkPlanningInterfaces {
		return nil, fmt.Errorf("%w: current interface snapshot must contain 1..%d entries", ErrInvalidNetworkPlanning, maxNetworkPlanningInterfaces)
	}
	result := make(map[string]CurrentNetworkInterface, len(input))
	for _, item := range input {
		item.InterfaceID = strings.TrimSpace(item.InterfaceID)
		item.Name = strings.TrimSpace(item.Name)
		item.CurrentZone = strings.ToUpper(strings.TrimSpace(item.CurrentZone))
		item.OperationalState = strings.ToLower(strings.TrimSpace(item.OperationalState))
		if !boundedPlanningText(item.InterfaceID, 128) || !boundedPlanningText(item.Name, maxNetworkPlanningText) {
			return nil, fmt.Errorf("%w: current interface identity/name is invalid", ErrInvalidNetworkPlanning)
		}
		if !validCurrentZone(item.CurrentZone) {
			return nil, fmt.Errorf("%w: current interface %q has unsupported zone %q", ErrInvalidNetworkPlanning, item.InterfaceID, item.CurrentZone)
		}
		switch item.OperationalState {
		case "up", "down", "unknown":
		default:
			return nil, fmt.Errorf("%w: current interface %q has invalid operational state %q", ErrInvalidNetworkPlanning, item.InterfaceID, item.OperationalState)
		}
		if _, duplicate := result[item.InterfaceID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate current interface %q", ErrInvalidNetworkPlanning, item.InterfaceID)
		}
		result[item.InterfaceID] = item
	}
	return result, nil
}

func validCurrentZone(zone string) bool {
	switch zone {
	case "UNASSIGNED", "TRUSTED":
		return true
	default:
		return networkpolicy.Zone(zone).Valid()
	}
}

func buildSupportGatewayChannels(interfaces []networkpolicy.InterfaceIntent) (SupportGatewayChannels, error) {
	lan := make([]string, 0, 1)
	wan := make([]string, 0, 1)
	for _, item := range interfaces {
		switch item.Zone {
		case networkpolicy.ZoneSupportLAN:
			lan = append(lan, item.InterfaceID)
		case networkpolicy.ZoneSupportWAN:
			wan = append(wan, item.InterfaceID)
		}
	}
	if len(lan) == 0 && len(wan) == 0 {
		return SupportGatewayChannels{Required: false, Separated: false, TransitAuthorized: false}, nil
	}
	if len(lan) != 1 || len(wan) != 1 || lan[0] == wan[0] {
		return SupportGatewayChannels{}, fmt.Errorf("%w: support gateway planning requires exactly one distinct SUPPORT_LAN and SUPPORT_WAN interface", ErrInvalidNetworkPlanning)
	}
	return SupportGatewayChannels{
		Required:          true,
		LANInterfaceID:    lan[0],
		WANInterfaceID:    wan[0],
		Separated:         true,
		TransitAuthorized: false,
	}, nil
}

func boundedPlanningText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
