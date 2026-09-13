# Control Center 0.34 — Managed Network Planning handoff

Статус: **SOURCE PREPARED / RUNNER НЕ ЗАПУСКАЛСЯ**.

Рабочая ветка: `work/cc034-network-planning-readmodel-nr1-b2-20260913`.
База при создании: canonical development `main` `e578a81c5e16e246916f79155e19d00e53c446f8`.

## Подготовленный scope

- SUPPORT_LAN / SUPPORT_WAN как first-class planning classifications;
- fail-closed запрет routed transit через любую SUPPORT_* zone;
- bounded `ui.network-planning/v1` read model поверх exact canonical `network.change.plan/v1`;
- exact `plan_id` / node / revision binding;
- current → target interface zone projection;
- forwarding/probes/staged-steps projection без backend/provider payload;
- mandatory rollback/recovery evidence projection;
- Support Gateway требует один distinct SUPPORT_LAN и один distinct SUPPORT_WAN interface;
- `transit_authorized=false`, `mutation_authorized=false`, `external_publication_authorized=false`;
- closed JSON schema и test-only negative/read-model coverage.

## Не входит в slice

- apply сетевых изменений;
- выполнение connectivity verification;
- automatic rollback runtime;
- firewall/NAT/port-forward execution;
- external publication;
- support remote-access execution.

Это plan-only 0.34 scope. Никакая runtime authority не должна выводиться из наличия read model или Support Gateway classifications.

## Runner handoff

Перед qualification заново сверить current `main`; PASS другой branch identity не переносить. После получения exact current-base head достаточно штатных Go/schema/security checks текущего pipeline. Не создавать synthetic workflow, duplicate check или manual rerun без конкретной инфраструктурной причины.

Текущий Public Stable, проверенный в этом run, — 0.31.1. Поэтому эта 0.34 линия находится на максимальной разрешённой границе `0.32 → 0.33 → 0.34`; дальнейшую функциональность в этой задаче не добавлять.