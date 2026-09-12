# Control Center 0.32 — Audit → Reports adapter boundary

Этот slice закрывает отдельную runner-зависимую часть milestone 0.32 `Health / Incidents / Audit / Reports`: преобразование уже авторизованного bounded Audit read-result в общий `ui.operational-report/v1` без расширения полномочий и без подмены Audit evidence состоянием Health.

## Инварианты

- adapter принимает только полный bounded Audit page; `HasMore=true` блокируется, чтобы усечённая история не становилась ложным overall state;
- для каждого Audit entry требуется отдельная exact resource binding по `sequence_id + event_id + event_hash + resource_kind + resource_id`;
- `resource_id` обязан совпадать с canonical `Audit.Event.SubjectID`; resource kind не угадывается из строки action и задаётся вызывающей authoritative boundary;
- cross-resource, cross-event и cross-hash binding fail-closed отклоняются;
- Audit event hash переносится только как content-addressed `sha256:` evidence digest;
- source IP, actor identity, Audit details и любые provider/private payload не проецируются в Reports;
- successful Audit event **не означает Healthy**: success/denied/rejected/blocked/cancelled отображаются как `unknown`, failed/error — как `degraded`;
- adapter никогда не выставляет `mutation_authorized=true`;
- неизвестный outcome, malformed hash, future timestamp, duplicate binding или неresource-bound Audit entry являются ошибкой, а не пустым/Healthy результатом.

## Что этот slice не делает

Он не добавляет HTTP/UI route, новую permission, SQL migration, runtime dependency, generic execution, remediation или external-publication authority. Он также не заменяет Audit integrity verification и не доказывает resource health. Следующие отдельные gates — authenticated server-side Reports/Evidence Drawer routing/resource binding, Audit search/export UI, accessibility и финальная release qualification 0.32.

Public Stable остаётся 0.31.0 до завершения собственного release cycle 0.32.
