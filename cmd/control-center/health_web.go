package main

import (
	"bytes"
	"html/template"
	"net/http"

	"control-center/internal/buildinfo"
	identityapi "control-center/internal/identity/httpapi"
	productui "control-center/internal/ui"
)

type healthWebResource struct {
	Kind           string
	ID             string
	State          string
	StateLabel     string
	Freshness      string
	FreshnessLabel string
	SignalCount    int
}

type healthWebSignal struct {
	ID             string
	ResourceKind   string
	ResourceID     string
	CheckName      string
	ObservedState  string
	ObservedLabel  string
	EffectiveState string
	EffectiveLabel string
	Freshness      string
	FreshnessLabel string
	ObservedAt     string
	RunbookRef     string
	EvidenceRefs   []string
}

type healthWebPageData struct {
	Version           string
	DisplayName       string
	Username          string
	DataState         string
	DataStateLabel    string
	OverallState      string
	OverallStateLabel string
	Resources         []healthWebResource
	Signals           []healthWebSignal
}

var healthWebTemplate = template.Must(template.New("health-web").Parse(`<!doctype html>
<html lang="ru" data-locale="ru-RU">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Control Center — Health</title>
<style>
:root{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color-scheme:light dark;--s1:.35rem;--s2:.65rem;--s3:1rem;--s4:1.5rem;--radius:.8rem;--border:1px solid currentColor}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:Canvas;color:CanvasText}a{color:inherit}.skip{position:absolute;left:-9999px}.skip:focus{left:var(--s3);top:var(--s3);z-index:10;background:Canvas;padding:var(--s2);border:var(--border)}.topbar{display:flex;gap:var(--s3);align-items:center;justify-content:space-between;padding:var(--s3) var(--s4);border-bottom:var(--border)}.brand{display:grid;gap:var(--s1)}.muted,.eyebrow{font-size:.85rem;opacity:.78}.content{padding:var(--s4);max-width:92rem;width:100%;margin:0 auto}.summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--s2);margin:var(--s3) 0}.card{padding:var(--s3);border:var(--border);border-radius:var(--radius);display:grid;gap:var(--s1)}.label{font-size:.8rem;opacity:.78}.value{font-size:1.1rem;font-weight:750}.table-wrap{overflow-x:auto;margin-top:var(--s4);border:var(--border);border-radius:var(--radius)}table{width:100%;border-collapse:collapse;min-width:60rem}th,td{text-align:left;padding:.65rem;border-bottom:var(--border);vertical-align:top;overflow-wrap:anywhere}th{font-size:.85rem}.badge{display:inline-flex;border:var(--border);border-radius:999px;padding:.25rem .55rem;font-size:.85rem}.refs{margin:0;padding-left:1.15rem}.footer{display:flex;gap:var(--s3);align-items:center;justify-content:space-between;flex-wrap:wrap;margin-top:var(--s4);padding-top:var(--s3);border-top:var(--border)}button{font:inherit;padding:.55rem .65rem;border:var(--border);border-radius:.55rem;background:Canvas;color:CanvasText}@media(max-width:760px){.summary{grid-template-columns:1fr}.topbar{align-items:flex-start;flex-direction:column}.content{padding:var(--s3)}}@media(max-width:600px){.table-wrap{border:0}table,thead,tbody,tr,th,td{display:block;min-width:0}thead{position:absolute;left:-9999px}tr{border:var(--border);border-radius:var(--radius);padding:var(--s2);margin-bottom:var(--s2)}td{border:0;padding:.3rem 0}td::before{content:attr(data-label) ": ";font-weight:700}}
</style>
</head>
<body>
<a class="skip" href="#main-content">К основному содержимому</a>
<header class="topbar"><div class="brand"><span class="eyebrow">Control Center {{.Version}}</span><strong>Health</strong></div><span class="badge">Пользователь: {{.DisplayName}}</span></header>
<main class="content" id="main-content">
<h1>Состояние инфраструктуры</h1>
<p class="muted">Только чтение. Статус строится из authoritative Health evidence. Устаревшие и недоступные данные не отображаются как Healthy.</p>
<section class="summary" aria-label="Сводка Health">
<div class="card"><span class="label">Общее состояние</span><span class="value">{{.OverallStateLabel}} ({{.OverallState}})</span></div>
<div class="card"><span class="label">Данные</span><span class="value">{{.DataStateLabel}} ({{.DataState}})</span></div>
<div class="card"><span class="label">Объекты / сигналы</span><span class="value">{{len .Resources}} / {{len .Signals}}</span></div>
</section>
<h2>Объекты</h2>
{{if .Resources}}
<div class="table-wrap"><table><thead><tr><th>Тип</th><th>Объект</th><th>Состояние</th><th>Свежесть</th><th>Сигналы</th></tr></thead><tbody>{{range .Resources}}<tr><td data-label="Тип">{{.Kind}}</td><td data-label="Объект">{{.ID}}</td><td data-label="Состояние">{{.StateLabel}} ({{.State}})</td><td data-label="Свежесть">{{.FreshnessLabel}} ({{.Freshness}})</td><td data-label="Сигналы">{{.SignalCount}}</td></tr>{{end}}</tbody></table></div>
{{else}}<p role="status">Подтверждённых объектов Health сейчас нет.</p>{{end}}
<h2>Сигналы</h2>
{{if .Signals}}
<div class="table-wrap"><table><thead><tr><th>Объект</th><th>Проверка</th><th>Наблюдалось</th><th>Эффективно</th><th>Свежесть</th><th>Время</th><th>Runbook / evidence</th></tr></thead><tbody>{{range .Signals}}<tr><td data-label="Объект">{{.ResourceKind}} / {{.ResourceID}}</td><td data-label="Проверка">{{.CheckName}}<br><span class="muted">{{.ID}}</span></td><td data-label="Наблюдалось">{{.ObservedLabel}} ({{.ObservedState}})</td><td data-label="Эффективно">{{.EffectiveLabel}} ({{.EffectiveState}})</td><td data-label="Свежесть">{{.FreshnessLabel}} ({{.Freshness}})</td><td data-label="Время">{{.ObservedAt}}</td><td data-label="Runbook / evidence">{{if .RunbookRef}}<div>Runbook: {{.RunbookRef}}</div>{{end}}{{if .EvidenceRefs}}<ul class="refs">{{range .EvidenceRefs}}<li>{{.}}</li>{{end}}</ul>{{else}}{{if not .RunbookRef}}—{{end}}{{end}}</td></tr>{{end}}</tbody></table></div>
{{else}}<p role="status">Подтверждённых сигналов Health сейчас нет.</p>{{end}}
<footer class="footer"><span class="muted">Учётная запись: {{.Username}} · данные не кешируются</span><form method="post" action="/web/logout"><button type="submit">Выйти</button></form></footer>
</main>
</body>
</html>`))

func healthWebHandler(provider productui.HealthOverviewProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := identityapi.PrincipalFromContext(r.Context())
		if !ok || provider == nil {
			http.Error(w, "Health unavailable", http.StatusServiceUnavailable)
			return
		}

		view, err := provider.HealthOverview(r.Context())
		status := http.StatusOK
		if err != nil || productui.ValidateHealthOverview(view) != nil {
			view = unavailableHealthOverview()
			status = http.StatusServiceUnavailable
		} else if view.DataState == productui.HealthDataUnavailable {
			status = http.StatusServiceUnavailable
		}

		resources := make([]healthWebResource, 0, len(view.Resources))
		for _, resource := range view.Resources {
			resources = append(resources, healthWebResource{
				Kind: resource.ResourceKind, ID: resource.ResourceID,
				State: resource.State, StateLabel: healthStateLabel(resource.State),
				Freshness: resource.Freshness, FreshnessLabel: healthFreshnessLabel(resource.Freshness),
				SignalCount: len(resource.SignalIDs),
			})
		}
		signals := make([]healthWebSignal, 0, len(view.Signals))
		for _, signal := range view.Signals {
			signals = append(signals, healthWebSignal{
				ID: signal.ID, ResourceKind: signal.ResourceKind, ResourceID: signal.ResourceID,
				CheckName: signal.CheckName,
				ObservedState: signal.ObservedState, ObservedLabel: healthStateLabel(signal.ObservedState),
				EffectiveState: signal.EffectiveState, EffectiveLabel: healthStateLabel(signal.EffectiveState),
				Freshness: signal.Freshness, FreshnessLabel: healthFreshnessLabel(signal.Freshness),
				ObservedAt: signal.ObservedAt.UTC().Format("2006-01-02 15:04:05Z"),
				RunbookRef: signal.RunbookRef, EvidenceRefs: append([]string(nil), signal.EvidenceRefs...),
			})
		}

		var body bytes.Buffer
		if err := healthWebTemplate.Execute(&body, healthWebPageData{
			Version: buildinfo.Version, DisplayName: principal.Identity.DisplayName, Username: principal.Identity.Username,
			DataState: view.DataState, DataStateLabel: healthDataStateLabel(view.DataState),
			OverallState: view.OverallState, OverallStateLabel: healthStateLabel(view.OverallState),
			Resources: resources, Signals: signals,
		}); err != nil {
			http.Error(w, "Health unavailable", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.WriteHeader(status)
		_, _ = w.Write(body.Bytes())
	})
}

func unavailableHealthOverview() productui.HealthOverview {
	return productui.HealthOverview{
		Schema: productui.HealthOverviewSchemaV1,
		DataState: productui.HealthDataUnavailable,
		OverallState: productui.HealthStateUnknown,
		Resources: []productui.ResourceHealthView{},
		Signals: []productui.HealthSignalView{},
		MutationAuthorized: false,
	}
}

func healthDataStateLabel(state string) string {
	switch state {
	case productui.HealthDataLoaded:
		return "Доступны"
	case productui.HealthDataUnavailable:
		return "Недоступны"
	default:
		return "Неизвестно"
	}
}

func healthStateLabel(state string) string {
	switch state {
	case productui.HealthStateHealthy:
		return "Исправно"
	case productui.HealthStateDegraded:
		return "Деградация"
	case productui.HealthStateUnhealthy:
		return "Неисправно"
	case productui.HealthStateUnknown:
		return "Неизвестно"
	default:
		return "Неизвестно"
	}
}

func healthFreshnessLabel(freshness string) string {
	switch freshness {
	case productui.HealthFreshnessCurrent:
		return "Актуально"
	case productui.HealthFreshnessStale:
		return "Устаревает"
	case productui.HealthFreshnessExpired:
		return "Просрочено"
	default:
		return "Неизвестно"
	}
}
