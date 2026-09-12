# Control Center 0.31 — OSS / SBOM / commercial evidence

Статус: **engineering evidence only / НЕ commercial clearance / НЕ RC / НЕ Public Stable**.

## Что закрывает этот slice

Для exact candidate 0.31 packaging теперь формируются и проверяются:

- точный dependency/license inventory из `third_party/manifest-0.31.json`;
- проверка полного соответствия inventory текущему `go.mod`;
- SHA-256 каждого включённого текста лицензии;
- CycloneDX 1.7 SBOM, привязанный к exact candidate SHA;
- `THIRD_PARTY_NOTICES.md` с точными версиями и SPDX;
- включение SBOM, notices и license evidence в distributable candidate;
- SHA-256 linkage этих evidence-artifacts в provenance/release manifest/SHA256SUMS;
- fail-closed проверка расширенного candidate artifact set.

Текущий Go dependency graph содержит только MIT и BSD-3-Clause компоненты, относящиеся к ALLOW-классу инженерной OSS policy. Это не является самостоятельным юридическим заключением.

## Контракт external commercial/legal clearance

Для устранения неоднозначности последнего внешнего release blocker подготовлен source-only контракт `control-center.commercial-legal-disposition.v1` (`api/commercial-legal-disposition-v1.schema.json`) и fail-closed evaluator `releasecandidate.EvaluateCommercialLegalDisposition`.

Контракт **не создаёт и не подменяет юридическое одобрение**. Он только позволяет после получения решения уполномоченного лица/органа представить его как bounded exact-candidate evidence и получить детерминированный SHA-256 digest для `commercial_legal_clearance`.

Допустимый disposition обязан одновременно:

- быть `decision=approved` и относиться строго к candidate `0.31.0` + exact 40-hex candidate SHA;
- фиксировать юридическое лицо, лицензиара, применимое право, market scope и B2B/B2C model;
- содержать opaque `authority_id`, timestamp approval и, если задан, ещё действующий expiry;
- содержать ровно по одному SHA-256 evidence reference для каждого обязательного класса: legal entity authority, governing law/markets, EULA, Terms, Privacy, Support policy, SLA, licensing model, pricing/billing/refund, qualified legal review и security/privacy disposition;
- быть canonical и непротиворечивым: duplicate/unknown/missing evidence, invalid digest, future approval, expired disposition или неверный candidate identity отклоняются fail closed.

Порядок market/evidence элементов не влияет на получаемый digest: evaluator сначала нормализует и сортирует bounded evidence, затем хеширует canonical JSON. Это предотвращает появление разных gate digests для одного и того же юридического решения только из-за порядка полей/элементов.

До фактического получения внешнего approved disposition этот контракт остаётся только подготовленным механизмом и **не переводит** `commercial_legal_clearance` в PASS.

## Qualification boundary

После любого изменения кода, форматирования или набора сохраняемых candidate artifacts требуется ровно один hosted Public CI на текущем exact PR head. Результаты предыдущего head не переиспользуются как qualification нового candidate identity; duplicate workflow, synthetic load и бессмысленные rerun запрещены.

Подготовка commercial/legal disposition выполняется отдельно от инженерного CI. Ее digest можно связывать с release readiness только после независимой qualification самого evaluator/schema на exact integration head и после получения реального external approval. Source-only ветка или тестовые placeholder values не являются clearance evidence.

## Что этот slice намеренно НЕ закрывает

`commercial_legal_clearance` остаётся закрытым. Техническая генерация SBOM/notices и наличие disposition-контракта не заменяют:

- утверждение юридического лица/лицензиара и применимого права;
- финальные EULA/Terms/Privacy/Support/SLA;
- решение по рынкам/юрисдикциям и B2B/B2C;
- финальную модель лицензирования, pricing/billing/refund/support;
- квалифицированный legal review применимых обязательств;
- финальное vulnerability/security disposition exact candidate.

До получения этих evidence promotion gate должен оставаться fail-closed.

## Release boundary

Наличие `SBOM=PASS` и `THIRD_PARTY_NOTICES=PASS` означает только инженерную готовность соответствующих subgates. Подготовленный `commercial-legal-disposition` evaluator означает только готовность формата для внешнего решения. Ни то, ни другое само по себе не разрешает публикацию 0.31 и не изменяет canonical Stable 0.30.0.
