package httpapi

import (
	"net/http"
	"strings"
)

// AuthenticatedActorID resolves the server-authenticated Control Center
// identity from request context. It is intended for application handlers that
// are composed behind Server.AuthenticatedCurrentPassword; it never accepts an
// actor identifier from headers, query parameters, or request payloads.
func AuthenticatedActorID(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || principal.PasswordChangeRequired {
		return "", false
	}
	actorID := strings.TrimSpace(principal.Identity.ID)
	if actorID == "" {
		return "", false
	}
	return actorID, true
}

// AuthenticatedCurrentPassword composes an external application handler with
// the same session authentication and mandatory first-login password-change
// gate used by built-in identity routes. RBAC remains the downstream handler's
// responsibility because its target scope can be resource-specific.
func (s *Server) AuthenticatedCurrentPassword(next http.Handler) http.Handler {
	if s == nil || next == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusServiceUnavailable, "route_unavailable", "Route is unavailable")
		})
	}
	return s.Authenticate(s.RequirePasswordCurrent(next))
}
