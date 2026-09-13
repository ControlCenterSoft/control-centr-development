package httpapi

import (
	"context"
	"fmt"
	"time"

	"control-center/internal/identity/auth"
	productui "control-center/internal/ui"
)

// buildSessionSecurityWorkspace adapts the existing self-only identity/session
// services into the bounded 0.33 UI projection. It never accepts an actor ID
// from request input and it does not authorize session mutation.
func (s *Server) buildSessionSecurityWorkspace(ctx context.Context, principal Principal, sourceIP string, now time.Time) (productui.SessionSecurityWorkspace, error) {
	if s == nil || s.auth == nil {
		return productui.SessionSecurityWorkspace{}, fmt.Errorf("session security workspace unavailable")
	}
	policy, err := s.auth.SessionSecurityPolicy(ctx, auth.SessionSecurityPolicyInput{
		UserID: principal.Identity.ID, SourceIP: sourceIP,
	})
	if err != nil {
		return productui.SessionSecurityWorkspace{}, fmt.Errorf("read session policy: %w", err)
	}
	sessions, err := s.auth.ListSessions(ctx, auth.ListSessionsInput{
		UserID: principal.Identity.ID, CurrentSessionID: principal.Session.ID, SourceIP: sourceIP,
	})
	if err != nil {
		return productui.SessionSecurityWorkspace{}, fmt.Errorf("read session inventory: %w", err)
	}

	observations := make([]productui.SessionSecurityInput, 0, len(sessions))
	for _, session := range sessions {
		observations = append(observations, productui.SessionSecurityInput{
			ID: session.ID, CreatedAt: session.CreatedAt, LastActivityAt: session.LastActivityAt,
			IdleExpiresAt: session.IdleExpiresAt, ExpiresAt: session.ExpiresAt,
			SourceIP: session.SourceIP, UserAgent: session.UserAgent, Current: session.Current,
		})
	}
	return productui.BuildSessionSecurityWorkspace(productui.SessionSecurityWorkspaceInput{
		Loaded:                 true,
		ActorID:                principal.Identity.ID,
		Username:               principal.Identity.Username,
		DisplayName:            principal.Identity.DisplayName,
		CurrentSessionID:       principal.Session.ID,
		PasswordChangeRequired: principal.PasswordChangeRequired,
		Policy: productui.SessionSecurityPolicyInput{
			AbsoluteTTLSeconds:            policy.AbsoluteTTLSeconds,
			IdleTimeoutSeconds:            policy.IdleTimeoutSeconds,
			ActivityRefreshesIdleDeadline: policy.ActivityRefreshesIdleDeadline,
			ActivityExtendsAbsoluteExpiry: policy.ActivityExtendsAbsoluteExpiry,
		},
		Sessions: observations,
		Now:      now.UTC(),
	})
}
