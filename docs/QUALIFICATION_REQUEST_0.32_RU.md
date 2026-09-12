# Control Center 0.32 — qualification request

Draft PR #186 qualifies the bounded Audit CSV export source slice on an exact head. The route remains disabled until hosted qualification passes and a separate integration decision is made.

Текущая опубликованная базовая версия — Control Center 0.31.0 PUBLIC STABLE: stable commit `ab9f15fe804631ee6828f2be9b489f1a748522e3`, tag `v0.31.0`. Текущий canonical development `main` после release-blocking browser-auth regression fix — `e4fd4cb5941059cd5d77325340fc538bdf31f987`. Qualification этого PR должна выполняться на свежем PR merge result против актуального `main`; PASS старого pre-0.31 base не переносится.

Legal-document development is deferred and is not part of this technical slice. Commercial/legal status не расширяется этим source-only изменением; обязательные security/privacy, no-secret, bounded-export и fail-closed evidence требования остаются release gates.
