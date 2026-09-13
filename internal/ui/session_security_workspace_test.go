package ui

import (
	"strings"
	"testing"
	"time"
)

func validSessionWorkspaceInput() SessionSecurityWorkspaceInput {
	now := time.Date(2026, 9, 13, 1, 30, 0, 0, time.UTC)
	return SessionSecurityWorkspaceInput{
		Loaded:           true,
		ActorID:          "viewer-1",
		Username:         "viewer",
		DisplayName:      "Viewer",
		CurrentSessionID: "session-current",
		Policy: SessionSecurityPolicyInput{
			AbsoluteTTLSeconds:            8 * 60 * 60,
			IdleTimeoutSeconds:            2 * 60 * 60,
			ActivityRefreshesIdleDeadline: true,
			ActivityExtendsAbsoluteExpiry: false,
		},
		Sessions: []SessionSecurityInput{
			{
				ID:             "session-other",
				CreatedAt:      now.Add(-3 * time.Hour),
				LastActivityAt: now.Add(-20 * time.Minute),
				IdleExpiresAt:  now.Add(100 * time.Minute),
				ExpiresAt:      now.Add(5 * time.Hour),
				SourceIP:       "192.0.2.20",
				UserAgent:      "Example Browser 2",
			},
			{
				ID:             "session-current",
				CreatedAt:      now.Add(-time.Hour),
				LastActivityAt: now.Add(-5 * time.Minute),
				IdleExpiresAt:  now.Add(115 * time.Minute),
				ExpiresAt:      now.Add(7 * time.Hour),
				SourceIP:       "192.0.2.10",
				UserAgent:      "Example Browser 1",
				Current:        true,
			},
		},
		Now: now,
	}
}

func TestBuildSessionSecurityWorkspaceNormalizesSelfOnlyInventory(t *testing.T) {
	view, err := BuildSessionSecurityWorkspace(validSessionWorkspaceInput())
	if err != nil {
		t.Fatalf("BuildSessionSecurityWorkspace() error = %v", err)
	}
	if view.Schema != SessionSecurityWorkspaceSchemaV1 || view.DataState != SessionSecurityDataLoaded {
		t.Fatalf("unexpected identity: %#v", view)
	}
	if !view.SelfOnly || view.MutationAuthorized {
		t.Fatalf("workspace authority boundary weakened: %#v", view)
	}
	if view.Identity == nil || view.Identity.ID != "viewer-1" || view.Identity.Username != "viewer" {
		t.Fatalf("identity = %#v", view.Identity)
	}
	if view.Policy == nil || view.Policy.AbsoluteTTLSeconds != 8*60*60 || view.Policy.IdleTimeoutSeconds != 2*60*60 {
		t.Fatalf("policy = %#v", view.Policy)
	}
	if len(view.Sessions) != 2 || !view.Sessions[0].Current || view.Sessions[0].ID != "session-current" {
		t.Fatalf("session ordering/current marker = %#v", view.Sessions)
	}
	if view.Sessions[0].RevocationRisk != SessionActionLogsOutCurrent || view.Sessions[1].RevocationRisk != SessionActionRevokeSession {
		t.Fatalf("revocation risk = %#v", view.Sessions)
	}
	if view.Sessions[0].IdleRemainingSeconds != 115*60 || view.Sessions[0].AbsoluteRemainingSeconds != 7*60*60 {
		t.Fatalf("remaining lifetime = %#v", view.Sessions[0])
	}
}

func TestBuildSessionSecurityWorkspaceUnavailableIsEmptyAndNonAuthorizing(t *testing.T) {
	view, err := BuildSessionSecurityWorkspace(SessionSecurityWorkspaceInput{
		Loaded: false,
		Now:    time.Date(2026, 9, 13, 1, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildSessionSecurityWorkspace() error = %v", err)
	}
	if view.DataState != SessionSecurityDataUnavailable || !view.SelfOnly || view.MutationAuthorized || view.Identity != nil || view.Policy != nil || len(view.Sessions) != 0 {
		t.Fatalf("unavailable projection = %#v", view)
	}
}

func TestBuildSessionSecurityWorkspaceRejectsPartialUnavailableEvidence(t *testing.T) {
	input := validSessionWorkspaceInput()
	input.Loaded = false
	if _, err := BuildSessionSecurityWorkspace(input); err == nil {
		t.Fatal("expected partial unavailable evidence to be rejected")
	}
}

func TestBuildSessionSecurityWorkspaceRejectsFirstLoginBoundaryBypass(t *testing.T) {
	input := validSessionWorkspaceInput()
	input.PasswordChangeRequired = true
	if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "password change") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildSessionSecurityWorkspaceRejectsCurrentSessionMismatch(t *testing.T) {
	input := validSessionWorkspaceInput()
	input.CurrentSessionID = "another-session"
	if _, err := BuildSessionSecurityWorkspace(input); err == nil {
		t.Fatal("expected current session mismatch to fail closed")
	}
}

func TestBuildSessionSecurityWorkspaceRejectsDuplicateSessions(t *testing.T) {
	input := validSessionWorkspaceInput()
	duplicate := input.Sessions[1]
	duplicate.Current = false
	input.Sessions = append(input.Sessions, duplicate)
	if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "duplicate session") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildSessionSecurityWorkspaceRejectsExpiredOrFutureEvidence(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		input := validSessionWorkspaceInput()
		input.Sessions[0].IdleExpiresAt = input.Now
		if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "no longer active") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("future-activity", func(t *testing.T) {
		input := validSessionWorkspaceInput()
		input.Sessions[0].LastActivityAt = input.Now.Add(time.Second)
		if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "future activity") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestBuildSessionSecurityWorkspaceRejectsInvalidPolicy(t *testing.T) {
	t.Run("idle-exceeds-absolute", func(t *testing.T) {
		input := validSessionWorkspaceInput()
		input.Policy.IdleTimeoutSeconds = input.Policy.AbsoluteTTLSeconds + 1
		if _, err := BuildSessionSecurityWorkspace(input); err == nil {
			t.Fatal("expected invalid session policy to be rejected")
		}
	})
	t.Run("activity-extends-absolute", func(t *testing.T) {
		input := validSessionWorkspaceInput()
		input.Policy.ActivityExtendsAbsoluteExpiry = true
		if _, err := BuildSessionSecurityWorkspace(input); err == nil {
			t.Fatal("expected absolute-expiry extension to be rejected")
		}
	})
}

func TestBuildSessionSecurityWorkspaceRejectsPolicyInconsistentSessionDeadlines(t *testing.T) {
	t.Run("absolute", func(t *testing.T) {
		input := validSessionWorkspaceInput()
		input.Sessions[0].ExpiresAt = input.Sessions[0].CreatedAt.Add(time.Duration(input.Policy.AbsoluteTTLSeconds+1) * time.Second)
		if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "absolute expiry exceeds effective policy") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("idle", func(t *testing.T) {
		input := validSessionWorkspaceInput()
		input.Sessions[0].IdleExpiresAt = input.Sessions[0].LastActivityAt.Add(time.Duration(input.Policy.IdleTimeoutSeconds+1) * time.Second)
		if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "idle expiry exceeds effective policy") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestBuildSessionSecurityWorkspaceRejectsInvalidSourceIP(t *testing.T) {
	input := validSessionWorkspaceInput()
	input.Sessions[0].SourceIP = "not-an-ip"
	if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "source ip is invalid") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildSessionSecurityWorkspaceRejectsControlCharacters(t *testing.T) {
	input := validSessionWorkspaceInput()
	input.Sessions[0].UserAgent = "Browser\nInjected"
	if _, err := BuildSessionSecurityWorkspace(input); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("error = %v", err)
	}
}
