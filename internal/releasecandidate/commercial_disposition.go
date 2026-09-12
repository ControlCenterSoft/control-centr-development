package releasecandidate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const CommercialLegalDispositionSchemaV1 = "control-center.commercial-legal-disposition.v1"

const (
	maxCommercialTextBytes       = 256
	maxCommercialMarkets         = 32
	maxCommercialRevisionBytes   = 128
	maxCommercialAuthorityIDBytes = 128
)

type CommercialCustomerModel string

const (
	CommercialCustomerB2B    CommercialCustomerModel = "b2b"
	CommercialCustomerB2C    CommercialCustomerModel = "b2c"
	CommercialCustomerB2BB2C CommercialCustomerModel = "b2b_b2c"
)

type CommercialEvidenceKind string

const (
	CommercialEvidenceLegalEntityAuthority       CommercialEvidenceKind = "legal_entity_authority"
	CommercialEvidenceGoverningLawMarketScope    CommercialEvidenceKind = "governing_law_market_scope"
	CommercialEvidenceEULA                       CommercialEvidenceKind = "eula"
	CommercialEvidenceTerms                      CommercialEvidenceKind = "terms"
	CommercialEvidencePrivacy                    CommercialEvidenceKind = "privacy"
	CommercialEvidenceSupportPolicy              CommercialEvidenceKind = "support_policy"
	CommercialEvidenceSLA                        CommercialEvidenceKind = "sla"
	CommercialEvidenceLicensingModel             CommercialEvidenceKind = "licensing_model"
	CommercialEvidencePricingBillingRefund       CommercialEvidenceKind = "pricing_billing_refund"
	CommercialEvidenceQualifiedLegalReview       CommercialEvidenceKind = "qualified_legal_review"
	CommercialEvidenceSecurityPrivacyDisposition CommercialEvidenceKind = "security_privacy_disposition"
)

var requiredCommercialEvidence = [...]CommercialEvidenceKind{
	CommercialEvidenceLegalEntityAuthority,
	CommercialEvidenceGoverningLawMarketScope,
	CommercialEvidenceEULA,
	CommercialEvidenceTerms,
	CommercialEvidencePrivacy,
	CommercialEvidenceSupportPolicy,
	CommercialEvidenceSLA,
	CommercialEvidenceLicensingModel,
	CommercialEvidencePricingBillingRefund,
	CommercialEvidenceQualifiedLegalReview,
	CommercialEvidenceSecurityPrivacyDisposition,
}

type CommercialEvidenceRef struct {
	Kind     CommercialEvidenceKind `json:"kind"`
	Revision string                 `json:"revision"`
	Digest   string                 `json:"digest"`
}

type CommercialLegalDisposition struct {
	Schema           string                   `json:"schema"`
	CandidateVersion string                   `json:"candidate_version"`
	CandidateSHA     string                   `json:"candidate_sha"`
	Decision         string                   `json:"decision"`
	LegalEntity      string                   `json:"legal_entity"`
	Licensor         string                   `json:"licensor"`
	GoverningLaw     string                   `json:"governing_law"`
	MarketScope      []string                 `json:"market_scope"`
	CustomerModel    CommercialCustomerModel  `json:"customer_model"`
	AuthorityID      string                   `json:"authority_id"`
	ApprovedAt       time.Time                `json:"approved_at"`
	ExpiresAt        *time.Time               `json:"expires_at,omitempty"`
	Evidence         []CommercialEvidenceRef  `json:"evidence"`
}

type CommercialLegalDispositionResult struct {
	Approved       bool   `json:"approved"`
	CandidateSHA   string `json:"candidate_sha"`
	EvidenceDigest string `json:"evidence_digest"`
}

// EvaluateCommercialLegalDisposition validates an externally authorized legal
// and commercial disposition without creating that authorization. Only a
// complete, exact-candidate, non-expired "approved" disposition can produce a
// digest suitable for the commercial_legal_clearance release gate. The digest
// is deterministic across evidence/market input ordering.
func EvaluateCommercialLegalDisposition(disposition CommercialLegalDisposition, now time.Time) (CommercialLegalDispositionResult, error) {
	if now.IsZero() {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: current time is required")
	}
	now = now.UTC()
	if disposition.Schema != CommercialLegalDispositionSchemaV1 {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: unsupported schema %q", disposition.Schema)
	}
	if disposition.CandidateVersion != CandidateVersion {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: unexpected candidate version %q", disposition.CandidateVersion)
	}
	if !shaRE.MatchString(disposition.CandidateSHA) {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: invalid candidate sha")
	}
	if disposition.Decision != "approved" {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: decision must be approved")
	}
	if err := validateCommercialCanonicalText("legal entity", disposition.LegalEntity, maxCommercialTextBytes); err != nil {
		return CommercialLegalDispositionResult{}, err
	}
	if err := validateCommercialCanonicalText("licensor", disposition.Licensor, maxCommercialTextBytes); err != nil {
		return CommercialLegalDispositionResult{}, err
	}
	if err := validateCommercialCanonicalText("governing law", disposition.GoverningLaw, maxCommercialTextBytes); err != nil {
		return CommercialLegalDispositionResult{}, err
	}
	if err := validateCommercialCanonicalText("authority id", disposition.AuthorityID, maxCommercialAuthorityIDBytes); err != nil {
		return CommercialLegalDispositionResult{}, err
	}
	switch disposition.CustomerModel {
	case CommercialCustomerB2B, CommercialCustomerB2C, CommercialCustomerB2BB2C:
	default:
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: unsupported customer model %q", disposition.CustomerModel)
	}
	markets, err := normalizeCommercialMarkets(disposition.MarketScope)
	if err != nil {
		return CommercialLegalDispositionResult{}, err
	}
	approvedAt := disposition.ApprovedAt.UTC()
	if disposition.ApprovedAt.IsZero() || approvedAt.After(now) {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: approved_at is invalid")
	}
	var expiresAt *time.Time
	if disposition.ExpiresAt != nil {
		if disposition.ExpiresAt.IsZero() {
			return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: expires_at is invalid")
		}
		normalized := disposition.ExpiresAt.UTC()
		if !normalized.After(approvedAt) || !normalized.After(now) {
			return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: clearance is expired or has invalid expiry")
		}
		expiresAt = &normalized
	}
	evidence, err := normalizeCommercialEvidence(disposition.Evidence)
	if err != nil {
		return CommercialLegalDispositionResult{}, err
	}

	normalized := CommercialLegalDisposition{
		Schema:           CommercialLegalDispositionSchemaV1,
		CandidateVersion: CandidateVersion,
		CandidateSHA:     disposition.CandidateSHA,
		Decision:         "approved",
		LegalEntity:      disposition.LegalEntity,
		Licensor:         disposition.Licensor,
		GoverningLaw:     disposition.GoverningLaw,
		MarketScope:      markets,
		CustomerModel:    disposition.CustomerModel,
		AuthorityID:      disposition.AuthorityID,
		ApprovedAt:       approvedAt,
		ExpiresAt:        expiresAt,
		Evidence:         evidence,
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return CommercialLegalDispositionResult{}, fmt.Errorf("commercial legal disposition: marshal normalized evidence: %w", err)
	}
	sum := sha256.Sum256(raw)
	return CommercialLegalDispositionResult{
		Approved:       true,
		CandidateSHA:   disposition.CandidateSHA,
		EvidenceDigest: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

func RequiredCommercialEvidence() []CommercialEvidenceKind {
	return append([]CommercialEvidenceKind(nil), requiredCommercialEvidence[:]...)
}

func normalizeCommercialMarkets(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maxCommercialMarkets {
		return nil, fmt.Errorf("commercial legal disposition: market_scope must contain between 1 and %d entries", maxCommercialMarkets)
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if err := validateCommercialCanonicalText("market scope", value, maxCommercialTextBytes); err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("commercial legal disposition: duplicate market scope %q", value)
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeCommercialEvidence(values []CommercialEvidenceRef) ([]CommercialEvidenceRef, error) {
	if len(values) != len(requiredCommercialEvidence) {
		return nil, fmt.Errorf("commercial legal disposition: expected %d evidence references, got %d", len(requiredCommercialEvidence), len(values))
	}
	required := make(map[CommercialEvidenceKind]struct{}, len(requiredCommercialEvidence))
	for _, kind := range requiredCommercialEvidence {
		required[kind] = struct{}{}
	}
	seen := make(map[CommercialEvidenceKind]struct{}, len(values))
	out := make([]CommercialEvidenceRef, 0, len(values))
	for _, value := range values {
		if _, ok := required[value.Kind]; !ok {
			return nil, fmt.Errorf("commercial legal disposition: unknown evidence kind %q", value.Kind)
		}
		if _, duplicate := seen[value.Kind]; duplicate {
			return nil, fmt.Errorf("commercial legal disposition: duplicate evidence kind %q", value.Kind)
		}
		seen[value.Kind] = struct{}{}
		if err := validateCommercialCanonicalText("evidence revision", value.Revision, maxCommercialRevisionBytes); err != nil {
			return nil, err
		}
		if !digestRE.MatchString(value.Digest) {
			return nil, fmt.Errorf("commercial legal disposition: evidence %q has invalid digest", value.Kind)
		}
		out = append(out, value)
	}
	for _, kind := range requiredCommercialEvidence {
		if _, ok := seen[kind]; !ok {
			return nil, fmt.Errorf("commercial legal disposition: missing evidence kind %q", kind)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}

func validateCommercialCanonicalText(label, value string, maxBytes int) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return fmt.Errorf("commercial legal disposition: %s must be canonical and non-empty", label)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("commercial legal disposition: %s exceeds %d bytes", label, maxBytes)
	}
	return nil
}
