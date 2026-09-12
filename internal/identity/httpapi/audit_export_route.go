package httpapi

import (
	"net/http"

	"control-center/internal/identity/rbac"
)

// AuditEventsExportHandler exposes the qualified bounded Audit CSV export
// through the same authentication, mandatory password-change and global
// audit.events.read authorization boundaries as the built-in Audit read API.
//
// The returned handler does not widen Audit access and preserves the
// fail-closed evidence-before-bytes behavior implemented by auditEventsExport.
func (s *Server) AuditEventsExportHandler() http.Handler {
	if s == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Audit export is unavailable", http.StatusServiceUnavailable)
		})
	}
	return s.Authenticate(
		s.Require(rbac.PermissionAuditRead, rbac.GlobalScope())(
			http.HandlerFunc(s.auditEventsExport),
		),
	)
}
