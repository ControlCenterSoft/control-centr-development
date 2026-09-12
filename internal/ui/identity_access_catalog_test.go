package ui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
)

func TestBuildIdentityAccessCatalogProjectsSafeDeterministicGlobalView(t *testing.T) {
	now := time.Date(2026, 9, 12, 17, 40, 0, 0, time.UTC)
	passwordChanged := now.Add(-48 * time.Hour)
	lastLogin := now.Add(-5 * time.Minute)
	input := IdentityAccessCatalogInput{
		GeneratedAt:      now,
		UsersReadAllowed: true,
		RolesReadAllowed: true,
		CallerGrants: []rbac.EffectiveGrant{{
			RoleName:    "administrator",
			Scope:       rbac.GlobalScope(),
			Permissions: []rbac.Permission{rbac.PermissionAll},
		}},
		Users: []auth.User{
			{
				ID:                     "user-2",
				Username:               "viewer",
				DisplayName:            "Viewer",
				PasswordHash:           "must-never-be-projected",
				Enabled:                true,
				PasswordChangeRequired: false,
				CreatedAt:              now.Add(-72 * time.Hour),
				PasswordChangedAt:      passwordChanged,
				LastLoginAt:            &lastLogin,
			},
			{
				ID:                     "user-1",
				Username:               "admin",
				DisplayName:            "Administrator",
				PasswordHash:           "also-secret",
				Enabled:                true,
				PasswordChangeRequired: true,
				CreatedAt:              now.Add(-96 * time.Hour),
			},
		},
		Roles: []rbac.Role{
			{Name: "viewer", Description: "Read only", Permissions: []rbac.Permission{rbac.PermissionResourcesRead, rbac.PermissionOverviewRead}},
			{Name: "administrator", Description: "Full administration", Permissions: []rbac.Permission{rbac.PermissionAll}},
		},
		Bindings: []rbac.Binding{
			{SubjectID: "user-2", RoleName: "viewer", Scope: rbac.Scope{Kind: rbac.ScopeSite, ID: "site-b"}},
			{SubjectID: "user-1", RoleName: "administrator", Scope: rbac.GlobalScope()},
		},
	}

	view, err := BuildIdentityAccessCatalog(input)
	if err != nil {
		t.Fatalf("BuildIdentityAccessCatalog() error = %v", err)
	}
	if view.ContractVersion != IdentityAccessCatalogContractVersion || !view.GlobalOnly || !view.ReadAuthorized || view.MutationAuthorized {
		t.Fatalf("unexpected security boundary: %+v", view)
	}
	if view.UserCount != 2 || view.RoleCount != 2 || view.BindingCount != 2 {
		t.Fatalf("unexpected counts: users=%d roles=%d bindings=%d", view.UserCount, view.RoleCount, view.BindingCount)
	}
	if got := view.Users[0].Username; got != "admin" {
		t.Fatalf("users are not deterministic: first username = %q", got)
	}
	if got := view.Roles[0].Name; got != "administrator" {
		t.Fatalf("roles are not deterministic: first role = %q", got)
	}
	if got := view.Roles[1].Permissions; len(got) != 2 || got[0] != rbac.PermissionOverviewRead || got[1] != rbac.PermissionResourcesRead {
		t.Fatalf("permissions are not sorted: %v", got)
	}
	if got := view.Bindings[0].SubjectID; got != "user-1" {
		t.Fatalf("bindings are not deterministic: first subject = %q", got)
	}

	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"must-never-be-projected", "also-secret", "password_hash", "token_digest"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("catalog leaked secret material %q: %s", forbidden, serialized)
		}
	}
}

func TestBuildIdentityAccessCatalogRejectsAuthorizationDrift(t *testing.T) {
	input := minimalIdentityAccessCatalogInput()
	input.RolesReadAllowed = false

	_, err := BuildIdentityAccessCatalog(input)
	if !errors.Is(err, ErrInvalidIdentityAccessCatalog) {
		t.Fatalf("expected ErrInvalidIdentityAccessCatalog, got %v", err)
	}
	if !strings.Contains(err.Error(), "inconsistent") {
		t.Fatalf("expected authorization drift failure, got %v", err)
	}
}

func TestBuildIdentityAccessCatalogRejectsScopedGrantForGlobalCatalog(t *testing.T) {
	input := minimalIdentityAccessCatalogInput()
	input.CallerGrants = []rbac.EffectiveGrant{{
		RoleName: "security-reader",
		Scope:    rbac.Scope{Kind: rbac.ScopeSite, ID: "site-a"},
		Permissions: []rbac.Permission{
			rbac.PermissionUsersRead,
			rbac.PermissionRolesRead,
		},
	}}
	input.UsersReadAllowed = false
	input.RolesReadAllowed = false

	_, err := BuildIdentityAccessCatalog(input)
	if !errors.Is(err, ErrInvalidIdentityAccessCatalog) {
		t.Fatalf("expected ErrInvalidIdentityAccessCatalog, got %v", err)
	}
	if !strings.Contains(err.Error(), "global users and roles read authorization") {
		t.Fatalf("expected global authorization failure, got %v", err)
	}
}

func TestBuildIdentityAccessCatalogRejectsUnknownBindingReferences(t *testing.T) {
	input := minimalIdentityAccessCatalogInput()
	input.Bindings = []rbac.Binding{{SubjectID: "missing-user", RoleName: "administrator", Scope: rbac.GlobalScope()}}

	_, err := BuildIdentityAccessCatalog(input)
	if !errors.Is(err, ErrInvalidIdentityAccessCatalog) || !strings.Contains(err.Error(), "unknown subject") {
		t.Fatalf("expected unknown subject failure, got %v", err)
	}

	input = minimalIdentityAccessCatalogInput()
	input.Bindings = []rbac.Binding{{SubjectID: "user-1", RoleName: "missing-role", Scope: rbac.GlobalScope()}}
	_, err = BuildIdentityAccessCatalog(input)
	if !errors.Is(err, ErrInvalidIdentityAccessCatalog) || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("expected unknown role failure, got %v", err)
	}
}

func TestBuildIdentityAccessCatalogRejectsDuplicateAndFutureEvidence(t *testing.T) {
	input := minimalIdentityAccessCatalogInput()
	duplicate := input.Users[0]
	duplicate.Username = "other"
	input.Users = append(input.Users, duplicate)
	if _, err := BuildIdentityAccessCatalog(input); !errors.Is(err, ErrInvalidIdentityAccessCatalog) || !strings.Contains(err.Error(), "duplicate user id") {
		t.Fatalf("expected duplicate user id failure, got %v", err)
	}

	input = minimalIdentityAccessCatalogInput()
	input.Users[0].LastLoginAt = ptrIdentityAccessTime(input.GeneratedAt.Add(time.Second))
	if _, err := BuildIdentityAccessCatalog(input); !errors.Is(err, ErrInvalidIdentityAccessCatalog) || !strings.Contains(err.Error(), "last login") {
		t.Fatalf("expected future last-login failure, got %v", err)
	}
}

func TestBuildIdentityAccessCatalogRejectsDuplicateBinding(t *testing.T) {
	input := minimalIdentityAccessCatalogInput()
	binding := rbac.Binding{SubjectID: "user-1", RoleName: "administrator", Scope: rbac.GlobalScope()}
	input.Bindings = []rbac.Binding{binding, binding}

	_, err := BuildIdentityAccessCatalog(input)
	if !errors.Is(err, ErrInvalidIdentityAccessCatalog) || !strings.Contains(err.Error(), "duplicate binding") {
		t.Fatalf("expected duplicate binding failure, got %v", err)
	}
}

func minimalIdentityAccessCatalogInput() IdentityAccessCatalogInput {
	now := time.Date(2026, 9, 12, 17, 40, 0, 0, time.UTC)
	return IdentityAccessCatalogInput{
		GeneratedAt:      now,
		UsersReadAllowed: true,
		RolesReadAllowed: true,
		CallerGrants: []rbac.EffectiveGrant{{
			RoleName:    "administrator",
			Scope:       rbac.GlobalScope(),
			Permissions: []rbac.Permission{rbac.PermissionAll},
		}},
		Users: []auth.User{{
			ID:                     "user-1",
			Username:               "admin",
			DisplayName:            "Administrator",
			PasswordHash:           "secret",
			Enabled:                true,
			PasswordChangeRequired: false,
			CreatedAt:              now.Add(-24 * time.Hour),
		}},
		Roles: []rbac.Role{{Name: "administrator", Description: "Full administration", Permissions: []rbac.Permission{rbac.PermissionAll}}},
		Bindings: []rbac.Binding{{SubjectID: "user-1", RoleName: "administrator", Scope: rbac.GlobalScope()}},
	}
}

func ptrIdentityAccessTime(value time.Time) *time.Time {
	return &value
}
