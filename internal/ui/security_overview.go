package ui

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"control-center/internal/identity/auth"
	"control-center/internal/identity/rbac"
)

const SecurityOverviewContractVersion = "ui.security-overview/v1"

const (
	maxSecurityOverviewSessions    = 128
	maxSecurityOverviewGrants      = 64
	maxSecurityOverviewIdentifier  = 255
	maxSecurityOverviewDisplayName = 512
	maxSecurityOverviewUserAgent   = 512
	maxSecurityOverviewSourceIP    = 64
)

var ErrInvalidSecurityOverview = errors.New("invalid security overview")

type FirstLoginState string

const (
	FirstLoginChangeRequired        FirstLoginState = "password_change_required"
	FirstLoginCompleteOrNotRequired FirstLoginState = "complete_or_not_required"
)

type PasswordLifecycleState string

const (
	PasswordLifecycleChangeRequired PasswordLifecycleState = "change_required"
	PasswordLifecycleActive         PasswordLifecycleState = "active"
)

type SecurityOverviewInput struct {
	Identity               auth.Identity
	PasswordChangeRequired bool
	PasswordChangedAt      time.Time
	CurrentSessionID       string
	Sessions               []auth.SessionSecurityView
	SessionPolicy          auth.SessionSecurityPolicyView
	Grants                 []rbac.EffectiveGrant
	Now                    time.Time
}

type PasswordLifecycleView struct {
	State          PasswordLifecycleState `json:"state"`
	ChangedAt      time.Time              `json:"changed_at"`
	ChangeRequired bool                   `json:"change_required"`
}

type SecurityOverviewView struct {
	ContractVersion        string                         `json:"contract_version"`
	GeneratedAt            time.Time                      `json:"generated_at"`
	Identity               auth.Identity                  `json:"identity"`
	SelfOnly               bool                           `json:"self_only"`
	MutationAuthorized     bool                           `json:"mutation_authorized"`
	FirstLogin             FirstLoginState                `json:"first_login"`
	PasswordChangeRequired bool                           `json:"password_change_required"`
	PasswordLifecycle      PasswordLifecycleView          `json:"password_lifecycle"`
	SessionPolicy          auth.SessionSecurityPolicyView `json:"session_policy"`
	SessionCount           int                            `json:"session_count"`
	CurrentSessionID       string                         `json:"current_session_id"`
	Sessions               []SecuritySessionView          `json:"sessions"`
	GrantCount             int                            `json:"grant_count"`
	PermissionCount        int                            `json:"permission_count"`
	HasWildcardPermission  bool                           `json:"has_wildcard_permission"`
	Grants                 []rbac.EffectiveGrant          `json:"grants"`
	Attention              []string                       `json:"attention"`
}

type SecuritySessionView struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	IdleExpiresAt  time.Time `json:"idle_expires_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	SourceIP       string    `json:"source_ip,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	Current        bool      `json:"current"`
}

// BuildSecurityOverview projects only the authenticated subject's existing
// identity, password-lifecycle, session and RBAC evidence into a deterministic
// read model for the 0.33 Security/Identity UI. It never grants mutation or
// authorization.
func BuildSecurityOverview(input SecurityOverviewInput) (SecurityOverviewView, error) {
	if input.Now.IsZero() {
		return SecurityOverviewView{}, invalidSecurityOverview("current time is required")
	}
	now := input.Now.UTC()
	identity, err := validateSecurityOverviewIdentity(input.Identity, now)
	if err != nil {
		return SecurityOverviewView{}, err
	}
	passwordLifecycle, err := projectPasswordLifecycle(identity, input.PasswordChangedAt, input.PasswordChangeRequired, now)
	if err != nil {
		return SecurityOverviewView{}, err
	}
	if err := validateCanonicalSecurityText("current session id", input.CurrentSessionID, maxSecurityOverviewIdentifier, true); err != nil {
		return SecurityOverviewView{}, err
	}
	currentSessionID := input.CurrentSessionID
	if err := validateSecurityOverviewPolicy(input.SessionPolicy); err != nil {
		return SecurityOverviewView{}, err
	}
	if len(input.Sessions) == 0 || len(input.Sessions) > maxSecurityOverviewSessions {
		return SecurityOverviewView{}, invalidSecurityOverview("active session evidence must contain between 1 and %d sessions", maxSecurityOverviewSessions)
	}
	if len(input.Grants) > maxSecurityOverviewGrants {
		return SecurityOverviewView{}, invalidSecurityOverview("effective grant evidence exceeds %d entries", maxSecurityOverviewGrants)
	}

	sessions, err := normalizeSecurityOverviewSessions(input.Sessions, currentSessionID, now)
	if err != nil {
		return SecurityOverviewView{}, err
	}
	grants, permissionCount, wildcard, err := normalizeSecurityOverviewGrants(input.Grants)
	if err != nil {
		return SecurityOverviewView{}, err
	}

	firstLogin := FirstLoginCompleteOrNotRequired
	attention := make([]string, 0, 2)
	if input.PasswordChangeRequired {
		firstLogin = FirstLoginChangeRequired
		attention = append(attention, "password_change_required")
	}
	if len(grants) == 0 {
		attention = append(attention, "no_effective_grants")
	}

	return SecurityOverviewView{
		ContractVersion:        SecurityOverviewContractVersion,
		GeneratedAt:            now,
		Identity:               identity,
		SelfOnly:               true,
		MutationAuthorized:     false,
		FirstLogin:             firstLogin,
		PasswordChangeRequired: input.PasswordChangeRequired,
		PasswordLifecycle:      passwordLifecycle,
		SessionPolicy:          input.SessionPolicy,
		SessionCount:           len(sessions),
		CurrentSessionID:       currentSessionID,
		Sessions:               sessions,
		GrantCount:             len(grants),
		PermissionCount:        permissionCount,
		HasWildcardPermission:  wildcard,
		Grants:                 grants,
		Attention:              attention,
	}, nil
}

func validateSecurityOverviewIdentity(identity auth.Identity, now time.Time) (auth.Identity, error) {
	if err := validateCanonicalSecurityText("identity id", identity.ID, maxSecurityOverviewIdentifier, true); err != nil {
		return auth.Identity{}, err
	}
	if err := validateCanonicalSecurityText("username", identity.Username, maxSecurityOverviewIdentifier, true); err != nil {
		return auth.Identity{}, err
	}
	if err := validateCanonicalSecurityText("display name", identity.DisplayName, maxSecurityOverviewDisplayName, false); err != nil {
		return auth.Identity{}, err
	}
	if identity.CreatedAt.IsZero() || identity.CreatedAt.After(now) {
		return auth.Identity{}, invalidSecurityOverview("identity creation timestamp is invalid")
	}
	identity.CreatedAt = identity.CreatedAt.UTC()
	return identity, nil
}

func projectPasswordLifecycle(identity auth.Identity, changedAt time.Time, changeRequired bool, now time.Time) (PasswordLifecycleView, error) {
	if changedAt.IsZero() {
		return PasswordLifecycleView{}, invalidSecurityOverview("password changed timestamp is required")
	}
	changedAt = changedAt.UTC()
	if changedAt.Before(identity.CreatedAt) || changedAt.After(now) {
		return PasswordLifecycleView{}, invalidSecurityOverview("password changed timestamp is outside the identity lifetime")
	}
	state := PasswordLifecycleActive
	if changeRequired {
		state = PasswordLifecycleChangeRequired
	}
	return PasswordLifecycleView{
		State:          state,
		ChangedAt:      changedAt,
		ChangeRequired: changeRequired,
	}, nil
}

func validateSecurityOverviewPolicy(policy auth.SessionSecurityPolicyView) error {
	if policy.AbsoluteTTLSeconds < 60 {
		return invalidSecurityOverview("absolute session ttl must be at least 60 seconds")
	}
	if policy.IdleTimeoutSeconds < 60 || policy.IdleTimeoutSeconds > policy.AbsoluteTTLSeconds {
		return invalidSecurityOverview("idle timeout must be between 60 seconds and the absolute ttl")
	}
	if policy.ActivityExtendsAbsoluteExpiry {
		return invalidSecurityOverview("session activity must not extend absolute expiry")
	}
	return nil
}

func normalizeSecurityOverviewSessions(sources []auth.SessionSecurityView, currentSessionID string, now time.Time) ([]SecuritySessionView, error) {
	out := make([]SecuritySessionView, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	currentMatches := 0
	for _, source := range sources {
		if err := validateCanonicalSecurityText("session id", source.ID, maxSecurityOverviewIdentifier, true); err != nil {
			return nil, err
		}
		id := source.ID
		if _, exists := seen[id]; exists {
			return nil, invalidSecurityOverview("duplicate session %q", id)
		}
		seen[id] = struct{}{}
		if source.CreatedAt.IsZero() || source.LastActivityAt.IsZero() || source.IdleExpiresAt.IsZero() || source.ExpiresAt.IsZero() {
			return nil, invalidSecurityOverview("session %q has incomplete timestamps", id)
		}
		createdAt := source.CreatedAt.UTC()
		lastActivityAt := source.LastActivityAt.UTC()
		idleExpiresAt := source.IdleExpiresAt.UTC()
		expiresAt := source.ExpiresAt.UTC()
		if lastActivityAt.Before(createdAt) || !idleExpiresAt.After(lastActivityAt) || !expiresAt.After(createdAt) {
			return nil, invalidSecurityOverview("session %q has inconsistent timestamps", id)
		}
		if !now.Before(idleExpiresAt) || !now.Before(expiresAt) {
			return nil, invalidSecurityOverview("session %q is not active at projection time", id)
		}
		if source.Current != (id == currentSessionID) {
			return nil, invalidSecurityOverview("session %q current-session marker is inconsistent", id)
		}
		if source.Current {
			currentMatches++
		}
		if err := validateCanonicalSecurityText("session source ip", source.SourceIP, maxSecurityOverviewSourceIP, false); err != nil {
			return nil, err
		}
		if source.SourceIP != "" && net.ParseIP(source.SourceIP) == nil {
			return nil, invalidSecurityOverview("session %q source ip is invalid", id)
		}
		if err := validateCanonicalSecurityText("session user agent", source.UserAgent, maxSecurityOverviewUserAgent, false); err != nil {
			return nil, err
		}
		out = append(out, SecuritySessionView{
			ID:             id,
			CreatedAt:      createdAt,
			LastActivityAt: lastActivityAt,
			IdleExpiresAt:  idleExpiresAt,
			ExpiresAt:      expiresAt,
			SourceIP:       source.SourceIP,
			UserAgent:      source.UserAgent,
			Current:        source.Current,
		})
	}
	if currentMatches != 1 {
		return nil, invalidSecurityOverview("exactly one active session must match the current session")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		if !out[i].LastActivityAt.Equal(out[j].LastActivityAt) {
			return out[i].LastActivityAt.After(out[j].LastActivityAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func normalizeSecurityOverviewGrants(sources []rbac.EffectiveGrant) ([]rbac.EffectiveGrant, int, bool, error) {
	out := make([]rbac.EffectiveGrant, 0, len(sources))
	seenGrants := make(map[string]struct{}, len(sources))
	effectivePermissions := make(map[rbac.Permission]struct{})
	hasWildcard := false
	for _, source := range sources {
		if err := validateCanonicalSecurityText("effective grant role", source.RoleName, maxSecurityOverviewIdentifier, true); err != nil {
			return nil, 0, false, err
		}
		if !source.Scope.Valid() {
			return nil, 0, false, invalidSecurityOverview("effective grant has invalid scope")
		}
		if err := validateCanonicalSecurityText("effective grant scope id", source.Scope.ID, maxSecurityOverviewIdentifier, source.Scope.Kind != rbac.ScopeGlobal); err != nil {
			return nil, 0, false, err
		}
		roleName := source.RoleName
		grantKey := roleName + "\x00" + string(source.Scope.Kind) + "\x00" + source.Scope.ID
		if _, exists := seenGrants[grantKey]; exists {
			return nil, 0, false, invalidSecurityOverview("duplicate effective grant for role %q", roleName)
		}
		seenGrants[grantKey] = struct{}{}
		if len(source.Permissions) == 0 {
			return nil, 0, false, invalidSecurityOverview("effective grant %q has no permissions", roleName)
		}
		permissions := make([]rbac.Permission, 0, len(source.Permissions))
		seenPermissions := make(map[rbac.Permission]struct{}, len(source.Permissions))
		for _, permission := range source.Permissions {
			if err := validateCanonicalSecurityText("effective permission", string(permission), maxSecurityOverviewIdentifier, true); err != nil {
				return nil, 0, false, err
			}
			if _, exists := seenPermissions[permission]; exists {
				return nil, 0, false, invalidSecurityOverview("effective grant %q contains duplicate permission %q", roleName, permission)
			}
			seenPermissions[permission] = struct{}{}
			effectivePermissions[permission] = struct{}{}
			if permission == rbac.PermissionAll {
				hasWildcard = true
			}
			permissions = append(permissions, permission)
		}
		sort.Slice(permissions, func(i, j int) bool { return permissions[i] < permissions[j] })
		out = append(out, rbac.EffectiveGrant{RoleName: roleName, Scope: source.Scope, Permissions: permissions})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope.Kind != out[j].Scope.Kind {
			return out[i].Scope.Kind < out[j].Scope.Kind
		}
		if out[i].Scope.ID != out[j].Scope.ID {
			return out[i].Scope.ID < out[j].Scope.ID
		}
		return out[i].RoleName < out[j].RoleName
	})
	return out, len(effectivePermissions), hasWildcard, nil
}

func validateCanonicalSecurityText(name, value string, max int, required bool) error {
	if value == "" && !required {
		return nil
	}
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > max {
		return invalidSecurityOverview("%s is not canonical or exceeds the bound", name)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return invalidSecurityOverview("%s contains a control character", name)
		}
	}
	return nil
}

func invalidSecurityOverview(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSecurityOverview, fmt.Sprintf(format, values...))
}
