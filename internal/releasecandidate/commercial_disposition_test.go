package releasecandidate

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateCommercialLegalDispositionProducesDeterministicExactCandidateDigest(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC)
	first := validCommercialLegalDisposition(now)
	second := validCommercialLegalDisposition(now)
	second.MarketScope = []string{"Russian Federation", "European Union"}
	for left, right := 0, len(second.Evidence)-1; left < right; left, right = left+1, right-1 {
		second.Evidence[left], second.Evidence[right] = second.Evidence[right], second.Evidence[left]
	}

	resultA, err := EvaluateCommercialLegalDisposition(first, now)
	if err != nil {
		t.Fatalf("EvaluateCommercialLegalDisposition(first) error = %v", err)
	}
	resultB, err := EvaluateCommercialLegalDisposition(second, now)
	if err != nil {
		t.Fatalf("EvaluateCommercialLegalDisposition(second) error = %v", err)
	}
	if !resultA.Approved || resultA.CandidateSHA != first.CandidateSHA {
		t.Fatalf("unexpected result: %+v", resultA)
	}
	if !digestRE.MatchString(resultA.EvidenceDigest) {
		t.Fatalf("unexpected evidence digest: %q", resultA.EvidenceDigest)
	}
	if resultA.EvidenceDigest != resultB.EvidenceDigest {
		t.Fatalf("digest must be independent of market/evidence ordering: %q != %q", resultA.EvidenceDigest, resultB.EvidenceDigest)
	}
}

func TestEvaluateCommercialLegalDispositionRejectsWrongCandidateIdentity(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC)
	disposition := validCommercialLegalDisposition(now)
	disposition.CandidateVersion = "0.32.0"
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "candidate version") {
		t.Fatalf("expected candidate version failure, got %v", err)
	}

	disposition = validCommercialLegalDisposition(now)
	disposition.CandidateSHA = "not-a-sha"
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "candidate sha") {
		t.Fatalf("expected candidate sha failure, got %v", err)
	}
}

func TestEvaluateCommercialLegalDispositionRejectsNonApprovedOrExpiredEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC)
	disposition := validCommercialLegalDisposition(now)
	disposition.Decision = "pending"
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "decision must be approved") {
		t.Fatalf("expected approval failure, got %v", err)
	}

	disposition = validCommercialLegalDisposition(now)
	expired := now.Add(-time.Minute)
	disposition.ExpiresAt = &expired
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expiry failure, got %v", err)
	}

	disposition = validCommercialLegalDisposition(now)
	disposition.ApprovedAt = now.Add(time.Minute)
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "approved_at") {
		t.Fatalf("expected future approval failure, got %v", err)
	}
}

func TestEvaluateCommercialLegalDispositionRejectsIncompleteOrAmbiguousEvidence(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC)
	disposition := validCommercialLegalDisposition(now)
	disposition.Evidence = disposition.Evidence[:len(disposition.Evidence)-1]
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "evidence references") {
		t.Fatalf("expected incomplete evidence failure, got %v", err)
	}

	disposition = validCommercialLegalDisposition(now)
	disposition.Evidence[1].Kind = disposition.Evidence[0].Kind
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "duplicate evidence kind") {
		t.Fatalf("expected duplicate evidence failure, got %v", err)
	}

	disposition = validCommercialLegalDisposition(now)
	disposition.Evidence[0].Digest = "sha256:bad"
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "invalid digest") {
		t.Fatalf("expected evidence digest failure, got %v", err)
	}
}

func TestEvaluateCommercialLegalDispositionRejectsNonCanonicalCommercialMetadata(t *testing.T) {
	now := time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC)
	disposition := validCommercialLegalDisposition(now)
	disposition.LegalEntity = " Example LLC "
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "legal entity") {
		t.Fatalf("expected canonical legal entity failure, got %v", err)
	}

	disposition = validCommercialLegalDisposition(now)
	disposition.MarketScope = append(disposition.MarketScope, disposition.MarketScope[0])
	if _, err := EvaluateCommercialLegalDisposition(disposition, now); err == nil || !strings.Contains(err.Error(), "duplicate market scope") {
		t.Fatalf("expected duplicate market failure, got %v", err)
	}
}

func TestRequiredCommercialEvidenceReturnsDefensiveCopy(t *testing.T) {
	first := RequiredCommercialEvidence()
	if len(first) == 0 {
		t.Fatal("required commercial evidence must not be empty")
	}
	first[0] = "tampered"
	second := RequiredCommercialEvidence()
	if second[0] == "tampered" {
		t.Fatal("required commercial evidence leaked mutable package state")
	}
}

func validCommercialLegalDisposition(now time.Time) CommercialLegalDisposition {
	kinds := RequiredCommercialEvidence()
	evidence := make([]CommercialEvidenceRef, 0, len(kinds))
	for index, kind := range kinds {
		char := byte('a' + index)
		evidence = append(evidence, CommercialEvidenceRef{
			Kind:     kind,
			Revision: "approved-r1",
			Digest:   "sha256:" + strings.Repeat(string(char), 64),
		})
	}
	expires := now.Add(30 * 24 * time.Hour)
	return CommercialLegalDisposition{
		Schema:           CommercialLegalDispositionSchemaV1,
		CandidateVersion: CandidateVersion,
		CandidateSHA:     strings.Repeat("a", 40),
		Decision:         "approved",
		LegalEntity:      "Example LLC",
		Licensor:         "Example LLC",
		GoverningLaw:     "Approved governing law reference",
		MarketScope:      []string{"European Union", "Russian Federation"},
		CustomerModel:    CommercialCustomerB2BB2C,
		AuthorityID:      "legal-review-board-2026-09",
		ApprovedAt:       now.Add(-time.Hour),
		ExpiresAt:        &expires,
		Evidence:         evidence,
	}
}
