package releasecandidate

import (
	"strings"
	"testing"
	"time"
)

func TestBindCommercialLegalDispositionClosesOnlyCommercialGate(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 15, 0, 0, time.UTC)
	snapshot := readinessSnapshotWithCommercialBlocked(strings.Repeat("a", 40))
	disposition := validCommercialLegalDisposition(now)

	before, err := Evaluate(snapshot)
	if err != nil {
		t.Fatalf("Evaluate(before) error = %v", err)
	}
	if before.Ready || len(before.Blockers) != 1 || before.Blockers[0] != GateCommercialLegal {
		t.Fatalf("expected commercial-only blocker before binding, got %+v", before)
	}

	bound, err := BindCommercialLegalDisposition(snapshot, disposition, now)
	if err != nil {
		t.Fatalf("BindCommercialLegalDisposition() error = %v", err)
	}
	after, err := Evaluate(bound)
	if err != nil {
		t.Fatalf("Evaluate(after) error = %v", err)
	}
	if !after.Ready || len(after.Blockers) != 0 {
		t.Fatalf("expected fully ready snapshot after external approval binding, got %+v", after)
	}
	if snapshot.Gates[len(snapshot.Gates)-2].Gate == GateCommercialLegal && snapshot.Gates[len(snapshot.Gates)-2].Status == GatePass {
		t.Fatal("BindCommercialLegalDisposition mutated input snapshot")
	}
	commercial, ok := findGate(bound.Gates, GateCommercialLegal)
	if !ok || commercial.Status != GatePass || !digestRE.MatchString(commercial.EvidenceDigest) {
		t.Fatalf("commercial gate was not bound correctly: %+v", commercial)
	}
}

func TestBindCommercialLegalDispositionRejectsDifferentCandidate(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 15, 0, 0, time.UTC)
	snapshot := readinessSnapshotWithCommercialBlocked(strings.Repeat("b", 40))
	disposition := validCommercialLegalDisposition(now)

	_, err := BindCommercialLegalDisposition(snapshot, disposition, now)
	if err == nil || !strings.Contains(err.Error(), "different candidate sha") {
		t.Fatalf("expected candidate mismatch failure, got %v", err)
	}
}

func TestBindCommercialLegalDispositionRejectsConflictingExistingPass(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 15, 0, 0, time.UTC)
	snapshot := readinessSnapshotWithCommercialBlocked(strings.Repeat("a", 40))
	for index := range snapshot.Gates {
		if snapshot.Gates[index].Gate == GateCommercialLegal {
			snapshot.Gates[index].Status = GatePass
			snapshot.Gates[index].EvidenceDigest = "sha256:" + strings.Repeat("f", 64)
		}
	}

	_, err := BindCommercialLegalDisposition(snapshot, validCommercialLegalDisposition(now), now)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflicting clearance failure, got %v", err)
	}
}

func TestBindCommercialLegalDispositionIsIdempotentForSameApproval(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 15, 0, 0, time.UTC)
	snapshot := readinessSnapshotWithCommercialBlocked(strings.Repeat("a", 40))
	disposition := validCommercialLegalDisposition(now)
	bound, err := BindCommercialLegalDisposition(snapshot, disposition, now)
	if err != nil {
		t.Fatalf("first bind error = %v", err)
	}
	boundAgain, err := BindCommercialLegalDisposition(bound, disposition, now)
	if err != nil {
		t.Fatalf("second bind error = %v", err)
	}
	first, _ := findGate(bound.Gates, GateCommercialLegal)
	second, _ := findGate(boundAgain.Gates, GateCommercialLegal)
	if first != second {
		t.Fatalf("idempotent binding changed gate evidence: %+v != %+v", first, second)
	}
}

func readinessSnapshotWithCommercialBlocked(candidateSHA string) Snapshot {
	gates := make([]GateEvidence, 0, len(RequiredGates()))
	for _, gate := range RequiredGates() {
		status := GatePass
		digest := "sha256:" + strings.Repeat("b", 64)
		if gate == GateCommercialLegal {
			status = GateBlocked
			digest = ""
		}
		gates = append(gates, GateEvidence{
			Gate:           gate,
			Status:         status,
			CandidateSHA:   candidateSHA,
			EvidenceDigest: digest,
		})
	}
	return Snapshot{
		Schema:               SchemaV1,
		StableVersion:        StableVersion,
		StableTag:            StableTag,
		StableArtifactDigest: StableArtifactDigest,
		CandidateVersion:     CandidateVersion,
		CandidateSHA:         candidateSHA,
		Gates:                gates,
	}
}

func findGate(gates []GateEvidence, gateID GateID) (GateEvidence, bool) {
	for _, gate := range gates {
		if gate.Gate == gateID {
			return gate, true
		}
	}
	return GateEvidence{}, false
}
