# Control Center 0.32 — authoritative Health report provider boundary

Status: runner-free prepared source slice for the committed 0.32 `Health / Incidents / Audit / Reports` release train. It is intentionally staged on `work/cc032-health-report-provider-nr1-20260913` and is not a qualified or canonical release claim until the normal runner flow later validates and integrates it.

## Problem being closed

The 0.32 line already contains a qualified fail-closed Health Overview, Health → Operational Report adapter, authenticated source-specific Reports routes and browser renderer. The normal runtime, however, must not manufacture Health evidence from arbitrary `resources.Resource.Status` strings, raw provider payloads or transport errors. Those sources do not by themselves satisfy the canonical Health signal contract and could turn unknown data into false Healthy state.

This slice therefore adds the missing typed source seam before any production activation:

`authoritative HealthSignalSource → HealthOverview → OperationalReportProvider → existing resources.read Reports boundary`.

It does not register a runtime provider or route by itself.

## Contract

`HealthSignalSource` returns only a bounded `HealthSignalSnapshot`:

- `Loaded=true` means the source was read and the supplied signals may be validated;
- `Loaded=false` means the source is unavailable and the snapshot must contain no partial signals;
- every signal is still revalidated by the existing `BuildHealthOverview` boundary;
- a source error discards any returned partial payload and becomes an explicit `unavailable / unknown` report;
- cancellation remains cancellation and is not rewritten into health evidence;
- the provider uses an injected trusted clock and explicit stale/expired budgets;
- loaded-but-empty evidence remains `unknown`, never `healthy`;
- future, malformed, duplicated or contradictory Health evidence fails closed;
- `mutation_authorized` remains false throughout the projection.

The provider deliberately has no actor, permission, provider-selection or mutation input. Authorization remains in the existing server-side route binding where resource/Health reports require global `resources.read`.

## Security boundary

This implementation MUST NOT infer Health from:

- arbitrary resource status strings;
- provider/private error text;
- credentials, tokens or transport metadata;
- absence of records;
- operator incident resolution alone;
- stale or expired evidence as if it were current.

Source/provider failures are data availability failures, not successful empty results. Healthy observations degrade when stale and become unknown when expired through the already-qualified Health Overview semantics. Existing degraded/unhealthy observations never improve merely because they age.

## Release and commercial boundary

This slice:

- stays entirely inside Control Center 0.32, which is within the allowed three-release window after Public Stable 0.31.0;
- adds no SQL migration, permission, third-party dependency or redistribution obligation;
- does not change VERSION, release metadata or the published 0.31 Stable identity;
- does not create generic execution, remediation, infrastructure mutation or external-publication authority;
- does not claim commercial/legal clearance;
- does not activate a production Health provider until an authoritative `HealthSignalSource` implementation is separately reviewed and qualified.

The prepared unit tests are intentionally not executed by this runner-free task. A subsequent runner-dependent flow may qualify this exact source and then decide whether a concrete authoritative source can be safely wired into normal runtime.
