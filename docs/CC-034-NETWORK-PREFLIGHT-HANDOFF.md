# Control Center 0.34 — Managed Network preflight/recovery handoff

Статус: **SOURCE PREPARED / RUNNER НЕ ЗАПУСКАЛСЯ**.

Ветка: `work/cc034-network-preflight-recovery-nr1-b2-20260913`.
Исходная ветка `work/cc034-network-preflight-recovery-nr1-20260913` отставала от canonical main, поэтому plan-only delta перенесён на более новую базу без изменения scope.

## Подготовлено

- `network.managed-planning-preflight/v1` closed schema;
- content-addressed deterministic `plan_id`;
- exact `node_id`, `base_revision_id`, `planned_revision_id` binding;
- bounded connectivity probes: management/gateway/DNS/NTP;
- canonical host/IP validation без URL/path/query/credential material;
- обязательный `management_reachability` probe;
- recovery projection `restore_previous_network_state` на exact base revision;
- mandatory operator confirmation и post-rollback verification;
- все mutation/apply/automatic-rollback/forwarding/NAT/external-publication/support-remote-access authorities hard-false;
- transport/storage revalidation, canonical-order check и plan-id tamper detection;
- test-only negative coverage, включая каждую запрещённую authority flag и target tampering.

## Не входит в 0.34 slice

- фактический apply сетевой конфигурации;
- выполнение connectivity probes;
- runtime automatic rollback;
- NAT/forwarding enablement;
- external publication;
- support remote-access enablement.

Эти действия не должны появляться через planning contract. Их реализация относится к последующим разрешённым milestone только в соответствии с canonical Roadmap и отдельными approval/Job/verification/recovery gates.

## Qualification handoff

Non-runner поток не запускает tests/build/CI и не заявляет PASS. Перед runner qualification нужно заново сверить current `main`, потому что canonical main продолжает изменяться параллельными потоками. Не переносить PASS со старой branch identity; квалифицировать только exact resulting head. Synthetic load и duplicate rerun не нужны.

Текущая продуктовая граница по фактическому Public Stable 0.31.1 допускает 0.32, 0.33 и 0.34; этот slice находится на последней разрешённой границе и не должен расширяться в 0.35+.