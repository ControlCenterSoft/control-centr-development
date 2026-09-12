# Control Center 0.32 — Health/Incidents → Reports adapters

Этот slice закрывает следующую непокрытую часть milestone 0.32 после уже квалифицированных Health Overview и Reports / Evidence Drawer read-model contracts. Он добавляет только детерминированные cross-contract adapters и не активирует новый HTTP/UI маршрут.

## Health Overview → Operational Report

Адаптер принимает только `ui.health-overview/v1` без mutation authority. `unavailable` остаётся `unavailable/unknown`. Для loaded evidence повторно проверяется согласованность `observed_state`, freshness и `effective_state`; противоречивый upstream view отклоняется fail-closed. Stale/expired evidence не улучшается при проекции. Идентификатор report evidence является bounded content-derived identity, поэтому максимально допустимый upstream signal ID не раздувает downstream contract.

## Incidents → Operational Report / Evidence Drawer

Адаптер принимает только полный bounded `incidents.ListPage`, уже полученный через authoritative Incident OperatorService/RBAC boundary. `HasMore`/cursor означает неполный набор и блокирует aggregate report, чтобы усечённая страница не могла создать ложный общий статус.

Каждый affected resource получает отдельное resource-bound evidence. Для активных incidents:

- `critical` → `unhealthy`;
- `warning` → `degraded`;
- `info` → `unknown`.

`resolved` намеренно проецируется как `unknown`, а не `healthy`: операторское закрытие incident само по себе не является post-condition доказательством здоровья ресурса. Evidence identity/digest детерминированно связываются с точной incident identity/generation/resource-version/status/severity и не содержат raw provider payload, credentials или secret values.

## Security / release boundary

- adapters read-only и всегда сохраняют `mutation_authorized=false`;
- malformed/tampered upstream evidence отклоняется целиком;
- partial incident page не считается полным operational report;
- Evidence Drawer остаётся resource-bound и не должен показывать evidence другого ресурса;
- новый permission, SQL migration, runtime dependency, execution authority или external publication authority не добавляются;
- этот slice не меняет `VERSION` и не заявляет 0.32 RC/Public Stable;
- authenticated server-side route/resource binding, Audit→Reports adapter и финальная UI/accessibility activation остаются отдельными последующими gates и не должны дублировать эту source qualification без содержательного изменения exact head.
