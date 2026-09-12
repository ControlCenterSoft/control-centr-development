#!/usr/bin/env python3
from __future__ import annotations

import copy
import importlib.util
from pathlib import Path

MODULE_PATH = Path(__file__).with_name("finalize-release-evidence-031.py")
spec = importlib.util.spec_from_file_location("finalize_release_evidence_031", MODULE_PATH)
if spec is None or spec.loader is None:
    raise RuntimeError("cannot load release finalizer")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

SHA = "9b96caea77a31166af88d6ba512b05f327fd9bcc"
DIGEST = "sha256:" + "a" * 64


def engineering() -> dict:
    gates = []
    for gate in module.REQUIRED_GATES:
        item = {"gate": gate, "candidate_sha": SHA}
        if gate == "commercial_legal_clearance":
            item["status"] = "blocked"
        else:
            item["status"] = "pass"
            item["evidence_digest"] = DIGEST
        gates.append(item)
    return {
        "schema": module.SCHEMA,
        "stable_version": module.STABLE_VERSION,
        "stable_tag": module.STABLE_TAG,
        "stable_artifact_digest": module.STABLE_ARTIFACT_DIGEST,
        "candidate_version": module.CANDIDATE_VERSION,
        "candidate_sha": SHA,
        "gates": gates,
    }


def commercial() -> dict:
    data = {
        "schema": module.COMMERCIAL_SCHEMA,
        "candidate_version": module.CANDIDATE_VERSION,
        "candidate_sha": SHA,
        "disposition": "approved",
        "review_reference": "external-qualified-review-record",
        "effective_documents": ["eula", "terms_of_use", "privacy_policy", "support_terms"],
    }
    for flag in module.REQUIRED_COMMERCIAL_FLAGS:
        data[flag] = True
    return data


def expect_error(label: str, eng: dict, com: dict) -> None:
    try:
        module.finalize(eng, com)
    except ValueError:
        return
    raise AssertionError(f"{label}: expected fail-closed rejection")


def main() -> int:
    final_readiness, authority = module.finalize(engineering(), commercial())
    commercial_gate = next(item for item in final_readiness["gates"] if item["gate"] == "commercial_legal_clearance")
    assert commercial_gate["status"] == "pass"
    assert module.DIGEST_RE.fullmatch(commercial_gate["evidence_digest"])
    assert authority["candidate_sha"] == SHA
    assert authority["publication_authority"] is True
    assert authority["commercial_legal_evidence_digest"] == commercial_gate["evidence_digest"]

    bad = commercial()
    bad["legal_review_completed"] = False
    expect_error("missing legal review", engineering(), bad)

    bad = commercial()
    bad["publication_authority"] = False
    expect_error("missing publication authority", engineering(), bad)

    bad = commercial()
    bad["candidate_sha"] = "0" * 40
    expect_error("candidate mismatch", engineering(), bad)

    bad = commercial()
    bad["effective_documents"] = ["eula", "privacy_policy"]
    expect_error("missing legal documents", engineering(), bad)

    eng = engineering()
    next(item for item in eng["gates"] if item["gate"] == "security_privacy")["status"] = "blocked"
    expect_error("non-commercial gate blocked", eng, commercial())

    eng = engineering()
    eng["gates"].append(copy.deepcopy(eng["gates"][0]))
    expect_error("duplicate gate", eng, commercial())

    print("RELEASE_FINALIZER_CONTRACT_TESTS=PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
