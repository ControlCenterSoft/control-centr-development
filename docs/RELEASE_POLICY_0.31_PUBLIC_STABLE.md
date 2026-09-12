# Control Center 0.31 — PUBLIC STABLE promotion policy

Owner decision and canonical Roadmap CC-RM-1.7 require every started release train to terminate in an official PUBLIC STABLE release.

For 0.31.0 the product-release gate remains fail-closed for technical correctness: exact-head CI, source identity, packaging, clean install, supported upgrade from 0.30.0, rollback/forward recovery, PostgreSQL restart/reconnect, security/privacy, migration immutability, data/settings/admin-password preservation, release metadata, checksums and provenance must all pass.

Commercial/legal document preparation is a separate commercial-launch track. It does not authorize unsupported commercial claims and does not weaken any technical safety gate, but it is not a blocker for publishing the technically qualified product as PUBLIC STABLE. Commercial capabilities whose legal/evidence package is incomplete remain unclaimed or disabled until separately approved.

A release must not claim capabilities that lack their required technical evidence. False success, stale evidence, cross-revision evidence, migration drift, data-loss risk without recovery evidence, or failed security/upgrade/rollback qualification remain hard release blockers.
