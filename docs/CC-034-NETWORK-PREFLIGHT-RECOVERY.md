# Control Center 0.34 — connectivity preflight и recovery projection

Этот контракт относится только к этапу планирования управляемой сети. Он дополняет staged network plan обязательным набором проверок связности и явной стратегией восстановления до того, как в более позднем этапе появится право применять сетевые изменения.

## Граница

План привязывается к точному узлу, текущей сетевой revision и новой planned revision. Planned revision обязана отличаться от base revision. Список проверок детерминированно канонизируется и входит в content-addressed `plan_id`, поэтому сохранённый или переданный план нельзя незаметно заменить другим набором проверок.

Обязательна как минимум одна проверка `management_reachability`. Поддерживаются только ограниченные типы проверок: management reachability, gateway reachability, DNS resolution и NTP reachability. Target — только hostname/IP, без URL, пути, query, credential material или управляющих символов. Gateway target обязан быть IP; DNS resolution проверяет hostname.

## Recovery

План всегда содержит recovery projection `restore_previous_network_state`, связанную с точной `base_revision_id`. Стратегия требует подтверждения оператора и проверки связности после rollback. Этот объект является только доказательством подготовленного recovery path: сам rollback не запускается и право на его выполнение не создаётся.

## Неизменяемая safety boundary

В 0.34 этот контракт всегда сохраняет:

- `connectivity_verification_required=true`;
- `mutation_authorized=false`;
- `apply_authorized=false`;
- `automatic_rollback_authorized=false`;
- `forwarding_authorized=false`;
- `nat_authorized=false`;
- `external_publication_authorized=false`;
- `support_remote_access_authorized=false`;
- `rollback.execution_authorized=false`.

Любой сохранённый/полученный через transport план повторно валидируется. Неканонический порядок probes, дублирование identity/binding, изменение target, rollback revision, authority flags или `plan_id` отклоняется fail-closed.

Фактический staged apply, connectivity verifier execution и automatic rollback runtime не входят в 0.34 и не должны появляться через этот контракт.
