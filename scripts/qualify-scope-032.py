#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import re

CANDIDATE_VERSION = "0.32.0"
SCHEMA = "control-center.release-scope-0.32.v1"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")

REQUIRED_FILES = (
    "migrations/0013_incident_read_models.up.sql",
    "internal/ui/health_overview.go",
    "internal/ui/health_overview_validate.go",
    "internal/ui/httpapi/health.go",
    "cmd/control-center/health_web.go",
    "internal/incidents/httpapi/web_v1.go",
    "internal/incidents/httpapi/wiring.go",
    "internal/identity/httpapi/audit_web.go",
    "internal/identity/httpapi/web_reports_v1.go",
    "docs/CC-032-HEALTH-OVERVIEW-READMODEL.md",
    "docs/CC-032-INCIDENT-PERSISTENCE.md",
    "docs/AUDIT_EXPORT_0_32_RU.md",
    "docs/CC-032-REPORTS-EVIDENCE-DRAWER.md",
)

SOURCE_ASSERTIONS = {
    "cmd/control-center/product_api.go": (
        'GET /api/v1/ui/health',
        'GET /health',
        'GET /api/v1/ui/reports/resources',
        'GET /api/v1/ui/reports/audit',
        'GET /reports/resources',
        'GET /reports/audit',
        "PermissionResourcesRead",
        "PermissionAuditRead",
    ),
    "internal/incidents/httpapi/wiring.go": (
        "NewWithBrowser",
        "AuthenticatedCurrentPassword",
    ),
    "internal/identity/httpapi/audit_web.go": (
        "audit.events_web",
        "Cache-Control",
        "no-store",
    ),
    "internal/identity/httpapi/web_reports_v1.go": (
        "Evidence Drawer",
        "Read-only evidence",
        "Unknown / нет достаточного подтверждения",
    ),
    "internal/ui/health_overview.go": (
        'HealthDataUnavailable = "unavailable"',
        'HealthStateUnknown',
        'MutationAuthorized bool',
    ),
}


def sha256_file(path: pathlib.Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            h.update(chunk)
    return "sha256:" + h.hexdigest()


def checked_text(path: pathlib.Path) -> str:
    data = path.read_text(encoding="utf-8")
    if "\x00" in data:
        raise ValueError(f"NUL byte in source file: {path}")
    return data


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", default=".")
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    candidate_sha = args.candidate_sha.strip()
    if not SHA_RE.fullmatch(candidate_sha):
        raise ValueError("candidate SHA must be exact lowercase 40-hex revision")

    root = pathlib.Path(args.repo).resolve()
    version_path = root / "VERSION"
    if not version_path.is_file() or version_path.read_text(encoding="utf-8").strip() != CANDIDATE_VERSION:
        raise ValueError("source VERSION must be 0.32.0 for scope qualification")

    evidence_files: dict[str, str] = {}
    for relative in REQUIRED_FILES:
        path = root / relative
        if not path.is_file():
            raise ValueError(f"required 0.32 scope file missing: {relative}")
        evidence_files[relative] = sha256_file(path)

    for relative, assertions in SOURCE_ASSERTIONS.items():
        path = root / relative
        if not path.is_file():
            raise ValueError(f"required 0.32 source boundary missing: {relative}")
        text = checked_text(path)
        missing = [value for value in assertions if value not in text]
        if missing:
            raise ValueError(f"0.32 source boundary incomplete in {relative}: missing={missing}")
        evidence_files.setdefault(relative, sha256_file(path))

    # Explicit negative-authority assertions are intentionally textual and
    # source-bound. If these public contracts drift, the release gate requires
    # re-review instead of silently inheriting an earlier PASS.
    health = checked_text(root / "internal/ui/health_overview.go")
    if "MutationAuthorized: false" not in health:
        raise ValueError("Health Overview no longer proves mutation_authorized=false")

    reports = checked_text(root / "internal/identity/httpapi/web_reports_v1.go")
    if "Mutation authority</dt><dd>нет" not in reports:
        raise ValueError("Reports Web UI no longer renders explicit no-mutation boundary")

    evidence = {
        "schema": SCHEMA,
        "status": "PASS",
        "candidate_version": CANDIDATE_VERSION,
        "candidate_sha": candidate_sha,
        "scope": {
            "health_overview": True,
            "incidents": True,
            "audit": True,
            "reports_evidence_drawer": True,
            "authenticated_server_side_rbac": True,
            "fail_closed_unknown_stale_unavailable": True,
            "mutation_authority_granted": False,
        },
        "source_files": dict(sorted(evidence_files.items())),
        "publication_authority": False,
    }
    output = pathlib.Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(evidence, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")
    print("CC_032_SCOPE=PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
