package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
)

const IdentityAccessCatalogContractVersion = "ui.identity-access-catalog/v1"

const (
	maxIdentityAccessCatalogUsers              = 256
	maxIdentityAccessCatalogRoles              = 128
	maxIdentityAccessCatalogBindings           = 1024
	maxIdentityAccessCatalogPermissionsPerRole = 128
	maxIdentityAccessCatalogDisplayNameBytes   = 256
	maxIdentityAccessCatalogDescriptionBytes   = 512
)

var ErrInvalidIdentityAccessCatalog = errors.New("invalid identity access catalog")

type IdentityAccessCatalogInput struct {
	CallerGrants     []rbac.EffectiveGrant
	UsersReadAllowed bool
	RolesReadAllowed bool
	Users            []auth.User
	Roles            []rbac.Role
	Bindings         []rbac.Binding
	GeneratedAt      time.Time
}

type IdentityAccessCatalogView struct {
	ContractVersion    string                      `json:"contract_version"`
	GeneratedAt        time.Time                   `json:"generated_at"`
	GlobalOnly         bool                        `json:"global_only"`
	ReadAuthorized     bool                        `json:"read_authorized"`
	MutationAuthorized bool                        `json:"mutation_authorized"`
	UserCount          int                         `json:"user_count"`
	RoleCount          int                         `json:"role_count"`
	BindingCount       int                         `json:"binding_count"`
	Users              []IdentityAccessUserView    `json:"users"`
	Roles              []IdentityAccessRoleView    `json:"roles"`
	Bindings           []IdentityAccessBindingView `json:"bindings"`
}

type IdentityAccessUserView struct {
	ID                     string     `json:"id"`
	Username               string     `json:"username"`
	DisplayName            string     `json:"display_name,omitempty"`
	Enabled                bool       `json:"enabled"`
	PasswordChangeRequired bool       `json:"password_change_required"`
	CreatedAt              time.Time  `json:"created_at"`
	PasswordChangedAt      *time.Time `json:"password_changed_at,omitempty"`
	LastLoginAt            *time.Time `json:"last_login_at,omitempty"`
}

type IdentityAccessRoleView struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Permissions []rbac.Permission `json:"permissions"`
}

type IdentityAccessBindingView struct {
	SubjectID string     `json:"subject_id"`
	RoleName  string     `json:"role_name"`
	Scope     rbac.Scope `json:"scope"`
}

// BuildIdentityAccessCatalog projects a complete global, read-only identity and
// RBAC catalog for the 0.33 administrative Security UI. The caller must already
// have authoritative server decisions for both identity.users.read and
// identity.roles.read. Those decisions are cross-checked against the caller's
// effective grants so stale or contradictory authorization evidence fails
// closed. The result never contains password hashes, tokens, secret material or
// mutation authority.
func BuildIdentityAccessCatalog(input IdentityAccessCatalogInput) (IdentityAccessCatalogView, error) {
	if input.GeneratedAt.IsZero() {
		return IdentityAccessCatalogView{}, invalidIdentityAccessCatalog("generated_at is required")
	}
	generatedAt := input.GeneratedAt.UTC()
	if len(input.CallerGrants) > maxSecurityOverviewGrants {
		return IdentityAccessCatalogView{}, invalidIdentityAccessCatalog("caller grant evidence exceeds %d entries", maxSecurityOverviewGrants)
	}
	grants, _, _, err := normalizeSecurityOverviewGrants(input.CallerGrants)
	if err != nil {
		return IdentityAccessCatalogView{}, fmt.Errorf("%w: invalid caller grant evidence: %v", ErrInvalidIdentityAccessCatalog, err)
	}

	usersDerived := identityAccessGlobalPermissionGranted(grants, rbac.PermissionUsersRead)
	rolesDerived := identityAccessGlobalPermissionGranted(grants, rbac.PermissionRolesRead)
	if usersDerived != input.UsersReadAllowed || rolesDerived != input.RolesReadAllowed {
		return IdentityAccessCatalogView{}, invalidIdentityAccessCatalog("server authorization decisions are inconsistent with supplied effective grants")
	}
	if !input.UsersReadAllowed || !input.RolesReadAllowed {
		return IdentityAccessCatalogView{}, invalidIdentityAccessCatalog("global users and roles read authorization is required")
	}

	users, userIDs, err := normalizeIdentityAccessUsers(input.Users, generatedAt)
	if err != nil {
		return IdentityAccessCatalogView{}, err
	}
	roles, roleNames, err := normalizeIdentityAccessRoles(input.Roles)
	if err != nil {
		return IdentityAccessCatalogView{}, err
	}
	bindings, err := normalizeIdentityAccessBindings(input.Bindings, userIDs, roleNames)
	if err != nil {
		return IdentityAccessCatalogView{}, err
	}

	return IdentityAccessCatalogView{
		ContractVersion:    IdentityAccessCatalogContractVersion,
		GeneratedAt:        generatedAt,
		GlobalOnly:         true,
		ReadAuthorized:     true,
		MutationAuthorized: false,
		UserCount:          len(users),
		RoleCount:          len(roles),
		BindingCount:       len(bindings),
		Users:              users,
		Roles:              roles,
		Bindings:           bindings,
	}, nil
}

func identityAccessGlobalPermissionGranted(grants []rbac.EffectiveGrant, permission rbac.Permission) bool {
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

func normalizeIdentityAccessUsers(sources []auth.User, generatedAt time.Time) ([]IdentityAccessUserView, map[string]struct{}, error) {
	if len(sources) > maxIdentityAccessCatalogUsers {
		return nil, nil, invalidIdentityAccessCatalog("user evidence exceeds %d entries", maxIdentityAccessCatalogUsers)
	}
	out := make([]IdentityAccessUserView, 0, len(sources))
	userIDs := make(map[string]struct{}, len(sources))
	usernames := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		id := strings.TrimSpace(source.ID)
		username := strings.TrimSpace(source.Username)
		displayName := strings.TrimSpace(source.DisplayName)
		if id == "" || id != source.ID {
			return nil, nil, invalidIdentityAccessCatalog("user id must be canonical")
		}
		if username == "" || username != source.Username {
			return nil, nil, invalidIdentityAccessCatalog("username must be canonical")
		}
		if source.DisplayName != "" && displayName != source.DisplayName {
			return nil, nil, invalidIdentityAccessCatalog("display name must be canonical when present")
		}
		if len(displayName) > maxIdentityAccessCatalogDisplayNameBytes {
			return nil, nil, invalidIdentityAccessCatalog("display name exceeds %d bytes", maxIdentityAccessCatalogDisplayNameBytes)
		}
		if _, exists := userIDs[id]; exists {
			return nil, nil, invalidIdentityAccessCatalog("duplicate user id %q", id)
		}
		if _, exists := usernames[username]; exists {
			return nil, nil, invalidIdentityAccessCatalog("duplicate username %q", username)
		}
		if source.CreatedAt.IsZero() || source.CreatedAt.After(generatedAt) {
			return nil, nil, invalidIdentityAccessCatalog("user %q has invalid creation timestamp", username)
		}
		createdAt := source.CreatedAt.UTC()
		passwordChangedAt, err := identityAccessOptionalTimestamp(source.PasswordChangedAt, createdAt, generatedAt, "password change", username)
		if err != nil {
			return nil, nil, err
		}
		lastLoginAt, err := identityAccessOptionalTimestampPointer(source.LastLoginAt, createdAt, generatedAt, "last login", username)
		if err != nil {
			return nil, nil, err
		}
		userIDs[id] = struct{}{}
		usernames[username] = struct{}{}
		out = append(out, IdentityAccessUserView{
			ID:                     id,
			Username:               username,
			DisplayName:            displayName,
			Enabled:                source.Enabled,
			PasswordChangeRequired: source.PasswordChangeRequired,
			CreatedAt:              createdAt,
			PasswordChangedAt:      passwordChangedAt,
			LastLoginAt:            lastLoginAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Username != out[j].Username {
			return out[i].Username < out[j].Username
		}
		return out[i].ID < out[j].ID
	})
	return out, userIDs, nil
}

func identityAccessOptionalTimestamp(value time.Time, lower, upper time.Time, label, username string) (*time.Time, error) {
	if value.IsZero() {
		return nil, nil
	}
	normalized := value.UTC()
	if normalized.Before(lower) || normalized.After(upper) {
		return nil, invalidIdentityAccessCatalog("user %q has invalid %s timestamp", username, label)
	}
	return &normalized, nil
}

func identityAccessOptionalTimestampPointer(value *time.Time, lower, upper time.Time, label, username string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if value.IsZero() {
		return nil, invalidIdentityAccessCatalog("user %q has invalid %s timestamp", username, label)
	}
	normalized := value.UTC()
	if normalized.Before(lower) || normalized.After(upper) {
		return nil, invalidIdentityAccessCatalog("user %q has invalid %s timestamp", username, label)
	}
	return &normalized, nil
}

func normalizeIdentityAccessRoles(sources []rbac.Role) ([]IdentityAccessRoleView, map[string]struct{}, error) {
	if len(sources) > maxIdentityAccessCatalogRoles {
		return nil, nil, invalidIdentityAccessCatalog("role evidence exceeds %d entries", maxIdentityAccessCatalogRoles)
	}
	out := make([]IdentityAccessRoleView, 0, len(sources))
	roleNames := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		name := strings.TrimSpace(source.Name)
		description := strings.TrimSpace(source.Description)
		if name == "" || name != source.Name {
			return nil, nil, invalidIdentityAccessCatalog("role name must be canonical")
		}
		if source.Description != "" && description != source.Description {
			return nil, nil, invalidIdentityAccessCatalog("role %q description must be canonical when present", name)
		}
		if len(description) > maxIdentityAccessCatalogDescriptionBytes {
			return nil, nil, invalidIdentityAccessCatalog("role %q description exceeds %d bytes", name, maxIdentityAccessCatalogDescriptionBytes)
		}
		if _, exists := roleNames[name]; exists {
			return nil, nil, invalidIdentityAccessCatalog("duplicate role %q", name)
		}
		if len(source.Permissions) == 0 || len(source.Permissions) > maxIdentityAccessCatalogPermissionsPerRole {
			return nil, nil, invalidIdentityAccessCatalog("role %q must contain between 1 and %d permissions", name, maxIdentityAccessCatalogPermissionsPerRole)
		}
		permissions := make([]rbac.Permission, 0, len(source.Permissions))
		seenPermissions := make(map[rbac.Permission]struct{}, len(source.Permissions))
		for _, permission := range source.Permissions {
			canonical := rbac.Permission(strings.TrimSpace(string(permission)))
			if canonical == "" || canonical != permission {
				return nil, nil, invalidIdentityAccessCatalog("role %q contains a non-canonical permission", name)
			}
			if _, exists := seenPermissions[canonical]; exists {
				return nil, nil, invalidIdentityAccessCatalog("role %q contains duplicate permission %q", name, canonical)
			}
			seenPermissions[canonical] = struct{}{}
			permissions = append(permissions, canonical)
		}
		sort.Slice(permissions, func(i, j int) bool { return permissions[i] < permissions[j] })
		roleNames[name] = struct{}{}
		out = append(out, IdentityAccessRoleView{Name: name, Description: description, Permissions: permissions})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, roleNames, nil
}

func normalizeIdentityAccessBindings(sources []rbac.Binding, userIDs, roleNames map[string]struct{}) ([]IdentityAccessBindingView, error) {
	if len(sources) > maxIdentityAccessCatalogBindings {
		return nil, invalidIdentityAccessCatalog("binding evidence exceeds %d entries", maxIdentityAccessCatalogBindings)
	}
	out := make([]IdentityAccessBindingView, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		subjectID := strings.TrimSpace(source.SubjectID)
		roleName := strings.TrimSpace(source.RoleName)
		if subjectID == "" || subjectID != source.SubjectID {
			return nil, invalidIdentityAccessCatalog("binding subject id must be canonical")
		}
		if roleName == "" || roleName != source.RoleName {
			return nil, invalidIdentityAccessCatalog("binding role name must be canonical")
		}
		if !source.Scope.Valid() {
			return nil, invalidIdentityAccessCatalog("binding for subject %q has invalid scope", subjectID)
		}
		if _, exists := userIDs[subjectID]; !exists {
			return nil, invalidIdentityAccessCatalog("binding references unknown subject %q", subjectID)
		}
		if _, exists := roleNames[roleName]; !exists {
			return nil, invalidIdentityAccessCatalog("binding references unknown role %q", roleName)
		}
		key := subjectID + "\x00" + roleName + "\x00" + string(source.Scope.Kind) + "\x00" + source.Scope.ID
		if _, exists := seen[key]; exists {
			return nil, invalidIdentityAccessCatalog("duplicate binding for subject %q and role %q", subjectID, roleName)
		}
		seen[key] = struct{}{}
		out = append(out, IdentityAccessBindingView{SubjectID: subjectID, RoleName: roleName, Scope: source.Scope})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SubjectID != out[j].SubjectID {
			return out[i].SubjectID < out[j].SubjectID
		}
		if out[i].RoleName != out[j].RoleName {
			return out[i].RoleName < out[j].RoleName
		}
		if out[i].Scope.Kind != out[j].Scope.Kind {
			return out[i].Scope.Kind < out[j].Scope.Kind
		}
		return out[i].Scope.ID < out[j].Scope.ID
	})
	return out, nil
}

func invalidIdentityAccessCatalog(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidIdentityAccessCatalog, fmt.Sprintf(format, values...))
}
