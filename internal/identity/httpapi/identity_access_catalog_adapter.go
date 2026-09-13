package httpapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
	"control-center/internal/ui"
)

var (
	ErrIdentityAccessCatalogUnavailable            = errors.New("identity access catalog unavailable")
	ErrIdentityAccessCatalogForbidden              = errors.New("identity access catalog forbidden")
	ErrIdentityAccessCatalogPasswordChangeRequired = errors.New("identity access catalog requires current password")
	ErrIdentityAccessCatalogAuthorizationDrift     = errors.New("identity access catalog authorization evidence drift")
)

// IdentityAccessCatalogSnapshot is the bounded authoritative source material
// needed by the 0.33 global read-only Identity/RBAC catalog. Implementations
// must not include password hashes, tokens or other secret material outside the
// auth.User objects consumed by the redacting ui builder.
type IdentityAccessCatalogSnapshot struct {
	Users    []auth.User
	Roles    []rbac.Role
	Bindings []rbac.Binding
}

// IdentityAccessCatalogSource deliberately exposes one complete snapshot rather
// than independent user/role/binding reads so the UI cannot combine evidence
// from different repository generations and present it as one coherent state.
type IdentityAccessCatalogSource interface {
	IdentityAccessCatalogSnapshot(context.Context) (IdentityAccessCatalogSnapshot, error)
}

// IdentityAccessCatalogAdapter binds the source-only 0.33 read model to
// authoritative server-side authentication/RBAC evidence without registering
// an HTTP route. Route activation remains a separate runner-qualified slice.
type IdentityAccessCatalogAdapter struct {
	Authorizer   rbac.Checker
	Introspector rbac.Introspector
	Source       IdentityAccessCatalogSource
	Now          func() time.Time
}

func (a IdentityAccessCatalogAdapter) Build(ctx context.Context, principal Principal) (ui.IdentityAccessCatalogView, error) {
	if a.Authorizer == nil || a.Introspector == nil || a.Source == nil {
		return ui.IdentityAccessCatalogView{}, ErrIdentityAccessCatalogUnavailable
	}
	if principal.Identity.ID == "" {
		return ui.IdentityAccessCatalogView{}, ErrIdentityAccessCatalogForbidden
	}
	if principal.PasswordChangeRequired {
		return ui.IdentityAccessCatalogView{}, ErrIdentityAccessCatalogPasswordChangeRequired
	}

	global := rbac.GlobalScope()
	usersAllowed := a.Authorizer.Allowed(principal.Identity.ID, rbac.PermissionUsersRead, global)
	rolesAllowed := a.Authorizer.Allowed(principal.Identity.ID, rbac.PermissionRolesRead, global)
	if !usersAllowed || !rolesAllowed {
		return ui.IdentityAccessCatalogView{}, ErrIdentityAccessCatalogForbidden
	}

	grants, err := a.Introspector.EffectiveGrants(ctx, principal.Identity.ID)
	if err != nil {
		return ui.IdentityAccessCatalogView{}, fmt.Errorf("%w: effective grants: %v", ErrIdentityAccessCatalogUnavailable, err)
	}
	if globalPermissionGranted(grants, rbac.PermissionUsersRead) != usersAllowed ||
		globalPermissionGranted(grants, rbac.PermissionRolesRead) != rolesAllowed {
		return ui.IdentityAccessCatalogView{}, ErrIdentityAccessCatalogAuthorizationDrift
	}

	snapshot, err := a.Source.IdentityAccessCatalogSnapshot(ctx)
	if err != nil {
		return ui.IdentityAccessCatalogView{}, fmt.Errorf("%w: snapshot: %v", ErrIdentityAccessCatalogUnavailable, err)
	}

	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	if now.IsZero() {
		return ui.IdentityAccessCatalogView{}, ErrIdentityAccessCatalogUnavailable
	}

	view, err := ui.BuildIdentityAccessCatalog(ui.IdentityAccessCatalogInput{
		CallerGrants:     grants,
		UsersReadAllowed: usersAllowed,
		RolesReadAllowed: rolesAllowed,
		Users:            snapshot.Users,
		Roles:            snapshot.Roles,
		Bindings:         snapshot.Bindings,
		GeneratedAt:      now,
	})
	if err != nil {
		return ui.IdentityAccessCatalogView{}, fmt.Errorf("%w: projection: %v", ErrIdentityAccessCatalogUnavailable, err)
	}
	return view, nil
}

func globalPermissionGranted(grants []rbac.EffectiveGrant, permission rbac.Permission) bool {
	for _, grant := range grants {
		if grant.Scope.Kind != rbac.ScopeGlobal || grant.Scope.ID != "" {
			continue
		}
		for _, granted := range grant.Permissions {
			if granted == rbac.PermissionAll || granted == permission {
				return true
			}
		}
	}
	return false
}
