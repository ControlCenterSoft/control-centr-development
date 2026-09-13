# Control Center 0.32 — Health Overview HTTP boundary

Status: **RUNNER-FREE PREPARED / SOURCE-ONLY / NOT RC / NOT PUBLIC STABLE**.

Этот slice является вторым non-runner продолжением фактической линии 0.32 и построен поверх `work/cc032-health-report-provider-nr1-20260913`. Он не повторяет NR1: NR1 ввёл authoritative `HealthSignalSource` и преобразование `HealthSignalSource → HealthOverview → OperationalReport`; этот slice закрывает следующую отдельную границу — каноническую валидацию Health Overview, permission-bound GET API и read-only browser UI, которые можно позднее активировать только вместе с квалифицированным authoritative source.

## Реализуемая граница

Добавляются два read-only presentation contract:

- `GET /api/v1/ui/health` — JSON projection;
- `GET /health` — server-rendered operator browser той же exact Health projection.

Оба маршрута регистрируются только если сервер получил `HealthOverviewProvider`. Сам production `main` в этом slice не получает конкретный Health source, поэтому отсутствие authoritative provider сохраняет маршруты фактически неактивными. Это намеренно: resource `Status`, отсутствие записей, transport/provider errors и другие неподтверждённые данные не должны автоматически превращаться в Health evidence.

`HealthOperationalReportProvider` расширяется интерфейсом `HealthOverviewProvider`, поэтому одна и та же exact source snapshot/freshness policy может обслуживать first-class Health JSON, browser UI и уже существующий Reports adapter без второго независимого вычисления семантики.

Browser UI выводит только bounded поля канонического `HealthOverview`: общий/data state, resource kind/id/state/freshness, signal ID/check/state/freshness/observed time, bounded runbook reference и evidence references. Provider-controlled строки проходят `html/template` escaping и не интерпретируются как HTML или ссылки. UI не содержит acknowledge/remediate/execute/mutation controls и всегда показывает текстовые названия состояния/свежести, то есть смысл статуса не кодируется только цветом.

## Fail-closed validation

`ValidateHealthOverview` повторно проверяет provider boundary перед HTTP-выдачей:

- точную schema identity `ui.health-overview/v1`;
- `mutation_authorized=false`;
- допустимый `loaded/unavailable` data state;
- unavailable всегда только `unknown` и без частичного resource/signal evidence;
- bounded IDs/kinds/check names/runbook/evidence references;
- отсутствие duplicate signal IDs;
- согласованность `observed_state + freshness → effective_state`;
- canonical ordering сигналов;
- точное восстановление resource aggregates из signal evidence;
- соответствие overall state фактическому worst resource state.

Если provider возвращает ошибку, malformed/tampered/non-canonical view или unavailable evidence, обе presentation boundary возвращают `503` с каноническим `unavailable / unknown`, а не `200` с пустым либо Healthy состоянием. Provider-private error text не передаётся браузеру.

## Authentication и RBAC

Конкретный provider не выбирается клиентом. Product router связывает JSON и browser routes с global `resources.read` через существующие server-side middleware:

- JSON: `Authenticate + Require`;
- browser: `AuthenticateWeb + RequireWeb`.

Следствия:

- anonymous API caller не получает Health evidence; anonymous browser session переводится в штатный login flow;
- identity без binding — denied;
- site-scoped viewer не получает global Health aggregate;
- глобальные роли с `resources.read` получают только read-only projection;
- маршруты отсутствуют целиком, если authoritative provider не подключён;
- HTTP/browser handlers не содержат mutation, acknowledge, remediation, execute или provider-selection операций.

Prepared test-code фиксирует эти условия, а также `GET`-only API, `no-store`, `nosniff`, browser `DENY` framing, `no-referrer`, same-origin CSP, fail-closed source errors, tampered provider output и escaping provider-controlled text. В рамках non-runner задачи этот test-code намеренно не запускается.

## Security boundary

Этот slice не разрешает:

- выводить arbitrary `resources.Resource.Status` как Health truth;
- считать отсутствие сигналов доказательством Healthy;
- улучшать stale/expired/degraded/unhealthy evidence;
- раскрывать provider-private payload/error text, credentials или transport metadata;
- выдавать mutation/execution/remediation/external-publication authority;
- переключать Health source параметром запроса;
- превращать runbook/evidence reference в доверенную external navigation без отдельного validation contract.

First-class Health остаётся evidence-only. Любое будущее действие по remediation должно проходить отдельный `RBAC → Change/Approval → durable Job → post-condition verification → Audit/recovery` путь.

## Release / commercial boundary

Работа находится строго в 0.32.0, то есть в ближайшем release train после текущего Public Stable 0.31.0 и внутри разрешённого горизонта 0.32–0.34.

Slice:

- не меняет `VERSION`;
- не меняет уже опубликованные migrations;
- не добавляет SQL migration или third-party dependency;
- не создаёт новых redistribution/license obligations;
- не заявляет commercial/legal clearance;
- не меняет официальный Stable 0.31.0;
- не активирует production Health source до отдельной qualification.

Отдельный известный packaging/install blocker опубликованного 0.31.0 (отсутствующий `scripts/migrate.sh` в immutable Linux asset) этим 0.32 slice не маскируется и не исправляется: corrective release/fallback qualification должна оставаться отдельной release-integrity работой и не смешиваться с Health source identity.

## Следующая runner-зависимая стадия

Отдельный runner-поток должен квалифицировать exact head этой prepared-линии обычным Public CI после перевода в PR/qualification flow. До этого commits ветки являются только подготовленным source/test-code и не должны учитываться как PASS evidence. После qualification можно отдельно решать, какой фактически authoritative runtime source удовлетворяет `HealthSignalSource`; подмена этого этапа synthetic source или произвольным resource status запрещена.
