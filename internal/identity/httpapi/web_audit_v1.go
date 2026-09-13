package httpapi

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"control-center/internal/buildinfo"
	"control-center/internal/identity/audit"
)

type auditWebEvent struct {
	SequenceID    int64
	OccurredAt    time.Time
	Action        string
	Outcome       string
	ActorID       string
	SubjectID     string
	CorrelationID string
	Hash          string
}

type auditWebPageData struct {
	Version     string
	DisplayName string
	Username    string
	Action      string
	Outcome     string
	ActorID     string
	SubjectID   string
	Limit       int
	Events      []auditWebEvent
	NextURL     string
	ExportURL   string
}

var auditWebTemplate = template.Must(template.New("audit-v1").Funcs(template.FuncMap{
	"auditTime": func(value time.Time) string {
		if value.IsZero() {
			return "нет времени"
		}
		return value.UTC().Format(time.RFC3339)
	},
}).Parse(`<!doctype html>
<html lang="ru" data-locale="ru-RU">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Control Center — Audit</title>
<style>
:root{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color-scheme:light dark;--s1:.35rem;--s2:.65rem;--s3:1rem;--s4:1.5rem;--radius:.8rem;--border:1px solid currentColor}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:Canvas;color:CanvasText}a{color:inherit}.skip{position:absolute;left:-9999px}.skip:focus{left:var(--s3);top:var(--s3);z-index:10;background:Canvas;padding:var(--s2);border:var(--border)}.topbar{display:flex;gap:var(--s3);align-items:center;justify-content:space-between;padding:var(--s3) var(--s4);border-bottom:var(--border)}.brand{display:grid;gap:var(--s1)}.muted,.eyebrow{font-size:.85rem;opacity:.78}.row{display:flex;gap:var(--s2);align-items:center;flex-wrap:wrap}.badge{display:inline-flex;border:var(--border);border-radius:999px;padding:.35rem .65rem;font-size:.85rem}.content{padding:var(--s4);max-width:92rem;width:100%;margin:0 auto}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--s3);padding:var(--s3);border:var(--border);border-radius:var(--radius)}label{display:grid;gap:var(--s1);font-weight:600}input,select,button{font:inherit;padding:.55rem .65rem;border:var(--border);border-radius:.55rem;background:Canvas;color:CanvasText;min-width:0}.actions{display:flex;gap:var(--s2);align-items:end;flex-wrap:wrap}.table-wrap{overflow:auto;margin-top:var(--s4);border:var(--border);border-radius:var(--radius)}table{width:100%;border-collapse:collapse;min-width:72rem}th,td{text-align:left;vertical-align:top;padding:.7rem;border-bottom:var(--border);overflow-wrap:anywhere}th{font-size:.85rem}.empty{padding:var(--s3)}.pager,.footer{margin-top:var(--s4);display:flex;gap:var(--s3);justify-content:space-between;align-items:center;flex-wrap:wrap}.hash{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:.8rem}@media(max-width:850px){.filters{grid-template-columns:1fr}.topbar{align-items:flex-start;flex-direction:column}.content{padding:var(--s3)}}
</style>
</head>
<body>
<a class="skip" href="#main-content">К основному содержимому</a>
<header class="topbar"><div class="brand"><span class="eyebrow">Control Center</span><strong>Audit / Поиск событий</strong></div><div class="row"><span class="badge">Версия {{.Version}}</span><span class="badge">Пользователь: {{.DisplayName}}</span></div></header>
<main class="content" id="main-content">
<h1>Audit events</h1>
<p class="muted">Read-only просмотр append-only Audit. Поиск использует только точные серверные фильтры. Source IP и Details в browser UI не отображаются; экспорт остаётся ограниченным и permission-gated.</p>
<form class="filters" method="get" action="/web/audit">
<label>Action<input name="action" value="{{.Action}}" maxlength="192" autocomplete="off"></label>
<label>Outcome<input name="outcome" value="{{.Outcome}}" maxlength="32" autocomplete="off"></label>
<label>Actor ID<input name="actor_id" value="{{.ActorID}}" maxlength="128" autocomplete="off"></label>
<label>Subject ID<input name="subject_id" value="{{.SubjectID}}" maxlength="256" autocomplete="off"></label>
<label>Limit<select name="limit"><option{{if eq .Limit 25}} selected{{end}}>25</option><option{{if eq .Limit 50}} selected{{end}}>50</option><option{{if eq .Limit 100}} selected{{end}}>100</option></select></label>
<div class="actions"><button type="submit">Применить фильтры</button><a href="/web/audit">Сбросить</a><a href="{{.ExportURL}}">Скачать текущую bounded CSV-страницу</a></div>
</form>
{{if .Events}}
<div class="table-wrap"><table><thead><tr><th>Seq</th><th>Время</th><th>Action</th><th>Outcome</th><th>Actor</th><th>Subject</th><th>Correlation</th><th>Hash</th></tr></thead><tbody>{{range .Events}}<tr><td>{{.SequenceID}}</td><td>{{auditTime .OccurredAt}}</td><td>{{.Action}}</td><td>{{.Outcome}}</td><td>{{.ActorID}}</td><td>{{.SubjectID}}</td><td>{{.CorrelationID}}</td><td class="hash">{{.Hash}}</td></tr>{{end}}</tbody></table></div>
{{else}}<section class="empty" role="status"><strong>События по текущим точным фильтрам не найдены.</strong></section>{{end}}
<div class="pager">{{if .NextURL}}<a href="{{.NextURL}}">Следующая страница →</a>{{else}}<span class="muted">Следующей страницы нет</span>{{end}}<a href="/reports/audit">Открыть Audit report / Evidence</a></div>
<footer class="footer"><span class="muted">Учётная запись: {{.Username}} · Язык: русский (ru-RU)</span><form method="post" action="/web/logout"><button type="submit">Выйти</button></form></footer>
</main>
</body>
</html>`))

func (s *Server) webAuditEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || s.auditReader == nil {
		http.Error(w, "Audit временно недоступен", http.StatusServiceUnavailable)
		return
	}
	query, err := parseAuditQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Некорректные фильтры Audit", http.StatusBadRequest)
		return
	}
	page, err := s.auditReader.Read(r.Context(), query)
	if err != nil {
		_ = s.audit.Append(r.Context(), audit.Event{
			Action: "audit.events_web", Outcome: "failed", ActorID: principal.Identity.ID,
			SourceIP: remoteIP(r), Details: map[string]any{"reason": "read_failed"},
		})
		http.Error(w, "Audit временно недоступен", http.StatusServiceUnavailable)
		return
	}

	events := make([]auditWebEvent, 0, len(page.Entries))
	for _, entry := range page.Entries {
		event := entry.Event
		events = append(events, auditWebEvent{
			SequenceID: entry.SequenceID, OccurredAt: event.OccurredAt,
			Action: event.Action, Outcome: event.Outcome, ActorID: event.ActorID,
			SubjectID: event.SubjectID, CorrelationID: event.CorrelationID, Hash: event.Hash,
		})
	}
	nextCursor := ""
	if page.HasMore && len(page.Entries) > 0 {
		nextCursor = encodeAuditCursor(page.Entries[len(page.Entries)-1].SequenceID)
	}
	if err := s.audit.Append(r.Context(), audit.Event{
		Action: "audit.events_web", Outcome: "success", ActorID: principal.Identity.ID,
		SourceIP: remoteIP(r), Details: map[string]any{
			"limit": query.Limit, "returned": len(events), "action": query.Action,
			"outcome": query.Outcome, "actor_id": query.ActorID, "subject_id": query.SubjectID,
			"before_sequence_id": query.BeforeSequenceID,
		},
	}); err != nil {
		http.Error(w, "Audit evidence не удалось зафиксировать", http.StatusServiceUnavailable)
		return
	}

	currentCursor := ""
	if query.BeforeSequenceID > 0 {
		currentCursor = encodeAuditCursor(query.BeforeSequenceID)
	}
	data := auditWebPageData{
		Version: buildinfo.Version, DisplayName: principal.Identity.DisplayName, Username: principal.Identity.Username,
		Action: query.Action, Outcome: query.Outcome, ActorID: query.ActorID, SubjectID: query.SubjectID, Limit: query.Limit,
		Events: events,
		NextURL: buildAuditWebURL("/web/audit", query, nextCursor),
		ExportURL: buildAuditWebURL("/api/v1/audit/events/export", query, currentCursor),
	}
	if nextCursor == "" {
		data.NextURL = ""
	}

	var body bytes.Buffer
	if err := auditWebTemplate.Execute(&body, data); err != nil {
		http.Error(w, "Audit UI временно недоступен", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func buildAuditWebURL(path string, query audit.Query, cursor string) string {
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
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	return path + "?" + values.Encode()
}
