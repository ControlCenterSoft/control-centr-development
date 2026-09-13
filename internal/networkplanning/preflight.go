// Package networkplanning defines plan-only contracts for managed network
// changes. The contracts in this file describe connectivity verification and
// recovery intent only; they never grant mutation, routing, NAT, publication,
// remote-support or rollback execution authority.
package networkplanning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const PreflightSchemaVersion = "network.managed-planning-preflight/v1"

const (
	maxPreflightProbes = 32
	maxIdentifierLen   = 128
	maxTargetLen       = 253
)

var (
	ErrInvalidPreflightPlan = errors.New("invalid managed network preflight plan")
	preflightIdentifierRE   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,126}[A-Za-z0-9])?$`)
	hostLabelRE              = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

type ProbeKind string

const (
	ProbeManagementReachability ProbeKind = "management_reachability"
	ProbeGatewayReachability    ProbeKind = "gateway_reachability"
	ProbeDNSResolution          ProbeKind = "dns_resolution"
	ProbeNTPReachability        ProbeKind = "ntp_reachability"
)

func (k ProbeKind) valid() bool {
	switch k {
	case ProbeManagementReachability, ProbeGatewayReachability, ProbeDNSResolution, ProbeNTPReachability:
		return true
	default:
		return false
	}
}

// ConnectivityProbe describes one side-effect-free connectivity check that
// must be executed by a separately authorized verifier after a future network
// apply. It contains no credentials and no command text.
type ConnectivityProbe struct {
	ID                string    `json:"id"`
	Kind              ProbeKind `json:"kind"`
	SourceInterfaceID string    `json:"source_interface_id"`
	Target            string    `json:"target"`
}

// RollbackStrategy is an operator-visible recovery projection. It identifies
// the exact previous network revision that must be restorable, but does not
// authorize either an apply or a rollback.
type RollbackStrategy struct {
	Mode                             string `json:"mode"`
	PreviousRevisionID               string `json:"previous_revision_id"`
	OperatorConfirmationRequired     bool   `json:"operator_confirmation_required"`
	PostRollbackVerificationRequired bool   `json:"post_rollback_verification_required"`
	ExecutionAuthorized              bool   `json:"execution_authorized"`
}

// PreflightRequest is the plan-only input. BuildPreflightPlan validates and
// canonicalizes it before producing a content-addressed immutable plan.
type PreflightRequest struct {
	NodeID            string              `json:"node_id"`
	BaseRevisionID    string              `json:"base_revision_id"`
	PlannedRevisionID string              `json:"planned_revision_id"`
	Probes            []ConnectivityProbe `json:"probes"`
}

// PreflightPlan is safe to persist or transport as review evidence. All
// authority flags are intentionally hard-false in the 0.34 planning boundary.
type PreflightPlan struct {
	Schema                           string              `json:"schema"`
	PlanID                           string              `json:"plan_id"`
	NodeID                           string              `json:"node_id"`
	BaseRevisionID                   string              `json:"base_revision_id"`
	PlannedRevisionID                string              `json:"planned_revision_id"`
	Probes                           []ConnectivityProbe `json:"probes"`
	Rollback                         RollbackStrategy    `json:"rollback"`
	ConnectivityVerificationRequired bool                `json:"connectivity_verification_required"`
	MutationAuthorized               bool                `json:"mutation_authorized"`
	ApplyAuthorized                  bool                `json:"apply_authorized"`
	AutomaticRollbackAuthorized      bool                `json:"automatic_rollback_authorized"`
	ForwardingAuthorized             bool                `json:"forwarding_authorized"`
	NATAuthorized                    bool                `json:"nat_authorized"`
	ExternalPublicationAuthorized    bool                `json:"external_publication_authorized"`
	SupportRemoteAccessAuthorized    bool                `json:"support_remote_access_authorized"`
}

const rollbackModeRestorePrevious = "restore_previous_network_state"

func BuildPreflightPlan(request PreflightRequest) (PreflightPlan, error) {
	if err := validateIdentifier("node_id", request.NodeID); err != nil {
		return PreflightPlan{}, err
	}
	if err := validateIdentifier("base_revision_id", request.BaseRevisionID); err != nil {
		return PreflightPlan{}, err
	}
	if err := validateIdentifier("planned_revision_id", request.PlannedRevisionID); err != nil {
		return PreflightPlan{}, err
	}
	if request.BaseRevisionID == request.PlannedRevisionID {
		return PreflightPlan{}, invalidPreflight("planned_revision_id must differ from base_revision_id")
	}

	probes, err := canonicalizeProbes(request.Probes)
	if err != nil {
		return PreflightPlan{}, err
	}

	plan := PreflightPlan{
		Schema:            PreflightSchemaVersion,
		NodeID:            request.NodeID,
		BaseRevisionID:    request.BaseRevisionID,
		PlannedRevisionID: request.PlannedRevisionID,
		Probes:            probes,
		Rollback: RollbackStrategy{
			Mode:                             rollbackModeRestorePrevious,
			PreviousRevisionID:               request.BaseRevisionID,
			OperatorConfirmationRequired:     true,
			PostRollbackVerificationRequired: true,
			ExecutionAuthorized:              false,
		},
		ConnectivityVerificationRequired: true,
		MutationAuthorized:                false,
		ApplyAuthorized:                   false,
		AutomaticRollbackAuthorized:       false,
		ForwardingAuthorized:              false,
		NATAuthorized:                     false,
		ExternalPublicationAuthorized:     false,
		SupportRemoteAccessAuthorized:     false,
	}

	plan.PlanID, err = computePlanID(plan)
	if err != nil {
		return PreflightPlan{}, err
	}
	return plan, nil
}

// ValidatePreflightPlan revalidates an immutable plan after a storage or
// transport boundary. It rejects semantic tampering instead of normalizing it.
func ValidatePreflightPlan(plan PreflightPlan) error {
	if plan.Schema != PreflightSchemaVersion {
		return invalidPreflight("unsupported schema")
	}
	if err := validateIdentifier("node_id", plan.NodeID); err != nil {
		return err
	}
	if err := validateIdentifier("base_revision_id", plan.BaseRevisionID); err != nil {
		return err
	}
	if err := validateIdentifier("planned_revision_id", plan.PlannedRevisionID); err != nil {
		return err
	}
	if plan.BaseRevisionID == plan.PlannedRevisionID {
		return invalidPreflight("planned_revision_id must differ from base_revision_id")
	}

	canonical, err := canonicalizeProbes(plan.Probes)
	if err != nil {
		return err
	}
	if !equalProbes(plan.Probes, canonical) {
		return invalidPreflight("probes are not in canonical order")
	}

	if plan.Rollback.Mode != rollbackModeRestorePrevious {
		return invalidPreflight("rollback mode is not supported")
	}
	if plan.Rollback.PreviousRevisionID != plan.BaseRevisionID {
		return invalidPreflight("rollback previous revision does not match base revision")
	}
	if !plan.Rollback.OperatorConfirmationRequired {
		return invalidPreflight("rollback operator confirmation must be required")
	}
	if !plan.Rollback.PostRollbackVerificationRequired {
		return invalidPreflight("post-rollback verification must be required")
	}
	if plan.Rollback.ExecutionAuthorized {
		return invalidPreflight("rollback execution authority is forbidden")
	}
	if !plan.ConnectivityVerificationRequired {
		return invalidPreflight("connectivity verification must be required")
	}
	if plan.MutationAuthorized || plan.ApplyAuthorized || plan.AutomaticRollbackAuthorized ||
		plan.ForwardingAuthorized || plan.NATAuthorized || plan.ExternalPublicationAuthorized ||
		plan.SupportRemoteAccessAuthorized {
		return invalidPreflight("planning evidence cannot grant runtime authority")
	}

	expected, err := computePlanID(plan)
	if err != nil {
		return err
	}
	if plan.PlanID != expected {
		return invalidPreflight("plan_id does not match canonical content")
	}
	return nil
}

func canonicalizeProbes(input []ConnectivityProbe) ([]ConnectivityProbe, error) {
	if len(input) == 0 || len(input) > maxPreflightProbes {
		return nil, invalidPreflight("probe count is outside the allowed range")
	}
	probes := make([]ConnectivityProbe, len(input))
	seenIDs := make(map[string]struct{}, len(input))
	seenBindings := make(map[string]struct{}, len(input))
	managementProbe := false

	for i, probe := range input {
		if err := validateIdentifier("probe id", probe.ID); err != nil {
			return nil, err
		}
		if _, exists := seenIDs[probe.ID]; exists {
			return nil, invalidPreflight("duplicate probe id")
		}
		seenIDs[probe.ID] = struct{}{}
		if !probe.Kind.valid() {
			return nil, invalidPreflight("unsupported probe kind")
		}
		if err := validateIdentifier("source_interface_id", probe.SourceInterfaceID); err != nil {
			return nil, err
		}
		target, err := canonicalTarget(probe.Kind, probe.Target)
		if err != nil {
			return nil, err
		}
		probe.Target = target
		if probe.Kind == ProbeManagementReachability {
			managementProbe = true
		}
		binding := string(probe.Kind) + "\x00" + probe.SourceInterfaceID + "\x00" + probe.Target
		if _, exists := seenBindings[binding]; exists {
			return nil, invalidPreflight("duplicate probe binding")
		}
		seenBindings[binding] = struct{}{}
		probes[i] = probe
	}
	if !managementProbe {
		return nil, invalidPreflight("management reachability probe is required")
	}

	sort.Slice(probes, func(i, j int) bool {
		if probes[i].Kind != probes[j].Kind {
			return probes[i].Kind < probes[j].Kind
		}
		if probes[i].SourceInterfaceID != probes[j].SourceInterfaceID {
			return probes[i].SourceInterfaceID < probes[j].SourceInterfaceID
		}
		if probes[i].Target != probes[j].Target {
			return probes[i].Target < probes[j].Target
		}
		return probes[i].ID < probes[j].ID
	})
	return probes, nil
}

func canonicalTarget(kind ProbeKind, raw string) (string, error) {
	if raw == "" || len(raw) > maxTargetLen || raw != strings.TrimSpace(raw) {
		return "", invalidPreflight("probe target is invalid")
	}
	if strings.Contains(raw, "://") || strings.ContainsAny(raw, `/\\?#@`) {
		return "", invalidPreflight("probe target must be a host or IP, not a URL/path")
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", invalidPreflight("probe target contains whitespace/control characters")
		}
	}
	if addr, err := netip.ParseAddr(raw); err == nil {
		if kind == ProbeDNSResolution {
			return "", invalidPreflight("dns_resolution target must be a hostname")
		}
		return addr.String(), nil
	}
	if kind == ProbeGatewayReachability {
		return "", invalidPreflight("gateway_reachability target must be an IP address")
	}
	host := strings.ToLower(raw)
	if len(host) > maxTargetLen || strings.HasSuffix(host, ".") {
		return "", invalidPreflight("probe hostname is not canonical")
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", invalidPreflight("probe hostname must be fully qualified")
	}
	for _, label := range labels {
		if !hostLabelRE.MatchString(label) {
			return "", invalidPreflight("probe hostname is invalid")
		}
	}
	return host, nil
}

func validateIdentifier(field, value string) error {
	if value == "" || len(value) > maxIdentifierLen || !preflightIdentifierRE.MatchString(value) {
		return invalidPreflight(field + " is invalid")
	}
	return nil
}

func equalProbes(a, b []ConnectivityProbe) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func computePlanID(plan PreflightPlan) (string, error) {
	copy := plan
	copy.PlanID = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		return "", fmt.Errorf("%w: canonical JSON: %v", ErrInvalidPreflightPlan, err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func invalidPreflight(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidPreflightPlan, message)
}
