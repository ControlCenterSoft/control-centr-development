package releasecandidate

import (
	"fmt"
	"time"
)

// BindCommercialLegalDisposition returns a copy of snapshot with the
// commercial_legal_clearance gate bound to an externally approved disposition.
// It never invents approval: the disposition must independently pass
// EvaluateCommercialLegalDisposition and must target the exact snapshot
// candidate SHA. Existing conflicting PASS evidence is rejected.
func BindCommercialLegalDisposition(snapshot Snapshot, disposition CommercialLegalDisposition, now time.Time) (Snapshot, error) {
	if _, err := Evaluate(snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("bind commercial legal disposition: invalid readiness snapshot: %w", err)
	}
	clearance, err := EvaluateCommercialLegalDisposition(disposition, now)
	if err != nil {
		return Snapshot{}, fmt.Errorf("bind commercial legal disposition: %w", err)
	}
	if clearance.CandidateSHA != snapshot.CandidateSHA {
		return Snapshot{}, fmt.Errorf("bind commercial legal disposition: disposition targets a different candidate sha")
	}

	gatesByID := make(map[GateID]GateEvidence, len(snapshot.Gates)+1)
	for _, gate := range snapshot.Gates {
		gatesByID[gate.Gate] = gate
	}
	if existing, ok := gatesByID[GateCommercialLegal]; ok && existing.Status == GatePass {
		if existing.EvidenceDigest != clearance.EvidenceDigest {
			return Snapshot{}, fmt.Errorf("bind commercial legal disposition: existing commercial clearance digest conflicts with approved disposition")
		}
		return cloneSnapshot(snapshot), nil
	}
	gatesByID[GateCommercialLegal] = GateEvidence{
		Gate:           GateCommercialLegal,
		Status:         GatePass,
		CandidateSHA:   snapshot.CandidateSHA,
		EvidenceDigest: clearance.EvidenceDigest,
	}

	bound := cloneSnapshot(snapshot)
	bound.Gates = make([]GateEvidence, 0, len(gatesByID))
	for _, gateID := range requiredGates {
		if gate, ok := gatesByID[gateID]; ok {
			bound.Gates = append(bound.Gates, gate)
		}
	}
	if _, err := Evaluate(bound); err != nil {
		return Snapshot{}, fmt.Errorf("bind commercial legal disposition: resulting readiness snapshot is invalid: %w", err)
	}
	return bound, nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Gates = append([]GateEvidence(nil), snapshot.Gates...)
	return clone
}
