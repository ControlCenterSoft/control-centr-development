package releasecandidate

import (
	"strings"
	"testing"
)

func complete032Snapshot() Snapshot {
	candidateSHA := strings.Repeat("d", 40)
	digest := "sha256:" + strings.Repeat("e", 64)
	gates := make([]GateEvidence, 0, len(RequiredGates032()))
	for _, gate := range RequiredGates032() {
		gates = append(gates, GateEvidence{
			Gate:           gate,
			Status:         GatePass,
			CandidateSHA:   candidateSHA,
			EvidenceDigest: digest,
		})
	}
	return Snapshot{
		Schema:               Schema032V1,
		StableVersion:        Stable032BaseVersion,
		StableTag:            Stable032BaseTag,
		StableArtifactDigest: Stable032ArtifactDigest,
		CandidateVersion:     Candidate032Version,
		CandidateSHA:         candidateSHA,
		Gates:                gates,
	}
}

func TestEvaluate032ReadyOnlyWhenEveryRequiredGatePasses(t *testing.T) {
	result, err := Evaluate032(complete032Snapshot())
	if err != nil {
		t.Fatalf("Evaluate032() error = %v", err)
	}
	if !result.Ready || len(result.Blockers) != 0 {
		t.Fatalf("result = %+v, want ready", result)
	}
}

func TestEvaluateProductStable032DoesNotInventCommercialClearance(t *testing.T) {
	snapshot := complete032Snapshot()
	for i := range snapshot.Gates {
		if snapshot.Gates[i].Gate == GateCommercialLegal {
			snapshot.Gates[i].Status = GateBlocked
			snapshot.Gates[i].EvidenceDigest = ""
		}
	}

	full, err := Evaluate032(snapshot)
	if err != nil {
		t.Fatalf("Evaluate032() error = %v", err)
	}
	if full.Ready || len(full.Blockers) != 1 || full.Blockers[0] != GateCommercialLegal {
		t.Fatalf("commercial result = %+v", full)
	}

	product, err := EvaluateProductStable032(snapshot)
	if err != nil {
		t.Fatalf("EvaluateProductStable032() error = %v", err)
	}
	if !product.Ready || len(product.Blockers) != 0 {
		t.Fatalf("product result = %+v, want technically ready", product)
	}
}

func TestEvaluateProductStable032StillFailsClosedOnTechnicalGate(t *testing.T) {
	snapshot := complete032Snapshot()
	for i := range snapshot.Gates {
		if snapshot.Gates[i].Gate == GateSecurityPrivacy {
			snapshot.Gates[i].Status = GateBlocked
			snapshot.Gates[i].EvidenceDigest = ""
		}
	}
	result, err := EvaluateProductStable032(snapshot)
	if err != nil {
		t.Fatalf("EvaluateProductStable032() error = %v", err)
	}
	if result.Ready || len(result.Blockers) != 1 || result.Blockers[0] != GateSecurityPrivacy {
		t.Fatalf("result = %+v, want security blocker", result)
	}
}

func TestEvaluate032RejectsWrongStableIdentityOrArtifact(t *testing.T) {
	snapshot := complete032Snapshot()
	snapshot.StableVersion = "0.31.0"
	if _, err := Evaluate032(snapshot); err == nil {
		t.Fatal("superseded stable version accepted")
	}

	snapshot = complete032Snapshot()
	snapshot.StableTag = "v0.31.0"
	if _, err := Evaluate032(snapshot); err == nil {
		t.Fatal("superseded stable tag accepted")
	}

	snapshot = complete032Snapshot()
	snapshot.StableArtifactDigest = "sha256:" + strings.Repeat("f", 64)
	if _, err := Evaluate032(snapshot); err == nil {
		t.Fatal("wrong stable artifact digest accepted")
	}
}

func TestEvaluate032RejectsCrossCandidateAndUnknownEvidence(t *testing.T) {
	snapshot := complete032Snapshot()
	snapshot.Gates[0].CandidateSHA = strings.Repeat("a", 40)
	if _, err := Evaluate032(snapshot); err == nil {
		t.Fatal("cross-candidate evidence accepted")
	}

	snapshot = complete032Snapshot()
	snapshot.Gates[0].Gate = GateID("unreviewed_future_gate")
	if _, err := Evaluate032(snapshot); err == nil {
		t.Fatal("unknown gate accepted")
	}
}

func TestEvaluate032PassRequiresBoundedDigest(t *testing.T) {
	snapshot := complete032Snapshot()
	snapshot.Gates[0].EvidenceDigest = ""
	if _, err := Evaluate032(snapshot); err == nil {
		t.Fatal("PASS without digest accepted")
	}
}

func TestRequired032PoliciesReturnDefensiveCopies(t *testing.T) {
	all := RequiredGates032()
	all[0] = GateID("weakened")
	if RequiredGates032()[0] != GateHealthIncidentsAuditReports {
		t.Fatal("RequiredGates032 policy mutated through caller")
	}

	product := ProductStableRequiredGates032()
	product[0] = GateID("weakened")
	if ProductStableRequiredGates032()[0] != GateHealthIncidentsAuditReports {
		t.Fatal("ProductStableRequiredGates032 policy mutated through caller")
	}
}

func complete032ArtifactManifest() ArtifactManifest {
	sha := strings.Repeat("d", 40)
	digest := "sha256:" + strings.Repeat("e", 64)
	artifacts := make([]ArtifactEvidence, 0, len(RequiredArtifactNames032()))
	for _, name := range RequiredArtifactNames032() {
		artifacts = append(artifacts, ArtifactEvidence{Name: name, Digest: digest})
	}
	return ArtifactManifest{
		Schema:           ArtifactManifestSchema032V1,
		CandidateVersion: Candidate032Version,
		CandidateSHA:     sha,
		Artifacts:        artifacts,
	}
}

func TestValidateArtifactManifest032AcceptsExactSet(t *testing.T) {
	if err := ValidateArtifactManifest032(complete032ArtifactManifest()); err != nil {
		t.Fatalf("ValidateArtifactManifest032() error = %v", err)
	}
}

func TestValidateArtifactManifest032RejectsMissingAndStableNames(t *testing.T) {
	manifest := complete032ArtifactManifest()
	manifest.Artifacts = manifest.Artifacts[:len(manifest.Artifacts)-1]
	if err := ValidateArtifactManifest032(manifest); err == nil {
		t.Fatal("missing required artifact accepted")
	}

	manifest = complete032ArtifactManifest()
	manifest.Artifacts[0].Name = "control-center-0.31.1-linux-amd64.tar.gz"
	if err := ValidateArtifactManifest032(manifest); err == nil {
		t.Fatal("Stable artifact accepted in 0.32 manifest")
	}
}

func TestRequiredArtifactNames032ReturnsDefensiveCopy(t *testing.T) {
	names := RequiredArtifactNames032()
	names[0] = "weakened"
	if RequiredArtifactNames032()[0] != "control-center-0.32.0-linux-amd64.tar.gz" {
		t.Fatal("RequiredArtifactNames032 mutated through caller")
	}
}
