package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
)

func TestBuildSecurityOverviewIsSelfOnlyDeterministicAndNonAuthorizing(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	input := securityOverviewFixture(now)
	input.Sessions = []auth.SessionSecurityView{input.Sessions[1], input.Sessions[0]}
	input.Grants = []rbac.EffectiveGrant{
		{RoleName: "site-viewer", Scope: rbac.Scope{Kind: rbac.ScopeSite, ID: "site-b"}, Permissions: []rbac.Permission{rbac.PermissionJobsRead, rbac.PermissionOverviewRead}},
		{RoleName: "administrator", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionAll}},
	}

	view, err := BuildSecurityOverview(input)
	if err != nil {
		t.Fatal(err)
	}
	if view.ContractVersion != SecurityOverviewContractVersion || !view.SelfOnly || view.MutationAuthorized {
		t.Fatalf("unexpected contract boundary: %#v", view)
	}
	if view.Identity.ID != "user-1" || view.CurrentSessionID != "session-current" {
		t.Fatalf("unexpected subject/session identity: %#v", view)
	}
	if view.PasswordLifecycle.State != PasswordLifecycleActive || view.PasswordLifecycle.ChangeRequired {
		t.Fatalf("unexpected password lifecycle state: %#v", view.PasswordLifecycle)
	}
	if !view.PasswordLifecycle.ChangedAt.Equal(input.PasswordChangedAt.UTC()) {
		t.Fatalf("password lifecycle timestamp changed: got=%s want=%s", view.PasswordLifecycle.ChangedAt, input.PasswordChangedAt.UTC())
	}
	if view.SessionCount != 2 || len(view.Sessions) != 2 || !view.Sessions[0].Current || view.Sessions[0].ID != "session-current" {
		t.Fatalf("sessions are not normalized deterministically: %#v", view.Sessions)
	}
	if view.GrantCount != 2 || len(view.Grants) != 2 || view.Grants[0].RoleName != "administrator" || view.Grants[1].RoleName != "site-viewer" {
		t.Fatalf("grants are not normalized deterministically: %#v", view.Grants)
	}
	if !reflect.DeepEqual(view.Grants[1].Permissions, []rbac.Permission{rbac.PermissionJobsRead, rbac.PermissionOverviewRead}) {
		t.Fatalf("permissions are not deterministic: %#v", view.Grants[1].Permissions)
	}
	if view.PermissionCount != 3 || !view.HasWildcardPermission {
		t.Fatalf("unexpected effective permission summary: count=%d wildcard=%v", view.PermissionCount, view.HasWildcardPermission)
	}
	if len(view.Attention) != 0 || view.FirstLogin != FirstLoginCompleteOrNotRequired || view.PasswordChangeRequired {
		t.Fatalf("unexpected attention/credential state: %#v", view)
	}
}

func TestBuildSecurityOverviewProjectsPasswordChangeRequirementWithoutGrantingAuthority(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	input := securityOverviewFixture(now)
	input.PasswordChangeRequired = true
	input.Grants = nil

	view, err := BuildSecurityOverview(input)
	if err != nil {
		t.Fatal(err)
	}
	if view.FirstLogin != FirstLoginChangeRequired || !view.PasswordChangeRequired {
		t.Fatalf("password-change requirement lost: %#v", view)
	}
	if view.PasswordLifecycle.State != PasswordLifecycleChangeRequired || !view.PasswordLifecycle.ChangeRequired {
		t.Fatalf("password lifecycle did not preserve the required-change state: %#v", view.PasswordLifecycle)
	}
	if view.MutationAuthorized || !reflect.DeepEqual(view.Attention, []string{"password_change_required", "no_effective_grants"}) {
		t.Fatalf("unexpected fail-closed projection: %#v", view)
	}
}

func TestBuildSecurityOverviewRejectsInvalidPasswordLifecycleEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	for name, changedAt := range map[string]time.Time{
		"missing":                 {},
		"before identity creation": now.Add(-31 * 24 * time.Hour),
		"future":                  now.Add(time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			input := securityOverviewFixture(now)
			input.PasswordChangedAt = changedAt
			_, err := BuildSecurityOverview(input)
			if !errors.Is(err, ErrInvalidSecurityOverview) {
				t.Fatalf("err=%v, want ErrInvalidSecurityOverview", err)
			}
		})
	}
}

func TestBuildSecurityOverviewRejectsMissingOrContradictoryCurrentSession(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*SecurityOverviewInput){
		"missing": func(in *SecurityOverviewInput) {
			in.CurrentSessionID = "session-missing"
			for index := range in.Sessions {
				in.Sessions[index].Current = false
			}
		},
		"contradictory marker": func(in *SecurityOverviewInput) {
			in.Sessions[1].Current = true
		},
		"expired": func(in *SecurityOverviewInput) {
			in.Sessions[0].ExpiresAt = now.Add(-time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := securityOverviewFixture(now)
			mutate(&input)
			_, err := BuildSecurityOverview(input)
			if !errors.Is(err, ErrInvalidSecurityOverview) {
				t.Fatalf("err=%v, want ErrInvalidSecurityOverview", err)
			}
		})
	}
}

func TestBuildSecurityOverviewRejectsNonCanonicalSecurityText(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*SecurityOverviewInput){
		"identity id too long": func(in *SecurityOverviewInput) {
			in.Identity.ID = strings.Repeat("a", maxSecurityOverviewIdentifier+1)
		},
		"display name control character": func(in *SecurityOverviewInput) {
			in.Identity.DisplayName = "Pavel\nAdmin"
		},
		"session id whitespace": func(in *SecurityOverviewInput) {
			in.Sessions[0].ID = " session-current"
		},
		"source ip whitespace": func(in *SecurityOverviewInput) {
			in.Sessions[0].SourceIP = " 192.0.2.10"
		},
		"user agent control character": func(in *SecurityOverviewInput) {
			in.Sessions[0].UserAgent = "browser\r\ninjected"
		},
		"role name control character": func(in *SecurityOverviewInput) {
			in.Grants[0].RoleName = "viewer\nadmin"
		},
		"permission too long": func(in *SecurityOverviewInput) {
			in.Grants[0].Permissions = []rbac.Permission{rbac.Permission(strings.Repeat("p", maxSecurityOverviewIdentifier+1))}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := securityOverviewFixture(now)
			mutate(&input)
			_, err := BuildSecurityOverview(input)
			if !errors.Is(err, ErrInvalidSecurityOverview) {
				t.Fatalf("err=%v, want ErrInvalidSecurityOverview", err)
			}
		})
	}
}

func TestBuildSecurityOverviewRejectsInvalidGrantEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	for name, grants := range map[string][]rbac.EffectiveGrant{
		"duplicate grant": {
			{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionOverviewRead}},
			{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionResourcesRead}},
		},
		"duplicate permission": {
			{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionOverviewRead, rbac.PermissionOverviewRead}},
		},
		"invalid scope": {
			{RoleName: "viewer", Scope: rbac.Scope{Kind: rbac.ScopeSite}, Permissions: []rbac.Permission{rbac.PermissionOverviewRead}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := securityOverviewFixture(now)
			input.Grants = grants
			_, err := BuildSecurityOverview(input)
			if !errors.Is(err, ErrInvalidSecurityOverview) {
				t.Fatalf("err=%v, want ErrInvalidSecurityOverview", err)
			}
		})
	}
}

func TestBuildSecurityOverviewRejectsInvalidPolicyOrIdentityEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 45, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*SecurityOverviewInput){
		"future identity": func(in *SecurityOverviewInput) {
			in.Identity.CreatedAt = now.Add(time.Second)
		},
		"idle exceeds absolute ttl": func(in *SecurityOverviewInput) {
			in.SessionPolicy.IdleTimeoutSeconds = in.SessionPolicy.AbsoluteTTLSeconds + 1
		},
		"activity extends absolute ttl": func(in *SecurityOverviewInput) {
			in.SessionPolicy.ActivityExtendsAbsoluteExpiry = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := securityOverviewFixture(now)
			mutate(&input)
			_, err := BuildSecurityOverview(input)
			if !errors.Is(err, ErrInvalidSecurityOverview) {
				t.Fatalf("err=%v, want ErrInvalidSecurityOverview", err)
			}
		})
	}
}

func securityOverviewFixture(now time.Time) SecurityOverviewInput {
	return SecurityOverviewInput{
		Identity: auth.Identity{
			ID:          "user-1",
			Username:    "pavel",
			DisplayName: "Pavel",
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		PasswordChangedAt: now.Add(-7 * 24 * time.Hour),
		CurrentSessionID:  "session-current",
		SessionPolicy: auth.SessionSecurityPolicyView{
			AbsoluteTTLSeconds:            int64((8 * time.Hour) / time.Second),
			IdleTimeoutSeconds:            int64((2 * time.Hour) / time.Second),
			ActivityRefreshesIdleDeadline: true,
			ActivityExtendsAbsoluteExpiry: false,
		},
		Sessions: []auth.SessionSecurityView{
			{
				ID:             "session-current",
				CreatedAt:      now.Add(-90 * time.Minute),
				LastActivityAt: now.Add(-5 * time.Minute),
				IdleExpiresAt:  now.Add(115 * time.Minute),
				ExpiresAt:      now.Add(390 * time.Minute),
				SourceIP:       "192.0.2.10",
				UserAgent:      "Mozilla/5.0 current",
				Current:        true,
			},
			{
				ID:             "session-other",
				CreatedAt:      now.Add(-2 * time.Hour),
				LastActivityAt: now.Add(-30 * time.Minute),
				IdleExpiresAt:  now.Add(90 * time.Minute),
				ExpiresAt:      now.Add(6 * time.Hour),
				SourceIP:       "2001:db8::10",
				UserAgent:      "Mozilla/5.0 other",
				Current:        false,
			},
		},
		Grants: []rbac.EffectiveGrant{
			{RoleName: "viewer", Scope: rbac.GlobalScope(), Permissions: []rbac.Permission{rbac.PermissionOverviewRead}},
		},
		Now: now,
	}
}
