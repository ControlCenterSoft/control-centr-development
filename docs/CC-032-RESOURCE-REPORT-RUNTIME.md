# Control Center 0.32 — runtime resource report from Incidents

## Цель

Закрыть непокрытую runtime-границу 0.32 `Health / Incidents / Audit / Reports`: уже квалифицированные Incident read model и `Incident -> OperationalReport` adapter должны иметь bounded production-capable provider, который способен построить полный resource/health report без ложного `Healthy` на частичном наборе данных.

Этот слой не является новым источником истины и не создаёт отдельную Incident-модель. Он читает canonical `incidents.Reader`, полностью собирает ограниченный keyset-pagination набор активных Incident и только затем использует существующий `BuildIncidentOperationalReport`.

## Fail-closed правила

Provider обязан:

- запрашивать только активные `open` / `acknowledged` Incident; resolved history остаётся в Incident/Audit surfaces и не накапливается бесконечно в current resource-health projection;
- повторно отклонять `resolved`, если storage adapter нарушил server-side status filter;
- читать страницы только с canonical `incidents.MaxListLimit` и передавать opaque `Before` cursor без интерпретации версии;
- повторно валидировать каждый полученный Incident на storage/transport boundary;
- отклонять страницу больше запрошенного лимита;
- отклонять `HasMore=true` без `Next`, пустую продолжаемую страницу, terminal page с `Next`, cursor не совпадающий с последним возвращённым Incident и не продвигающийся cursor;
- отклонять повтор одного Incident между страницами;
- ограничивать итоговый объём по числу resource-bound evidence, а не только числу Incident: максимум `ui.MaxReportItems`;
- при ошибке чтения, malformed stored state, pagination inconsistency или overflow не возвращать частичный report;
- считать пустой, но успешно прочитанный источник `loaded + unknown`, а не `healthy`.

Отсутствие активного Incident не является доказательством Healthy. Поэтому пустая текущая проекция остаётся `unknown`; фактический Healthy должен происходить из отдельного authoritative Health observation source.

## RBAC и минимизация данных

`IncidentOperationalReportProvider` не принимает actor, permission, scope или source от клиента. Авторизация остаётся у server-side route binding.

Normal runtime связывает provider с существующими `/api/v1/ui/reports/resources`, `/api/v1/ui/reports/resources/evidence` и `/reports/resources`. Для них сохраняется существующая глобальная граница `resources.read`. Site-scoped identity не должна получать глобальный aggregate через этот route.

Resource report не публикует Incident title, signal summary, actor/acknowledgement identity, SourceIP, provider payload, raw error, credential или mutation material. В общий report переходят только resource identity, bounded state/reason, optional runbook reference и content-addressed Incident evidence reference/digest, уже определённые `BuildIncidentOperationalReport`.

## Audit report

Этот work package намеренно не активирует Audit-backed Operational Report. Для Audit adapter требуется отдельный exact `AuditReportBinding` к resource identity. До появления authoritative binding source Audit report должен оставаться отсутствующим/fail-closed, а не строить resource binding по action string или другим предположениям.

## Runtime activation

Normal runtime теперь строит `IncidentOperationalReportProvider` на canonical PostgreSQL `IncidentReadRepository` и передаёт его в существующий `withResourceReportsProvider`. Incident mutation API, permissions и persistence semantics не меняются.

Изменение подготовлено отдельно от Incident browser PR и не зависит от его UI кода. Оно добавляет provider-level negative/unit coverage для pagination integrity, active-status filter, backend failure, repeated state и bounded evidence overflow.

До runner-квалификации эта ветка не является canonical `main`, Release Candidate или Public Stable evidence. Она не меняет `VERSION`, SQL migrations, permissions, dependencies, release artifacts и commercial/legal status.
