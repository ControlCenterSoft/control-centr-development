#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path

SCHEMA = "control-center.release-candidate-readiness.v1"
COMMERCIAL_SCHEMA = "control-center.commercial-legal-evidence.v1"
AUTHORITY_SCHEMA = "control-center.publication-authorization.v1"
STABLE_VERSION = "0.30.0"
STABLE_TAG = "v0.30.0"
STABLE_ARTIFACT_DIGEST = "sha256:02d15e8ff13bbcb52b6d0c9293ab8804500991fbb41c8b575e88306a8a5ce0f2"
CANDIDATE_VERSION = "0.31.0"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")

REQUIRED_GATES = (
    "manual_retry_lineage_integration",
    "operational_workflow_e2e",
    "candidate_artifact_packaging",
    "clean_install",
    "upgrade_from_stable_0_30",
    "rollback_forward_recovery",
    "postgres_restart_reconnect",
    "security_privacy",
    "commercial_legal_clearance",
    "release_metadata",
)

REQUIRED_COMMERCIAL_FLAGS = (
    "owner_authorization",
    "legal_review_completed",
    "licensor_identity_approved",
    "governing_law_approved",
    "market_scope_approved",
    "b2b_scope_approved",
    "licensing_model_approved",
    "support_model_approved",
    "dependencies_reviewed",
    "redistribution_reviewed",
    "notices_prepared",
    "source_obligations_resolved",
    "sbom_prepared",
    "legal_terms_dispositioned",
    "release_claims_reviewed",
    "publication_authority",
)

REQUIRED_DOCUMENTS = {"eula", "terms_of_use", "privacy_policy", "support_terms"}


def load_object(path: Path) -> dict:
    with path.open(encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError(f"{path.name} must contain a JSON object")
    return value


def canonical_digest(value: object) -> str:
    raw = json.dumps(value, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def validate_engineering(snapshot: dict) -> tuple[str, list[dict]]:
    if snapshot.get("schema") != SCHEMA:
        raise ValueError("unsupported readiness schema")
    if snapshot.get("stable_version") != STABLE_VERSION or snapshot.get("stable_tag") != STABLE_TAG:
        raise ValueError("unexpected Stable identity")
    if snapshot.get("stable_artifact_digest") != STABLE_ARTIFACT_DIGEST:
        raise ValueError("unexpected Stable artifact digest")
    if snapshot.get("candidate_version") != CANDIDATE_VERSION:
        raise ValueError("unexpected candidate version")

    candidate_sha = snapshot.get("candidate_sha")
    if not isinstance(candidate_sha, str) or not SHA_RE.fullmatch(candidate_sha):
        raise ValueError("invalid candidate sha")

    gates = snapshot.get("gates")
    if not isinstance(gates, list):
        raise ValueError("release gates missing")

    by_id: dict[str, dict] = {}
    for item in gates:
        if not isinstance(item, dict):
            raise ValueError("invalid release gate entry")
        gate = item.get("gate")
        if gate not in REQUIRED_GATES:
            raise ValueError(f"unknown release gate: {gate}")
        if gate in by_id:
            raise ValueError(f"duplicate release gate: {gate}")
        if item.get("candidate_sha") != candidate_sha:
            raise ValueError(f"release gate bound to different candidate: {gate}")
        by_id[gate] = item

    if set(by_id) != set(REQUIRED_GATES):
        raise ValueError("required release gate set is incomplete")

    for gate in REQUIRED_GATES:
        item = by_id[gate]
        if gate == "commercial_legal_clearance":
            if item.get("status") not in {"blocked", "pending", "pass"}:
                raise ValueError("invalid commercial/legal gate status")
            if item.get("status") == "pass" and not DIGEST_RE.fullmatch(str(item.get("evidence_digest", ""))):
                raise ValueError("commercial/legal PASS lacks bounded digest")
            continue
        if item.get("status") != "pass":
            raise ValueError(f"engineering gate is not PASS: {gate}")
        if not DIGEST_RE.fullmatch(str(item.get("evidence_digest", ""))):
            raise ValueError(f"engineering PASS lacks bounded digest: {gate}")

    return candidate_sha, gates


def validate_commercial(evidence: dict, candidate_sha: str) -> str:
    if evidence.get("schema") != COMMERCIAL_SCHEMA:
        raise ValueError("unsupported commercial/legal evidence schema")
    if evidence.get("candidate_version") != CANDIDATE_VERSION or evidence.get("candidate_sha") != candidate_sha:
        raise ValueError("commercial/legal evidence identity mismatch")
    if str(evidence.get("disposition", "")).strip().lower() != "approved":
        raise ValueError("commercial/legal disposition is not approved")

    missing = [name for name in REQUIRED_COMMERCIAL_FLAGS if evidence.get(name) is not True]
    if missing:
        raise ValueError("commercial/legal evidence incomplete: " + ",".join(missing))

    review_reference = evidence.get("review_reference")
    if not isinstance(review_reference, str) or not review_reference.strip():
        raise ValueError("qualified legal review reference is required")

    documents = evidence.get("effective_documents")
    if not isinstance(documents, list):
        raise ValueError("effective legal document list is required")
    normalized = {str(item).strip().lower() for item in documents}
    if not REQUIRED_DOCUMENTS.issubset(normalized):
        missing_docs = sorted(REQUIRED_DOCUMENTS - normalized)
        raise ValueError("effective legal documents incomplete: " + ",".join(missing_docs))

    return canonical_digest(evidence)


def finalize(engineering: dict, commercial: dict) -> tuple[dict, dict]:
    candidate_sha, gates = validate_engineering(engineering)
    commercial_digest = validate_commercial(commercial, candidate_sha)

    final_gates: list[dict] = []
    for item in gates:
        if item["gate"] == "commercial_legal_clearance":
            final_gates.append(
                {
                    "gate": "commercial_legal_clearance",
                    "status": "pass",
                    "candidate_sha": candidate_sha,
                    "evidence_digest": commercial_digest,
                }
            )
        else:
            final_gates.append(dict(item))

    final_readiness = dict(engineering)
    final_readiness["gates"] = final_gates

    authority = {
        "schema": AUTHORITY_SCHEMA,
        "candidate_version": CANDIDATE_VERSION,
        "candidate_sha": candidate_sha,
        "publication_authority": True,
        "commercial_legal_evidence_digest": commercial_digest,
    }
    return final_readiness, authority


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--engineering-readiness", required=True)
    parser.add_argument("--commercial-evidence", required=True)
    parser.add_argument("--readiness-output", required=True)
    parser.add_argument("--authority-output", required=True)
    args = parser.parse_args()

    engineering = load_object(Path(args.engineering_readiness))
    commercial = load_object(Path(args.commercial_evidence))
    readiness, authority = finalize(engineering, commercial)

    Path(args.readiness_output).write_text(
        json.dumps(readiness, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )
    Path(args.authority_output).write_text(
        json.dumps(authority, sort_keys=True, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )

    print("COMMERCIAL_LEGAL_CLEARANCE=PASS")
    print("PUBLICATION_AUTHORITY=GRANTED")
    print(f"COMMERCIAL_LEGAL_EVIDENCE_DIGEST={authority['commercial_legal_evidence_digest']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
