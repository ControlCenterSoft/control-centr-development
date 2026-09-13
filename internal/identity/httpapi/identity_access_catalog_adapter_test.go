package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
)

type identityCatalogChecker struct {
	users bool
	roles bool
}

func (c identityCatalogChecker) Allowed(_ string, permission rbac.Permission, scope rbac.Scope) bool {
	if scope != rbac.GlobalScope() {
		return false
	}
	switch permission {
	case rbac.PermissionUsersRead:
		return c.users
	case rbac.PermissionRolesRead:
		return c.roles
	default:
		return false
	}
}

type identityCatalogIntrospector struct {
	grants []rbac.EffectiveGrant
	err    error
	calls  int
}

func (i *identityCatalogIntrospector) EffectiveGrants(context.Context, string) ([]rbac.EffectiveGrant, error) {
	i.calls++
	return append([]rbac.EffectiveGrant(nil), i.grants...), i.err
}

type identityCatalogSource struct {
	snapshot IdentityAccessCatalogSnapshot
	err      error
	calls    int
}

func (s *identityCatalogSource) IdentityAccessCatalogSnapshot(context.Context) (IdentityAccessCatalogSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

func TestIdentityAccessCatalogAdapterBuildsGlobalReadOnlySnapshot(t *testing.T) {
	now := time.Date(2026, 9, 13, 1, 50, 0, 0, time.UTC)
	introspector := &identityCatalogIntrospector{grants: []rbac.EffectiveGrant{{
		RoleName:    "administrator",
		Scope:       rbac.GlobalScope(),
		Permissions: []rbac.Permission{rbac.PermissionAll},
	}}}
	source := &identityCatalogSource{snapshot: IdentityAccessCatalogSnapshot{
		Users: []auth.User{{ID: "user-1", Username: "admin", Enabled: true, CreatedAt: now.Add(-24 * time.Hour)}},
		Roles: []rbac.Role{{Name: "administrator", Permissions: []rbac.Permission{rbac.PermissionAll}}},
		Bindings: []rbac.Binding{{SubjectID: "user-1", RoleName: "administrator", Scope: rbac.GlobalScope()}},
	}}
	adapter := IdentityAccessCatalogAdapter{
		Authorizer:   identityCatalogChecker{users: true, roles: true},
		Introspector: introspector,
		Source:       source,
		Now:          func() time.Time { return now },
	}

	view, err := adapter.Build(context.Background(), Principal{Identity: auth.Identity{ID: "operator"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !view.GlobalOnly || !view.ReadAuthorized || view.MutationAuthorized {
		t.Fatalf("unexpected catalog authority boundary: %+v", view)
	}
	if view.UserCount != 1 || view.RoleCount != 1 || view.BindingCount != 1 {
		t.Fatalf("unexpected catalog counts: %+v", view)
	}
	if introspector.calls != 1 || source.calls != 1 {
		t.Fatalf("calls introspector/source = %d/%d, want 1/1", introspector.calls, source.calls)
	}
}

func TestIdentityAccessCatalogAdapterDeniesBeforeReadingEvidence(t *testing.T) {
	for name, checker := range map[string]identityCatalogChecker{
		"missing users read": {users: false, roles: true},
		"missing roles read": {users: true, roles: false},
	} {
		t.Run(name, func(t *testing.T) {
			introspector := &identityCatalogIntrospector{}
			source := &identityCatalogSource{}
			adapter := IdentityAccessCatalogAdapter{Authorizer: checker, Introspector: introspector, Source: source}
			_, err := adapter.Build(context.Background(), Principal{Identity: auth.Identity{ID: "operator"}})
			if !errors.Is(err, ErrIdentityAccessCatalogForbidden) {
				t.Fatalf("error = %v, want forbidden", err)
			}
			if introspector.calls != 0 || source.calls != 0 {
				t.Fatalf("denied request read evidence: introspector/source = %d/%d", introspector.calls, source.calls)
			}
		})
	}
}

func TestIdentityAccessCatalogAdapterPreservesPasswordChangeBoundary(t *testing.T) {
	introspector := &identityCatalogIntrospector{}
	source := &identityCatalogSource{}
	adapter := IdentityAccessCatalogAdapter{
		Authorizer:   identityCatalogChecker{users: true, roles: true},
		Introspector: introspector,
		Source:       source,
	}
	_, err := adapter.Build(context.Background(), Principal{
		Identity:               auth.Identity{ID: "operator"},
		PasswordChangeRequired: true,
	})
	if !errors.Is(err, ErrIdentityAccessCatalogPasswordChangeRequired) {
		t.Fatalf("error = %v, want password-change boundary", err)
	}
	if introspector.calls != 0 || source.calls != 0 {
		t.Fatalf("password-change request read evidence: introspector/source = %d/%d", introspector.calls, source.calls)
	}
}

func TestIdentityAccessCatalogAdapterRejectsAuthorizationEvidenceDriftBeforeSnapshot(t *testing.T) {
	introspector := &identityCatalogIntrospector{grants: []rbac.EffectiveGrant{{
		RoleName:    "security-reader",
		Scope:       rbac.GlobalScope(),
		Permissions: []rbac.Permission{rbac.PermissionUsersRead},
	}}}
	source := &identityCatalogSource{}
	adapter := IdentityAccessCatalogAdapter{
		Authorizer:   identityCatalogChecker{users: true, roles: true},
		Introspector: introspector,
		Source:       source,
	}
	_, err := adapter.Build(context.Background(), Principal{Identity: auth.Identity{ID: "operator"}})
	if !errors.Is(err, ErrIdentityAccessCatalogAuthorizationDrift) {
		t.Fatalf("error = %v, want authorization drift", err)
	}
	if source.calls != 0 {
		t.Fatalf("authorization drift read protected snapshot %d times", source.calls)
	}
}

func TestIdentityAccessCatalogAdapterFailsClosedOnSourceOrGrantFailure(t *testing.T) {
	grantsErr := errors.New("grants unavailable")
	introspector := &identityCatalogIntrospector{err: grantsErr}
	source := &identityCatalogSource{}
	adapter := IdentityAccessCatalogAdapter{
		Authorizer:   identityCatalogChecker{users: true, roles: true},
		Introspector: introspector,
		Source:       source,
	}
	if _, err := adapter.Build(context.Background(), Principal{Identity: auth.Identity{ID: "operator"}}); !errors.Is(err, ErrIdentityAccessCatalogUnavailable) {
		t.Fatalf("grant failure error = %v, want unavailable", err)
	}
	if source.calls != 0 {
		t.Fatalf("grant failure read source %d times", source.calls)
	}

	introspector = &identityCatalogIntrospector{grants: []rbac.EffectiveGrant{{
		RoleName:    "administrator",
		Scope:       rbac.GlobalScope(),
		Permissions: []rbac.Permission{rbac.PermissionAll},
	}}}
	source = &identityCatalogSource{err: errors.New("repository unavailable")}
	adapter.Introspector = introspector
	adapter.Source = source
	if _, err := adapter.Build(context.Background(), Principal{Identity: auth.Identity{ID: "operator"}}); !errors.Is(err, ErrIdentityAccessCatalogUnavailable) {
		t.Fatalf("source failure error = %v, want unavailable", err)
	}
}
