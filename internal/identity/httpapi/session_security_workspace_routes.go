package httpapi

import (
	"errors"
	"html/template"
	"net/http"
	"strings"

	"control-center/internal/buildinfo"
	"control-center/internal/identity/audit"
	"control-center/internal/identity/auth"
)

// NewServerWithSessionSecurityWorkspace extends the cumulative identity server
// with the bounded 0.33 self-only session-security workspace. Existing session
// policy/inventory/revocation services remain the sole source of state and
// mutation authority.
func NewServerWithSessionSecurityWorkspace(
	authService *auth.Service,
	authorizer SelfAccessAuthorizer,
	log audit.Logger,
	config Config,
) (*Server, error) {
	server, err := NewServerWithAuditExport(authService, authorizer, log, config)
	if err != nil {
		return nil, err
	}
	server.registerSessionSecurityWorkspaceRoutes()
	return server, nil
}

func (s *Server) registerSessionSecurityWorkspaceRoutes() {
	s.mux.Handle(
		"GET /api/v1/identity/session-security",
		s.Authenticate(s.RequirePasswordCurrent(http.HandlerFunc(s.sessionSecurityWorkspace))),
	)
	s.mux.Handle(
		"GET /web/security/sessions",
		s.AuthenticateWeb(s.requirePasswordCurrentWeb(http.HandlerFunc(s.webSessionSecurityWorkspace))),
	)
	s.mux.Handle(
		"POST /web/security/sessions/revoke",
		s.AuthenticateWeb(s.requirePasswordCurrentWeb(http.HandlerFunc(s.webRevokeSession))),
	)
	s.mux.Handle(
		"POST /web/security/sessions/revoke-all",
		s.AuthenticateWeb(s.requirePasswordCurrentWeb(http.HandlerFunc(s.webRevokeAllSessions))),
	)
}

// requirePasswordCurrentWeb is the browser equivalent of
// RequirePasswordCurrent. It preserves navigation semantics instead of
// returning a JSON API error for the mandatory first-login boundary.
func (s *Server) requirePasswordCurrentWeb(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if principal.PasswordChangeRequired {
			http.Redirect(w, r, "/password/change", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var sessionSecurityWorkspaceTemplate = template.Must(template.New("session-security-workspace").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Control Center — Безопасность сессий</title><style>
body{font-family:system-ui;max-width:72rem;margin:4vh auto;padding:1rem}table{border-collapse:collapse;width:100%}th,td{border-bottom:1px solid #ddd;padding:.55rem;text-align:left;vertical-align:top}.card{border:1px solid #ccc;border-radius:.7rem;padding:1rem;margin:1rem 0}.muted{opacity:.75}.risk{font-weight:600}button{padding:.55rem .8rem}form{margin:.25rem 0}code{overflow-wrap:anywhere}
</style></head><body><main>
<h1>Безопасность сессий</h1>
<p>Пользователь: {{.Workspace.Identity.DisplayName}} (<code>{{.Workspace.Identity.Username}}</code>)</p>
<section class="card" aria-labelledby="policy"><h2 id="policy">Политика сессий</h2>
<p>Абсолютный срок: {{.Workspace.Policy.AbsoluteTTLSeconds}} секунд. Таймаут бездействия: {{.Workspace.Policy.IdleTimeoutSeconds}} секунд.</p>
<p class="muted">Активность обновляет idle deadline: {{.Workspace.Policy.ActivityRefreshesIdleDeadline}}. Абсолютный срок активностью не продлевается.</p></section>
<section class="card" aria-labelledby="sessions"><h2 id="sessions">Активные сессии</h2>
<table><thead><tr><th>Сессия</th><th>Источник</th><th>Время</th><th>Действие</th></tr></thead><tbody>
{{range .Workspace.Sessions}}<tr><td><code>{{.ID}}</code>{{if .Current}}<br><strong>Текущая</strong>{{end}}</td>
<td>{{if .SourceIP}}IP: <code>{{.SourceIP}}</code><br>{{end}}{{if .UserAgent}}<span class="muted">{{.UserAgent}}</span>{{end}}</td>
<td>Создана: {{.CreatedAt}}<br>Активность: {{.LastActivityAt}}<br>Idle осталось: {{.IdleRemainingSeconds}} сек.<br>Абсолютно осталось: {{.AbsoluteRemainingSeconds}} сек.</td>
<td><span class="risk">{{if .Current}}Отзыв завершит текущий вход{{else}}Отозвать эту сессию{{end}}</span>
<form method="post" action="/web/security/sessions/revoke"><input type="hidden" name="session_id" value="{{.ID}}"><button type="submit">Отозвать</button></form></td></tr>{{end}}
</tbody></table>
<form method="post" action="/web/security/sessions/revoke-all"><button type="submit">Отозвать все мои сессии</button></form>
</section>
<p><a href="/overview">Вернуться к обзору</a></p>
</main><footer class="muted">Control Center {{.Version}}</footer></body></html>`))

type sessionSecurityWorkspacePage struct {
	Version   string
	Workspace interface {
	}
}

func (s *Server) webSessionSecurityWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	workspace, err := s.buildSessionSecurityWorkspace(r.Context(), principal, remoteIP(r), nowUTC())
	if err != nil {
		http.Error(w, "Session security workspace is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := sessionSecurityWorkspaceTemplate.Execute(w, struct {
		Version   string
		Workspace any
	}{Version: buildinfo.Version, Workspace: workspace}); err != nil {
		return
	}
}

func (s *Server) webRevokeSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	sessionID := strings.TrimSpace(r.FormValue("session_id"))
	if sessionID == "" || len(sessionID) > 128 || strings.ContainsAny(sessionID, "\r\n\x00") {
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}
	result, err := s.auth.RevokeSession(r.Context(), auth.RevokeSessionInput{
		UserID: principal.Identity.ID, SessionID: sessionID,
		CurrentSessionID: principal.Session.ID, SourceIP: remoteIP(r),
	})
	if result.CurrentSessionRevoked {
		s.clearSessionCookie(w)
	}
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			s.clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		case errors.Is(err, auth.ErrNotFound):
			http.Error(w, "Active session was not found", http.StatusNotFound)
		default:
			http.Error(w, "Session revocation is temporarily unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	if result.CurrentSessionRevoked {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/web/security/sessions", http.StatusSeeOther)
}

func (s *Server) webRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if _, err := s.auth.RevokeAllSessions(r.Context(), auth.RevokeAllSessionsInput{
		UserID: principal.Identity.ID, SourceIP: remoteIP(r),
	}); err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			s.clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Error(w, "Session revocation is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

var nowUTC = func() time.Time { return time.Now().UTC() }
