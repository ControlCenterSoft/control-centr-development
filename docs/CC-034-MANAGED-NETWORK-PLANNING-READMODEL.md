# Control Center 0.34 — Managed Network Planning read model

Статус: **runner-free source slice / не Release Candidate / не Public Stable**.

## Граница

Этот slice развивает закреплённый Roadmap milestone 0.34 **Managed Network Planning UI** и намеренно не реализует применение сетевой конфигурации. Он строит bounded read-only operator projection поверх существующего canonical `network.change.plan/v1`.

В scope текущего slice:

- точная привязка UI к `plan_id`, `node_id`, `revision_id`;
- current → target zone projection для интерфейсов одного canonical plan;
- staged steps и connectivity probes без provider/backend payload;
- явный rollback contract: recovery snapshot обязателен, probe failure/timeout требуют automatic rollback в будущем execution milestone;
- `forwarding_default_deny=true` сохраняется в UI evidence;
- `mutation_authorized=false` и `external_publication_authorized=false` являются жёсткими свойствами;
- planning classifications `SUPPORT_LAN` и `SUPPORT_WAN`;
- если plan содержит Support Gateway channels, требуется ровно один отдельный `SUPPORT_LAN` interface и один отдельный `SUPPORT_WAN` interface;
- любой routed forwarding между SUPPORT_* и другой zone fail-closed отклоняется. Наличие двух каналов Support Gateway не делает узел общим router/NAT gateway.

## Fail-closed semantics

`BuildNetworkPlanningView` повторно строит canonical ChangePlan из разрешённых typed inputs и принимает projection только при полном совпадении exact plan, включая content-addressed `plan_id`, canonical interfaces/probes/forwarding/timeouts и staged steps. Tampered/stale/noncanonical plan не визуализируется как допустимый change.

Current interface evidence ограничено bounded identity/name, текущей zone classification и operational state. Неизвестная zone, повтор interface ID, отсутствующий interface из exact plan или malformed state блокируют projection целиком.

Support Gateway projection допустима только при одновременно присутствующих и различных интерфейсах `SUPPORT_LAN` и `SUPPORT_WAN`; `transit_authorized=false` неизменяемо. Network-policy layer отдельно запрещает inter-zone forwarding, если source или destination является SUPPORT_* zone, даже при `explicitly_enabled=true` и назначенном Edge Gateway.

## ИБ

Текущий contract не содержит:

- credentials/secrets;
- arbitrary shell/command;
- provider endpoint/payload;
- gateway/address route mutation;
- firewall/NAT/port-forward mutation authority;
- approval/execution authority;
- external publication authority.

SUPPORT_LAN/SUPPORT_WAN являются только planning classifications. Межзонный transit через них запрещён независимо от `explicitly_enabled` или Edge Gateway assignment.

## Коммерческая/лицензионная граница

Slice не добавляет third-party runtime dependencies, bundled components, redistribution obligations или новые license/notices requirements. Существующий commercial/legal metadata pipeline не изменяется.

## Что ещё требуется для полного 0.34

Этот NR1 slice не закрывает весь milestone. Независимые slices внутри 0.34 должны добавить typed planning/evidence для routes, DNS/NTP, firewall policy, VLAN/bonding capability constraints и operator Web UI/API routing. Все они должны оставаться plan-only. Реальный staged apply/verify/rollback относится к следующему Roadmap milestone и в этой задаче не реализуется, потому что текущая разрешённая development boundary заканчивается на 0.34.

## Source identity и qualification boundary

Старая branch identity была значительно позади canonical development main. Поэтому source delta перенесён на `work/cc034-network-planning-readmodel-nr1-b2-20260913`, созданную от актуального на момент переноса `main` `e578a81c5e16e246916f79155e19d00e53c446f8`. Перенос не расширяет feature scope.

Tests/workflows в этой non-runner задаче не запускались. PR не создаётся, runner PASS не заявляется. Перед интеграцией runner-поток должен заново сверить текущий main, получить exact resulting head и выполнить только штатную qualification без synthetic load/duplicate rerun.