package ui

import (
	"errors"
	"reflect"
	"testing"

	"control-center/internal/identity/rbac"
)

func TestBuildPermissionExplanationAllowedExactAndWildcard(t *testing.T) {
	for name, tc := range map[string]struct {
		input      PermissionExplanationInput
		wantReason PermissionExplanationReason
		wantRoles  []string
	}{
		"exact": {
			input: PermissionExplanationInput{
				Permission: rbac.PermissionJobsRead,
				Target:     rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"},
				Grants: []rbac.EffectiveGrant{
					{RoleName: "operator", Scope: rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"}, Permissions: []rbac.Permission{rbac.PermissionJobsRead}},
				},
				ServerAllowed: true,
			},
			wantReason: PermissionAllowedExact,
			wantRoles:  []string{"operator"},
		},
		"wildcard": {
			input: PermissionExplanationInput{
				Permission: rbac.PermissionJobsRead,
				Target:     rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"},
				Grants: []rbac.EffectiveGrant{
					{RoleName: "administrator", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionAll}},
				},
				ServerAllowed: true,
			},
			wantReason: PermissionAllowedWildcard,
			wantRoles:  []string{"administrator"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			view, err := BuildPermissionExplanation(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if !view.Allowed || view.MutationAuthorized || view.Reason != tc.wantReason || !reflect.DeepEqual(view.MatchingRoles, tc.wantRoles) {
				t.Fatalf("unexpected explanation: %#v", view)
			}
		})
	}
}

func TestBuildPermissionExplanationDeniedReasons(t *testing.T) {
	for name, tc := range map[string]struct {
		input PermissionExplanationInput
		want  PermissionExplanationReason
	}{
		"no grants": {
			input: PermissionExplanationInput{Permission: rbac.PermissionJobsRead, Target: rbac.GlobalScope(), ServerAllowed: false},
			want:  PermissionDeniedNoGrants,
		},
		"scope mismatch": {
			input: PermissionExplanationInput{
				Permission: rbac.PermissionJobsRead,
				Target:     rbac.Scope{Kind: rbac.ScopeSite, ID: "site-b"},
				Grants: []rbac.EffectiveGrant{
					{RoleName: "operator", Scope: rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"}, Permissions: []rbac.Permission{rbac.PermissionJobsRead}},
				},
			},
			want: PermissionDeniedScopeNotGranted,
		},
		"permission missing": {
			input: PermissionExplanationInput{
				Permission: rbac.PermissionJobsRead,
				Target:     rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"},
				Grants: []rbac.EffectiveGrant{
					{RoleName: "viewer", Scope: rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"}, Permissions: []rbac.Permission{rbac.PermissionOverviewRead}},
				},
			},
			want: PermissionDeniedNotGranted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			view, err := BuildPermissionExplanation(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if view.Allowed || view.MutationAuthorized || view.Reason != tc.want || len(view.MatchingRoles) != 0 {
				t.Fatalf("unexpected explanation: %#v", view)
			}
		})
	}
}

func TestBuildPermissionExplanationFailsClosedOnDecisionDrift(t *testing.T) {
	input := PermissionExplanationInput{
		Permission: rbac.PermissionJobsRead,
		Target:     rbac.GlobalScope(),
		Grants: []rbac.EffectiveGrant{
			{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionOverviewRead}},
		},
		ServerAllowed: true,
	}
	_, err := BuildPermissionExplanation(input)
	if !errors.Is(err, ErrInvalidPermissionExplanation) {
		t.Fatalf("err=%v, want ErrInvalidPermissionExplanation", err)
	}
}

func TestBuildPermissionExplanationRejectsInvalidTargetOrGrantEvidence(t *testing.T) {
	for name, input := range map[string]PermissionExplanationInput{
		"invalid target": {
			Permission: rbac.PermissionJobsRead,
			Target:     rbac.Scope{Kind: rbac.ScopeSite},
		},
		"duplicate grant": {
			Permission: rbac.PermissionJobsRead,
			Target:     rbac.GlobalScope(),
			Grants: []rbac.EffectiveGrant{
				{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionOverviewRead}},
				{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionJobsRead}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := BuildPermissionExplanation(input)
			if !errors.Is(err, ErrInvalidPermissionExplanation) {
				t.Fatalf("err=%v, want ErrInvalidPermissionExplanation", err)
			}
		})
	}
}
