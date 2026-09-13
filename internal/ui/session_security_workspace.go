package ui

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	SessionSecurityWorkspaceSchemaV1 = "ui.session-security-workspace/v1"

	SessionSecurityDataLoaded      = "loaded"
	SessionSecurityDataUnavailable = "unavailable"

	SessionActionRevokeSession  = "revoke-session"
	SessionActionLogsOutCurrent = "logs-out-current-session"

	maxSessionSecuritySessions   = 64
	maxSessionIdentityLength     = 128
	maxSessionDisplayNameLength  = 256
	maxSessionIdentifierLength   = 128
	maxSessionSourceIPLength     = 128
	maxSessionUserAgentLength    = 512
	maxSessionAbsoluteTTLSeconds = int64(7 * 24 * 60 * 60)
	maxSessionIdleTimeoutSeconds = maxSessionAbsoluteTTLSeconds
)

// SessionSecurityPolicyInput is the already-effective server-side policy that
// may be projected into the self-only session security workspace. It does not
// accept a client-selected policy or grant policy mutation authority.
type SessionSecurityPolicyInput struct {
	AbsoluteTTLSeconds            int64
	IdleTimeoutSeconds            int64
	ActivityRefreshesIdleDeadline bool
	ActivityExtendsAbsoluteExpiry bool
}

// SessionSecurityInput is a bounded server-side session observation. The input
// intentionally contains no bearer token, token digest, password material,
// role binding or authorization secret.
type SessionSecurityInput struct {
	ID             string
	CreatedAt      time.Time
	LastActivityAt time.Time
	IdleExpiresAt  time.Time
	ExpiresAt      time.Time
	SourceIP       string
	UserAgent      string
	Current        bool
}

// SessionSecurityWorkspaceInput is assembled only after authentication and the
// mandatory first-login password-change boundary. Actor identity is therefore
// authoritative server context, never a client-selected subject.
type SessionSecurityWorkspaceInput struct {
	Loaded                 bool
	ActorID                string
	Username               string
	DisplayName            string
	CurrentSessionID       string
	PasswordChangeRequired bool
	Policy                 SessionSecurityPolicyInput
	Sessions               []SessionSecurityInput
	Now                    time.Time
}

type SessionSecurityIdentityView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type SessionSecurityPolicyView struct {
	AbsoluteTTLSeconds            int64 `json:"absolute_ttl_seconds"`
	IdleTimeoutSeconds            int64 `json:"idle_timeout_seconds"`
	ActivityRefreshesIdleDeadline bool  `json:"activity_refreshes_idle_deadline"`
	ActivityExtendsAbsoluteExpiry bool  `json:"activity_extends_absolute_expiry"`
}

type SessionSecuritySessionView struct {
	ID                       string    `json:"id"`
	CreatedAt                time.Time `json:"created_at"`
	LastActivityAt           time.Time `json:"last_activity_at"`
	IdleExpiresAt            time.Time `json:"idle_expires_at"`
	ExpiresAt                time.Time `json:"expires_at"`
	SourceIP                 string    `json:"source_ip,omitempty"`
	UserAgent                string    `json:"user_agent,omitempty"`
	Current                  bool      `json:"current"`
	IdleRemainingSeconds     int64     `json:"idle_remaining_seconds"`
	AbsoluteRemainingSeconds int64     `json:"absolute_remaining_seconds"`
	RevocationRisk           string    `json:"revocation_risk"`
}

// SessionSecurityWorkspace is a self-only read model. MutationAuthorized is
// permanently false: rendering a revoke control is not authorization to revoke
// a session. Any mutation must re-enter the authenticated server-side self-only
// revocation boundary and its Audit path.
type SessionSecurityWorkspace struct {
	Schema             string                       `json:"schema"`
	DataState          string                       `json:"data_state"`
	SelfOnly           bool                         `json:"self_only"`
	Identity           *SessionSecurityIdentityView `json:"identity,omitempty"`
	Policy             *SessionSecurityPolicyView   `json:"policy,omitempty"`
	Sessions           []SessionSecuritySessionView `json:"sessions"`
	MutationAuthorized bool                         `json:"mutation_authorized"`
}

// BuildSessionSecurityWorkspace validates and normalizes self-only session
// evidence for the 0.33 Security/Identity UI. It fails closed on partial,
// expired, duplicate, policy-inconsistent or cross-current-session evidence
// rather than presenting an incomplete session inventory as authoritative.
func BuildSessionSecurityWorkspace(input SessionSecurityWorkspaceInput) (SessionSecurityWorkspace, error) {
	if input.Now.IsZero() {
		return SessionSecurityWorkspace{}, fmt.Errorf("trusted now is required")
	}
	if len(input.Sessions) > maxSessionSecuritySessions {
		return SessionSecurityWorkspace{}, fmt.Errorf("session inventory limit exceeded")
	}
	if !input.Loaded {
		if strings.TrimSpace(input.ActorID) != "" || strings.TrimSpace(input.Username) != "" || strings.TrimSpace(input.CurrentSessionID) != "" || len(input.Sessions) != 0 {
			return SessionSecurityWorkspace{}, fmt.Errorf("unavailable session source cannot include partial identity or session evidence")
		}
		return SessionSecurityWorkspace{
			Schema: SessionSecurityWorkspaceSchemaV1, DataState: SessionSecurityDataUnavailable,
			SelfOnly: true, Sessions: []SessionSecuritySessionView{}, MutationAuthorized: false,
		}, nil
	}
	if input.PasswordChangeRequired {
		return SessionSecurityWorkspace{}, fmt.Errorf("password change is required before session security workspace access")
	}

	actorID := strings.TrimSpace(input.ActorID)
	username := strings.TrimSpace(input.Username)
	displayName := strings.TrimSpace(input.DisplayName)
	currentSessionID := strings.TrimSpace(input.CurrentSessionID)
	if err := validateSessionText("actor id", actorID, maxSessionIdentityLength, false); err != nil {
		return SessionSecurityWorkspace{}, err
	}
	if err := validateSessionText("username", username, maxSessionIdentityLength, false); err != nil {
		return SessionSecurityWorkspace{}, err
	}
	if err := validateSessionText("display name", displayName, maxSessionDisplayNameLength, true); err != nil {
		return SessionSecurityWorkspace{}, err
	}
	if err := validateSessionText("current session id", currentSessionID, maxSessionIdentifierLength, false); err != nil {
		return SessionSecurityWorkspace{}, err
	}
	policy, err := normalizeSessionSecurityPolicy(input.Policy)
	if err != nil {
		return SessionSecurityWorkspace{}, err
	}

	now := input.Now.UTC()
	sessions := make([]SessionSecuritySessionView, 0, len(input.Sessions))
	seen := make(map[string]struct{}, len(input.Sessions))
	currentCount := 0
	for _, raw := range input.Sessions {
		view, err := normalizeSessionSecuritySession(raw, now, policy)
		if err != nil {
			return SessionSecurityWorkspace{}, err
		}
		if _, exists := seen[view.ID]; exists {
			return SessionSecurityWorkspace{}, fmt.Errorf("duplicate session id %q", view.ID)
		}
		seen[view.ID] = struct{}{}
		if view.Current {
			currentCount++
			if view.ID != currentSessionID {
				return SessionSecurityWorkspace{}, fmt.Errorf("current session marker does not match authenticated session")
			}
		} else if view.ID == currentSessionID {
			return SessionSecurityWorkspace{}, fmt.Errorf("authenticated session is missing current-session marker")
		}
		sessions = append(sessions, view)
	}
	if currentCount != 1 {
		return SessionSecurityWorkspace{}, fmt.Errorf("exactly one current session is required")
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Current != sessions[j].Current {
			return sessions[i].Current
		}
		if !sessions[i].LastActivityAt.Equal(sessions[j].LastActivityAt) {
			return sessions[i].LastActivityAt.After(sessions[j].LastActivityAt)
		}
		return sessions[i].ID < sessions[j].ID
	})

	return SessionSecurityWorkspace{
		Schema:    SessionSecurityWorkspaceSchemaV1,
		DataState: SessionSecurityDataLoaded,
		SelfOnly:  true,
		Identity: &SessionSecurityIdentityView{
			ID: actorID, Username: username, DisplayName: displayName,
		},
		Policy:             &policy,
		Sessions:           sessions,
		MutationAuthorized: false,
	}, nil
}

func normalizeSessionSecurityPolicy(input SessionSecurityPolicyInput) (SessionSecurityPolicyView, error) {
	if input.AbsoluteTTLSeconds < 60 || input.AbsoluteTTLSeconds > maxSessionAbsoluteTTLSeconds {
		return SessionSecurityPolicyView{}, fmt.Errorf("absolute session TTL outside allowed range")
	}
	if input.IdleTimeoutSeconds < 60 || input.IdleTimeoutSeconds > input.AbsoluteTTLSeconds || input.IdleTimeoutSeconds > maxSessionIdleTimeoutSeconds {
		return SessionSecurityPolicyView{}, fmt.Errorf("session idle timeout outside allowed range")
	}
	if input.ActivityExtendsAbsoluteExpiry {
		return SessionSecurityPolicyView{}, fmt.Errorf("activity must not extend absolute session expiry")
	}
	return SessionSecurityPolicyView{
		AbsoluteTTLSeconds:            input.AbsoluteTTLSeconds,
		IdleTimeoutSeconds:            input.IdleTimeoutSeconds,
		ActivityRefreshesIdleDeadline: input.ActivityRefreshesIdleDeadline,
		ActivityExtendsAbsoluteExpiry: false,
	}, nil
}

func normalizeSessionSecuritySession(input SessionSecurityInput, now time.Time, policy SessionSecurityPolicyView) (SessionSecuritySessionView, error) {
	id := strings.TrimSpace(input.ID)
	sourceIP := strings.TrimSpace(input.SourceIP)
	userAgent := strings.TrimSpace(input.UserAgent)
	if err := validateSessionText("session id", id, maxSessionIdentifierLength, false); err != nil {
		return SessionSecuritySessionView{}, err
	}
	if err := validateSessionText("source ip", sourceIP, maxSessionSourceIPLength, true); err != nil {
		return SessionSecuritySessionView{}, err
	}
	if sourceIP != "" && net.ParseIP(sourceIP) == nil {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q source ip is invalid", id)
	}
	if err := validateSessionText("user agent", userAgent, maxSessionUserAgentLength, true); err != nil {
		return SessionSecuritySessionView{}, err
	}
	if input.CreatedAt.IsZero() || input.LastActivityAt.IsZero() || input.IdleExpiresAt.IsZero() || input.ExpiresAt.IsZero() {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q has incomplete temporal evidence", id)
	}
	createdAt := input.CreatedAt.UTC()
	lastActivityAt := input.LastActivityAt.UTC()
	idleExpiresAt := input.IdleExpiresAt.UTC()
	expiresAt := input.ExpiresAt.UTC()
	if createdAt.After(now) || lastActivityAt.After(now) {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q contains future activity evidence", id)
	}
	if lastActivityAt.Before(createdAt) {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q activity predates creation", id)
	}
	if !idleExpiresAt.After(lastActivityAt) {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q idle expiry is not after activity", id)
	}
	if !expiresAt.After(createdAt) {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q absolute expiry is not after creation", id)
	}
	if !now.Before(idleExpiresAt) || !now.Before(expiresAt) {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q is no longer active", id)
	}
	if idleExpiresAt.After(expiresAt) {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q idle expiry exceeds absolute expiry", id)
	}
	if expiresAt.Sub(createdAt) > time.Duration(policy.AbsoluteTTLSeconds)*time.Second {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q absolute expiry exceeds effective policy", id)
	}
	if idleExpiresAt.Sub(lastActivityAt) > time.Duration(policy.IdleTimeoutSeconds)*time.Second {
		return SessionSecuritySessionView{}, fmt.Errorf("session %q idle expiry exceeds effective policy", id)
	}

	risk := SessionActionRevokeSession
	if input.Current {
		risk = SessionActionLogsOutCurrent
	}
	return SessionSecuritySessionView{
		ID: id, CreatedAt: createdAt, LastActivityAt: lastActivityAt,
		IdleExpiresAt: idleExpiresAt, ExpiresAt: expiresAt,
		SourceIP: sourceIP, UserAgent: userAgent, Current: input.Current,
		IdleRemainingSeconds:     durationSecondsBounded(idleExpiresAt.Sub(now)),
		AbsoluteRemainingSeconds: durationSecondsBounded(expiresAt.Sub(now)),
		RevocationRisk:           risk,
	}, nil
}

func validateSessionText(field, value string, maximum int, emptyAllowed bool) error {
	if value == "" {
		if emptyAllowed {
			return nil
		}
		return fmt.Errorf("%s is required", field)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximum {
		return fmt.Errorf("%s exceeds maximum length or is invalid UTF-8", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", field)
		}
	}
	return nil
}

func durationSecondsBounded(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value / time.Second)
}
