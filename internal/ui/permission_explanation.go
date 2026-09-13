package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"control-center/internal/identity/rbac"
)

const PermissionExplanationContractVersion = "ui.permission-explanation/v1"

var ErrInvalidPermissionExplanation = errors.New("invalid permission explanation")

type PermissionExplanationReason string

const (
	PermissionAllowedExact          PermissionExplanationReason = "allowed_permission"
	PermissionAllowedWildcard       PermissionExplanationReason = "allowed_wildcard"
	PermissionDeniedNoGrants        PermissionExplanationReason = "denied_no_effective_grants"
	PermissionDeniedScopeNotGranted PermissionExplanationReason = "denied_scope_not_granted"
	PermissionDeniedNotGranted      PermissionExplanationReason = "denied_permission_not_granted"
)

type PermissionExplanationInput struct {
	Permission    rbac.Permission
	Target        rbac.Scope
	Grants        []rbac.EffectiveGrant
	ServerAllowed bool
}

type PermissionExplanationView struct {
	ContractVersion    string                      `json:"contract_version"`
	Permission         rbac.Permission             `json:"permission"`
	Target             rbac.Scope                  `json:"target"`
	Allowed            bool                        `json:"allowed"`
	Reason             PermissionExplanationReason `json:"reason"`
	MatchingRoles      []string                    `json:"matching_roles"`
	MutationAuthorized bool                        `json:"mutation_authorized"`
}

// BuildPermissionExplanation converts authoritative self-only effective grants
// plus the server's authorization decision into a bounded user-facing reason.
// It validates consistency but never authorizes a request by itself.
func BuildPermissionExplanation(input PermissionExplanationInput) (PermissionExplanationView, error) {
	permission := rbac.Permission(strings.TrimSpace(string(input.Permission)))
	if permission == "" || permission != input.Permission {
		return PermissionExplanationView{}, invalidPermissionExplanation("permission must be canonical")
	}
	if !input.Target.Valid() {
		return PermissionExplanationView{}, invalidPermissionExplanation("target scope is invalid")
	}
	if len(input.Grants) > maxSecurityOverviewGrants {
		return PermissionExplanationView{}, invalidPermissionExplanation("effective grant evidence exceeds %d entries", maxSecurityOverviewGrants)
	}
	grants, _, _, err := normalizeSecurityOverviewGrants(input.Grants)
	if err != nil {
		return PermissionExplanationView{}, fmt.Errorf("%w: %v", ErrInvalidPermissionExplanation, err)
	}

	scopeMatches := 0
	exactRoles := make([]string, 0)
	wildcardRoles := make([]string, 0)
	for _, grant := range grants {
		if !permissionScopeContains(grant.Scope, input.Target) {
			continue
		}
		scopeMatches++
		for _, granted := range grant.Permissions {
			switch granted {
			case rbac.PermissionAll:
				wildcardRoles = append(wildcardRoles, grant.RoleName)
			case permission:
				exactRoles = append(exactRoles, grant.RoleName)
			}
		}
	}

	derivedAllowed := len(wildcardRoles) > 0 || len(exactRoles) > 0
	if derivedAllowed != input.ServerAllowed {
		return PermissionExplanationView{}, invalidPermissionExplanation("server decision is inconsistent with supplied effective grants")
	}

	view := PermissionExplanationView{
		ContractVersion:    PermissionExplanationContractVersion,
		Permission:         permission,
		Target:             input.Target,
		Allowed:            input.ServerAllowed,
		MatchingRoles:      []string{},
		MutationAuthorized: false,
	}
	if input.ServerAllowed {
		if len(exactRoles) > 0 {
			view.Reason = PermissionAllowedExact
			view.MatchingRoles = uniqueSortedStrings(exactRoles)
			return view, nil
		}
		view.Reason = PermissionAllowedWildcard
		view.MatchingRoles = uniqueSortedStrings(wildcardRoles)
		return view, nil
	}

	switch {
	case len(grants) == 0:
		view.Reason = PermissionDeniedNoGrants
	case scopeMatches == 0:
		view.Reason = PermissionDeniedScopeNotGranted
	default:
		view.Reason = PermissionDeniedNotGranted
	}
	return view, nil
}

func permissionScopeContains(binding, target rbac.Scope) bool {
	if binding.Kind == rbac.ScopeGlobal && binding.ID == "" {
		return true
	}
	return binding.Kind == target.Kind && binding.ID == target.ID
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func invalidPermissionExplanation(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidPermissionExplanation, fmt.Sprintf(format, values...))
}
