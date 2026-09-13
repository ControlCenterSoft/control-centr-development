#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path

STABLE_VERSION = "0.31.1"
STABLE_TAG = "v0.31.1"
STABLE_ARTIFACT_DIGEST = "sha256:b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97"
CANDIDATE_VERSION = "0.32.0"
SCHEMA = "control-center.release-candidate-readiness.0.32.v1"
SCOPE_SCHEMA = "control-center.release-scope-0.32.v1"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")

UPSTREAM_KEYS = (
    "public_safety",
    "static_analysis",
    "unit",
    "build",
    "postgres_migration",
    "postgres_adapter",
    "race",
)

EXPECTED_ARTIFACTS = (
    "control-center-0.32.0-linux-amd64.tar.gz",
    "control-center-0.32.0-linux-amd64.tar.gz.sha256",
    "control-center-0.32.0-source.tar.gz",
    "control-center-0.32.0.sbom.cdx.json",
    "THIRD_PARTY_NOTICES.md",
    "control-center-0.32.0.provenance.json",
    "control-center-0.32.0.qualification.json",
    "control-center-0.32.0.release-manifest.json",
)


def sha256_bytes(data: bytes) -> str:
    return "sha256:" + hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def canonical_digest(value: object) -> str:
    raw = json.dumps(value, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return sha256_bytes(raw)


def load_json(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        data = json.load(handle)
    if not isinstance(data, dict):
        raise ValueError(f"{path.name} must contain a JSON object")
    return data


def parse_sha256sums(path: Path) -> dict[str, str]:
    parsed: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        parts = line.split(maxsplit=1)
        if len(parts) != 2:
            raise ValueError("invalid SHA256SUMS line")
        digest, name = parts
        name = name.lstrip("*")
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError(f"invalid digest for {name}")
        if name in parsed:
            raise ValueError(f"duplicate checksum entry for {name}")
        parsed[name] = "sha256:" + digest
    return parsed


def validate_scope(path: Path, candidate_sha: str) -> str:
    evidence = load_json(path)
    if evidence.get("schema") != SCOPE_SCHEMA:
        raise ValueError("unsupported 0.32 scope evidence schema")
    if evidence.get("status") != "PASS":
        raise ValueError("0.32 scope evidence is not PASS")
    if evidence.get("candidate_version") != CANDIDATE_VERSION or evidence.get("candidate_sha") != candidate_sha:
        raise ValueError("0.32 scope evidence identity mismatch")
    scope = evidence.get("scope")
    if not isinstance(scope, dict):
        raise ValueError("0.32 scope evidence missing")
    for name in (
        "health_overview",
        "incidents",
        "audit",
        "reports_evidence_drawer",
        "authenticated_server_side_rbac",
        "fail_closed_unknown_stale_unavailable",
    ):
        if scope.get(name) is not True:
            raise ValueError(f"0.32 scope gate incomplete: {name}")
    if scope.get("mutation_authority_granted") is not False:
        raise ValueError("0.32 read-only scope unexpectedly grants mutation authority")
    if evidence.get("publication_authority") is not False:
        raise ValueError("0.32 scope evidence unexpectedly grants publication authority")
    digest = sha256_file(path)
    if not DIGEST_RE.fullmatch(digest):
        raise ValueError("invalid 0.32 scope evidence digest")
    return digest


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-dir", required=True)
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--scope-evidence", required=True)
    parser.add_argument("--workflow-run-id", required=True)
    parser.add_argument("--workflow-run-attempt", required=True)
    for key in UPSTREAM_KEYS:
        parser.add_argument("--" + key.replace("_", "-"), required=True)
    parser.add_argument("--workflow-evidence-output", required=True)
    parser.add_argument("--readiness-output", required=True)
    args = parser.parse_args()

    candidate_sha = args.candidate_sha.strip()
    if not SHA_RE.fullmatch(candidate_sha):
        raise ValueError("invalid exact candidate SHA")

    statuses = {key: getattr(args, key) for key in UPSTREAM_KEYS}
    failed = {key: value for key, value in statuses.items() if value != "success"}
    if failed:
        raise ValueError(f"upstream qualification is not fully successful: {failed}")

    scope_digest = validate_scope(Path(args.scope_evidence), candidate_sha)

    root = Path(args.candidate_dir)
    if not root.is_dir():
        raise ValueError("candidate directory missing")

    sums_path = root / "SHA256SUMS"
    if not sums_path.is_file():
        raise ValueError("SHA256SUMS missing")
    sums = parse_sha256sums(sums_path)

    for name in EXPECTED_ARTIFACTS:
        path = root / name
        if not path.is_file():
            raise ValueError(f"candidate artifact missing: {name}")
        actual = sha256_file(path)
        if sums.get(name) != actual:
            raise ValueError(f"candidate checksum mismatch: {name}")

    qualification_path = root / "control-center-0.32.0.qualification.json"
    provenance_path = root / "control-center-0.32.0.provenance.json"
    manifest_path = root / "control-center-0.32.0.release-manifest.json"

    qualification = load_json(qualification_path)
    provenance = load_json(provenance_path)
    manifest = load_json(manifest_path)

    if qualification.get("candidate_version") != CANDIDATE_VERSION or qualification.get("candidate_sha") != candidate_sha:
        raise ValueError("qualification identity mismatch")
    stable_base = qualification.get("stable_base")
    if not isinstance(stable_base, dict):
        raise ValueError("qualification Stable base missing")
    if stable_base.get("version") != STABLE_VERSION or stable_base.get("artifact_digest") != STABLE_ARTIFACT_DIGEST:
        raise ValueError("qualification Stable base mismatch")
    if qualification.get("status") != "PARTIAL_PASS_NOT_RC":
        raise ValueError("unexpected qualification status")
    qualification_gates = qualification.get("gates")
    if not isinstance(qualification_gates, dict):
        raise ValueError("qualification gates missing")
    for gate in (
        "candidate_artifact_packaging",
        "clean_install",
        "upgrade_from_stable_0_31_1",
        "rollback_forward_recovery",
    ):
        if qualification_gates.get(gate) != "PASS":
            raise ValueError(f"qualification gate is not PASS: {gate}")

    commercial_engineering = qualification.get("commercial_engineering_subgates")
    if not isinstance(commercial_engineering, dict):
        raise ValueError("commercial engineering subgates missing")
    for gate in ("dependency_license_inventory", "sbom", "third_party_notices"):
        if commercial_engineering.get(gate) != "PASS":
            raise ValueError(f"commercial engineering subgate is not PASS: {gate}")

    if provenance.get("candidate_version") != CANDIDATE_VERSION or provenance.get("candidate_sha") != candidate_sha:
        raise ValueError("provenance identity mismatch")
    if provenance.get("publication_authority") is not False:
        raise ValueError("candidate provenance unexpectedly grants publication authority")

    if manifest.get("version") != CANDIDATE_VERSION or manifest.get("revision") != candidate_sha:
        raise ValueError("release manifest identity mismatch")
    if manifest.get("stable_base") != STABLE_VERSION:
        raise ValueError("release manifest Stable base mismatch")
    if manifest.get("status") != "candidate-only-not-public-stable":
        raise ValueError("unexpected release manifest status")
    if manifest.get("publication_authority") is not False:
        raise ValueError("candidate release manifest unexpectedly grants publication authority")

    workflow_evidence = {
        "schema": "control-center.candidate-workflow-evidence.0.32.v1",
        "candidate_version": CANDIDATE_VERSION,
        "candidate_sha": candidate_sha,
        "workflow_run_id": str(args.workflow_run_id),
        "workflow_run_attempt": str(args.workflow_run_attempt),
        "checks": statuses,
        "security_privacy_scope": [
            "public_repository_no_credential_boundary",
            "rbac_and_scope_contracts",
            "first_login_password_change_boundary",
            "stale_and_idempotency_fail_closed_contracts",
            "audit_integrity_and_export_contracts",
            "evidence_drawer_resource_binding",
            "recovery_and_false_success_contracts",
            "postgres_restart_and_reconnect_contracts",
        ],
        "publication_authority": False,
    }
    workflow_output = Path(args.workflow_evidence_output)
    workflow_output.write_text(
        json.dumps(workflow_evidence, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )

    qualification_digest = sha256_file(qualification_path)
    workflow_digest = sha256_file(workflow_output)
    security_digest = canonical_digest({"workflow": workflow_digest, "scope": scope_digest})
    metadata_digest = canonical_digest(
        {
            "release_manifest": sha256_file(manifest_path),
            "provenance": sha256_file(provenance_path),
            "sha256sums": sha256_file(sums_path),
        }
    )

    def passed(gate: str, evidence_digest: str) -> dict:
        return {
            "gate": gate,
            "status": "pass",
            "candidate_sha": candidate_sha,
            "evidence_digest": evidence_digest,
        }

    readiness = {
        "schema": SCHEMA,
        "stable_version": STABLE_VERSION,
        "stable_tag": STABLE_TAG,
        "stable_artifact_digest": STABLE_ARTIFACT_DIGEST,
        "candidate_version": CANDIDATE_VERSION,
        "candidate_sha": candidate_sha,
        "gates": [
            passed("health_incidents_audit_reports_integration", scope_digest),
            passed("candidate_artifact_packaging", qualification_digest),
            passed("clean_install", qualification_digest),
            passed("upgrade_from_stable_0_31_1", qualification_digest),
            passed("rollback_forward_recovery", qualification_digest),
            passed("postgres_restart_reconnect", workflow_digest),
            passed("security_privacy", security_digest),
            {
                "gate": "commercial_legal_clearance",
                "status": "blocked",
                "candidate_sha": candidate_sha,
            },
            passed("release_metadata", metadata_digest),
        ],
    }
    Path(args.readiness_output).write_text(
        json.dumps(readiness, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )

    print("RELEASE_032_EVIDENCE_AGGREGATION=PASS")
    print("RELEASE_032_SCOPE_GATE=PASS")
    print("SECURITY_PRIVACY_ENGINEERING_GATE=PASS")
    print("RELEASE_METADATA_GATE=PASS")
    print("COMMERCIAL_LEGAL_CLEARANCE=BLOCKED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
