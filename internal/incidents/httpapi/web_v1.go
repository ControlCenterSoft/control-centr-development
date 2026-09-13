package httpapi

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"control-center/internal/incidents"
)

// NewWithBrowser layers a read-only operator browser surface over the existing
// incident API without changing API contracts or mutation semantics. The same
// Service and ActorResolver are used for both surfaces, so scoped RBAC remains
// authoritative and cannot be bypassed by browser rendering.
func NewWithBrowser(service Service, actor ActorResolver, options ...Option) http.Handler {
	api := New(service, actor, options...)
	web := &server{service: service, actor: actor, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(web)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/incidents/ui", web.handleWebList)
	mux.HandleFunc("GET /api/v1/incidents/ui/{incidentID}", web.handleWebGet)
	mux.Handle("/", api)
	return mux
}

type incidentWebListData struct {
	Items        []incidents.Incident
	Limit        int
	Status       string
	Severity     string
	ScopeID      string
	ResourceKind string
	ResourceID   string
	NextURL      string
}

type incidentWebDetailData struct {
	Incident incidents.Incident
}

var incidentListTemplate = template.Must(template.New("incident-list").Funcs(template.FuncMap{
	"incidentTime": incidentWebTime,
	"incidentURL": func(id string) string {
		return "/api/v1/incidents/ui/" + url.PathEscape(id)
	},
}).Parse(`<!doctype html>
<html lang="ru" data-locale="ru-RU"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Control Center — Incidents</title><style>
:root{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color-scheme:light dark}*{box-sizing:border-box}body{margin:0;background:Canvas;color:CanvasText}.content{max-width:92rem;margin:0 auto;padding:1.25rem}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:.8rem;padding:1rem;border:1px solid currentColor;border-radius:.8rem}label{display:grid;gap:.3rem;font-weight:600}input,select,button{font:inherit;min-height:2.6rem;padding:.5rem;border:1px solid currentColor;border-radius:.5rem;background:Canvas;color:CanvasText}.table-wrap{overflow:auto;margin-top:1.2rem;border:1px solid currentColor;border-radius:.8rem}table{width:100%;border-collapse:collapse;min-width:66rem}th,td{text-align:left;vertical-align:top;padding:.65rem;border-bottom:1px solid currentColor;overflow-wrap:anywhere}.badge{display:inline-flex;border:1px solid currentColor;border-radius:999px;padding:.25rem .55rem}.muted{opacity:.75}.pager{display:flex;gap:1rem;justify-content:space-between;margin-top:1rem;flex-wrap:wrap}@media(max-width:760px){.filters{grid-template-columns:1fr}.content{padding:.8rem}}
</style></head><body><main class="content"><h1>Инциденты</h1>
<p class="muted">Read-only operator view. Доступ и scope проверяются серверным Incident RBAC; неизвестное или запрещённое состояние не подменяется пустым успешным результатом.</p>
<form class="filters" method="get" action="/api/v1/incidents/ui">
<label>Status<select name="status"><option value="">Все</option><option value="open" {{if eq .Status "open"}}selected{{end}}>open</option><option value="acknowledged" {{if eq .Status "acknowledged"}}selected{{end}}>acknowledged</option><option value="resolved" {{if eq .Status "resolved"}}selected{{end}}>resolved</option></select></label>
<label>Severity<select name="severity"><option value="">Все</option><option value="critical" {{if eq .Severity "critical"}}selected{{end}}>critical</option><option value="warning" {{if eq .Severity "warning"}}selected{{end}}>warning</option><option value="info" {{if eq .Severity "info"}}selected{{end}}>info</option></select></label>
<label>Scope ID<input name="scope_id" value="{{.ScopeID}}" autocomplete="off"></label>
<label>Resource kind<input name="resource_kind" value="{{.ResourceKind}}" autocomplete="off"></label>
<label>Resource ID<input name="resource_id" value="{{.ResourceID}}" autocomplete="off"></label>
<label>Limit<input name="limit" type="number" min="1" max="100" value="{{.Limit}}"></label>
<button type="submit">Применить</button></form>
{{if .Items}}<div class="table-wrap"><table aria-label="Incidents"><thead><tr><th>Incident</th><th>Severity</th><th>Status</th><th>Scope</th><th>Started</th><th>Last observed</th></tr></thead><tbody>{{range .Items}}<tr><td><a href="{{incidentURL .ObjectID}}">{{.Title}}</a><br><span class="muted">{{.ObjectID}}</span></td><td><span class="badge">{{.Severity}}</span></td><td>{{.Status}}</td><td>{{.ScopeID}}</td><td>{{incidentTime .StartedAt}}</td><td>{{incidentTime .LastObservedAt}}</td></tr>{{end}}</tbody></table></div>{{else}}<p role="status">Инциденты по текущим точным фильтрам не найдены.</p>{{end}}
<div class="pager">{{if .NextURL}}<a href="{{.NextURL}}">Следующая страница →</a>{{else}}<span class="muted">Следующей страницы нет</span>{{end}}<a href="/overview">Overview</a></div>
</main></body></html>`))

var incidentDetailTemplate = template.Must(template.New("incident-detail").Funcs(template.FuncMap{
	"incidentTime": incidentWebTime,
}).Parse(`<!doctype html>
<html lang="ru" data-locale="ru-RU"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Control Center — Incident</title><style>
:root{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color-scheme:light dark}*{box-sizing:border-box}body{margin:0;background:Canvas;color:CanvasText}.content{max-width:80rem;margin:0 auto;padding:1.25rem}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.card{border:1px solid currentColor;border-radius:.8rem;padding:1rem}.muted{opacity:.75}.badge{display:inline-flex;border:1px solid currentColor;border-radius:999px;padding:.25rem .55rem}ul{padding-left:1.2rem}li{margin:.35rem 0;overflow-wrap:anywhere}@media(max-width:760px){.grid{grid-template-columns:1fr}.content{padding:.8rem}}
</style></head><body><main class="content"><p><a href="/api/v1/incidents/ui">← Инциденты</a></p><h1>{{.Incident.Title}}</h1><p><span class="badge">{{.Incident.Severity}}</span> <span class="badge">{{.Incident.Status}}</span></p>
<div class="grid"><section class="card"><h2>Состояние</h2><p>ID: {{.Incident.ObjectID}}</p><p>Scope: {{.Incident.ScopeID}}</p><p>Generation: {{.Incident.Generation}}</p><p>Started: {{incidentTime .Incident.StartedAt}}</p><p>Last observed: {{incidentTime .Incident.LastObservedAt}}</p>{{if .Incident.Acknowledgement}}<p>Acknowledged by {{.Incident.Acknowledgement.ActorID}} at {{incidentTime .Incident.Acknowledgement.At}}</p>{{end}}{{if .Incident.Runbook}}<p>Runbook: {{.Incident.Runbook.ID}} @ {{.Incident.Runbook.Revision}}</p>{{end}}</section>
<section class="card"><h2>Affected resources</h2><ul>{{range .Incident.AffectedResources}}<li>{{.Kind}} / {{.ID}} ({{.ScopeID}})</li>{{end}}</ul></section>
<section class="card"><h2>Signals</h2><ul>{{range .Incident.Signals}}<li><strong>{{.Kind}}</strong> · {{.Source}} · {{incidentTime .ObservedAt}}<br>{{.Summary}}</li>{{end}}</ul></section>
<section class="card"><h2>Evidence refs</h2>{{if .Incident.Evidence}}<ul>{{range .Incident.Evidence}}<li>{{.Kind}} / {{.ID}} · {{incidentTime .Collected}} · redaction={{.Redaction}}</li>{{end}}</ul>{{else}}<p class="muted">Нет incident-level evidence references.</p>{{end}}</section>
<section class="card"><h2>Timeline</h2><ul>{{range .Incident.Timeline}}<li><strong>{{.Kind}}</strong> · {{incidentTime .At}}{{if .ActorID}} · actor={{.ActorID}}{{end}}{{if .Summary}}<br>{{.Summary}}{{end}}</li>{{end}}</ul></section></div>
<p class="muted">Этот экран read-only и не предоставляет acknowledge/resolve/mutation authority.</p></main></body></html>`))

func (s *server) handleWebList(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.resolveActor(w, r)
	if !ok {
		return
	}
	query, err := parseListQuery(r)
	if err != nil {
		http.Error(w, "Некорректные фильтры incidents", http.StatusBadRequest)
		return
	}
	page, err := s.service.List(r.Context(), actor, query)
	if err != nil {
		writeIncidentWebError(w, err)
		return
	}
	for index := range page.Items {
		if err := page.Items[index].Validate(); err != nil {
			http.Error(w, "Incident evidence unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	data := incidentWebListData{
		Items: page.Items, Limit: query.Limit, ScopeID: query.ScopeID,
		ResourceKind: query.ResourceKind, ResourceID: query.ResourceID,
	}
	if len(query.Statuses) == 1 {
		data.Status = string(query.Statuses[0])
	}
	if len(query.Severities) == 1 {
		data.Severity = string(query.Severities[0])
	}
	if page.HasMore && page.Next != nil {
		data.NextURL = buildIncidentWebURL(query, page.Next)
	}
	writeIncidentHTML(w, incidentListTemplate, data)
}

func (s *server) handleWebGet(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.resolveActor(w, r)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "Query parameters are not supported", http.StatusBadRequest)
		return
	}
	incident, err := s.service.Get(r.Context(), actor, r.PathValue("incidentID"))
	if err != nil {
		writeIncidentWebError(w, err)
		return
	}
	if err := incident.Validate(); err != nil {
		http.Error(w, "Incident evidence unavailable", http.StatusServiceUnavailable)
		return
	}
	writeIncidentHTML(w, incidentDetailTemplate, incidentWebDetailData{Incident: incident})
}

func buildIncidentWebURL(query incidents.ListQuery, cursor *incidents.ListCursor) string {
	values := url.Values{}
	values.Set("limit", strconv.Itoa(query.Limit))
	for _, status := range query.Statuses {
		values.Add("status", string(status))
	}
	for _, severity := range query.Severities {
		values.Add("severity", string(severity))
	}
	if query.ScopeID != "" {
		values.Set("scope_id", query.ScopeID)
	}
	if query.ResourceKind != "" {
		values.Set("resource_kind", query.ResourceKind)
	}
	if query.ResourceID != "" {
		values.Set("resource_id", query.ResourceID)
	}
	if query.StartedFrom != nil {
		values.Set("started_from", query.StartedFrom.UTC().Format(time.RFC3339))
	}
	if query.StartedBefore != nil {
		values.Set("started_before", query.StartedBefore.UTC().Format(time.RFC3339))
	}
	if cursor != nil {
		values.Set("before_started_at", cursor.StartedAt.UTC().Format(time.RFC3339))
		values.Set("before_object_id", cursor.ObjectID)
	}
	return "/api/v1/incidents/ui?" + values.Encode()
}

func writeIncidentHTML(w http.ResponseWriter, tmpl *template.Template, data any) {
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		http.Error(w, "Incident UI unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func writeIncidentWebError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "Incident view unavailable"
	switch {
	case errors.Is(err, incidents.ErrOperatorIdentityRequired):
		status, message = http.StatusUnauthorized, "Authentication is required"
	case errors.Is(err, incidents.ErrNotFound):
		status, message = http.StatusNotFound, "Incident was not found"
	case errors.Is(err, incidents.ErrOperatorAccessDenied):
		status, message = http.StatusForbidden, "Incident access is denied"
	case errors.Is(err, incidents.ErrOperatorDependencyUnavailable):
		status, message = http.StatusServiceUnavailable, "Incident service is unavailable"
	}
	http.Error(w, message, status)
}

func incidentWebTime(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.UTC().Format(time.RFC3339)
}
