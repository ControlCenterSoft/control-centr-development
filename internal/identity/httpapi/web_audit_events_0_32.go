package httpapi

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"control-center/internal/buildinfo"
	"control-center/internal/identity/audit"
)

type auditWebEventView struct {
	OccurredAt string
	Action     string
	Outcome    string
	ActorID    string
	SubjectID  string
}

type auditWebView struct {
	Version     string
	DisplayName string
	Username    string
	Limit       int
	Action      string
	Outcome     string
	ActorID     string
	SubjectID   string
	Events      []auditWebEventView
	NextURL     string
	ExportURL   string
}

var auditWebTemplate = template.Must(template.New("audit-search").Parse(`<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Control Center — Audit</title>
  <style>
    :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
    body { margin: 0; padding: 1rem; max-width: 1100px; }
    header, main, nav { margin-bottom: 1rem; }
    form { display: grid; gap: .75rem; grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr)); align-items: end; }
    label { display: grid; gap: .25rem; font-weight: 600; }
    input { min-height: 2.5rem; padding: 0 .6rem; }
    button, a.button { min-height: 2.5rem; padding: .55rem .8rem; display: inline-flex; align-items: center; justify-content: center; }
    table { border-collapse: collapse; width: 100%; }
    th, td { text-align: left; vertical-align: top; padding: .55rem; border-bottom: 1px solid currentColor; overflow-wrap: anywhere; }
    .muted { opacity: .75; }
    .actions { display: flex; gap: .75rem; flex-wrap: wrap; margin-top: .75rem; }
    @media (max-width: 760px) {
      table, thead, tbody, tr, th, td { display: block; }
      thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
      tr { margin-bottom: .8rem; border: 1px solid currentColor; padding: .4rem; }
      td { border: 0; }
      td::before { content: attr(data-label) ": "; font-weight: 700; }
    }
  </style>
</head>
<body>
<header>
  <h1>Audit</h1>
  <p class="muted">Control Center {{.Version}} · {{.DisplayName}} ({{.Username}})</p>
  <p>Поиск выполняется только по append-only Audit с текущим глобальным разрешением <code>audit.events.read</code>. Source IP и произвольные details на этом экране не отображаются.</p>
</header>
<main>
  <form method="get" action="/web/audit">
    <label>Action<input name="action" value="{{.Action}}" maxlength="192" autocomplete="off"></label>
    <label>Outcome<input name="outcome" value="{{.Outcome}}" maxlength="32" autocomplete="off"></label>
    <label>Actor ID<input name="actor_id" value="{{.ActorID}}" maxlength="128" autocomplete="off"></label>
    <label>Subject ID<input name="subject_id" value="{{.SubjectID}}" maxlength="256" autocomplete="off"></label>
    <label>Limit<input name="limit" type="number" min="1" max="100" value="{{.Limit}}"></label>
    <button type="submit">Найти</button>
  </form>

  <div class="actions">
    <a class="button" href="{{.ExportURL}}">Экспортировать эту bounded-выборку в CSV</a>
    {{if .NextURL}}<a class="button" href="{{.NextURL}}">Следующая страница</a>{{end}}
  </div>

  {{if .Events}}
  <table aria-label="События Audit">
    <thead><tr><th>Время</th><th>Action</th><th>Outcome</th><th>Actor</th><th>Subject</th></tr></thead>
    <tbody>
    {{range .Events}}
      <tr>
        <td data-label="Время">{{.OccurredAt}}</td>
        <td data-label="Action">{{.Action}}</td>
        <td data-label="Outcome">{{.Outcome}}</td>
        <td data-label="Actor">{{.ActorID}}</td>
        <td data-label="Subject">{{.SubjectID}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <p>По заданным точным фильтрам событий не найдено.</p>
  {{end}}
</main>
<nav><a href="/overview">Вернуться к обзору</a></nav>
</body>
</html>`))

// webAuditEvents is the bounded browser surface for the already-qualified Audit
// reader. It uses the same exact filters/cursor semantics as GET
// /api/v1/audit/events and records privileged read evidence before returning any
// event data. It deliberately omits SourceIP, Details, correlation IDs and hash
// chain material from the browser projection.
func (s *Server) webAuditEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || s.auditReader == nil {
		http.Error(w, "Audit events are temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	query, err := parseAuditQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid audit query", http.StatusBadRequest)
		return
	}
	page, err := s.auditReader.Read(r.Context(), query)
	if err != nil {
		_ = s.audit.Append(r.Context(), audit.Event{
			Action: "audit.events_list", Outcome: "failed", ActorID: principal.Identity.ID,
			SourceIP: remoteIP(r), Details: map[string]any{"reason": "read_failed", "surface": "web"},
		})
		http.Error(w, "Audit events are temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.audit.Append(r.Context(), audit.Event{
		Action: "audit.events_list", Outcome: "success", ActorID: principal.Identity.ID,
		SourceIP: remoteIP(r), Details: map[string]any{
			"surface": "web", "limit": query.Limit, "returned": len(page.Entries),
			"action": query.Action, "outcome": query.Outcome,
			"actor_id": query.ActorID, "subject_id": query.SubjectID,
		},
	}); err != nil {
		http.Error(w, "Audit read evidence could not be recorded", http.StatusServiceUnavailable)
		return
	}

	events := make([]auditWebEventView, 0, len(page.Entries))
	for _, entry := range page.Entries {
		event := entry.Event
		events = append(events, auditWebEventView{
			OccurredAt: event.OccurredAt.UTC().Format("2006-01-02 15:04:05Z07:00"),
			Action: event.Action, Outcome: event.Outcome,
			ActorID: event.ActorID, SubjectID: event.SubjectID,
		})
	}

	view := auditWebView{
		Version: buildinfo.Version, DisplayName: principal.Identity.DisplayName,
		Username: principal.Identity.Username, Limit: query.Limit,
		Action: query.Action, Outcome: query.Outcome, ActorID: query.ActorID,
		SubjectID: query.SubjectID, Events: events,
		ExportURL: auditWebURL("/api/v1/audit/events/export", query, query.BeforeSequenceID),
	}
	if page.HasMore && len(page.Entries) > 0 {
		view.NextURL = auditWebURL("/web/audit", query, page.Entries[len(page.Entries)-1].SequenceID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := auditWebTemplate.Execute(w, view); err != nil {
		return
	}
}

func auditWebURL(path string, query audit.Query, beforeSequenceID int64) string {
	values := url.Values{}
	values.Set("limit", strconv.Itoa(query.Limit))
	if query.Action != "" {
		values.Set("action", query.Action)
	}
	if query.Outcome != "" {
		values.Set("outcome", query.Outcome)
	}
	if query.ActorID != "" {
		values.Set("actor_id", query.ActorID)
	}
	if query.SubjectID != "" {
		values.Set("subject_id", query.SubjectID)
	}
	if beforeSequenceID > 0 {
		values.Set("cursor", encodeAuditCursor(beforeSequenceID))
	}
	encoded := values.Encode()
	if strings.TrimSpace(encoded) == "" {
		return path
	}
	return path + "?" + encoded
}
