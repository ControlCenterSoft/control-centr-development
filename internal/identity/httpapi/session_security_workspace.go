package httpapi

import (
	"net/http"
	"time"
)

// sessionSecurityWorkspace serves the bounded self-only 0.33 session-security
// projection. Route registration is intentionally separate so qualification
// can bind it to the existing Authenticate + RequirePasswordCurrent chain
// without introducing a parallel authorization path.
func (s *Server) sessionSecurityWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_required", "Authentication is required")
		return
	}
	workspace, err := s.buildSessionSecurityWorkspace(r.Context(), principal, remoteIP(r), time.Now().UTC())
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "session_security_workspace_unavailable", "Session security workspace is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": workspace})
}
