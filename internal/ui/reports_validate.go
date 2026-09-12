package ui

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ValidateOperationalReport revalidates an OperationalReport after any
// transport/storage boundary. The report digest is an integrity binding for
// the canonical projection; it is not an authenticity/signature mechanism.
func ValidateOperationalReport(report OperationalReport) error {
	if report.Schema != OperationalReportSchema {
		return errors.New("unsupported report schema")
	}
	if report.MutationAuthorized {
		return errors.New("report unexpectedly carries mutation authority")
	}
	if report.GeneratedAt.IsZero() {
		return errors.New("generated_at is required")
	}
	if report.DataState != ReportDataLoaded && report.DataState != ReportDataUnavailable {
		return fmt.Errorf("unsupported report data state %q", report.DataState)
	}
	if !validReportState(report.OverallState) {
		return fmt.Errorf("unsupported overall state %q", report.OverallState)
	}
	if len(report.Evidence) > MaxReportItems {
		return fmt.Errorf("evidence count exceeds %d", MaxReportItems)
	}
	if !validSHA256Digest(report.EvidenceDigest) {
		return errors.New("evidence_digest must be sha256:<64 lowercase hex>")
	}

	if report.DataState == ReportDataUnavailable {
		if len(report.Evidence) != 0 {
			return errors.New("unavailable report cannot contain evidence")
		}
		if report.OverallState != ReportHealthUnknown {
			return errors.New("unavailable report must have unknown overall state")
		}
		if report.EvidenceDigest != digestReportEvidence([]ReportEvidence{}) {
			return errors.New("report evidence digest mismatch")
		}
		return nil
	}

	if len(report.Evidence) == 0 {
		if report.OverallState != ReportHealthUnknown {
			return errors.New("empty report must have unknown overall state")
		}
		if report.EvidenceDigest != digestReportEvidence([]ReportEvidence{}) {
			return errors.New("report evidence digest mismatch")
		}
		return nil
	}

	canonical := make([]ReportEvidence, 0, len(report.Evidence))
	seen := make(map[string]struct{}, len(report.Evidence))
	for _, item := range report.Evidence {
		built, err := buildReportEvidence(report.GeneratedAt, ReportEvidenceInput{
			ID:             item.ID,
			Kind:           item.Kind,
			ResourceKind:   item.ResourceKind,
			ResourceID:     item.ResourceID,
			ObservedState:  item.ObservedState,
			Freshness:      item.Freshness,
			ObservedAt:     item.ObservedAt,
			ReasonCode:     item.ReasonCode,
			RunbookRef:     item.RunbookRef,
			EvidenceRefs:   item.EvidenceRefs,
			EvidenceDigest: item.EvidenceDigest,
		})
		if err != nil {
			return fmt.Errorf("invalid evidence %q: %w", item.ID, err)
		}
		if _, exists := seen[built.ID]; exists {
			return fmt.Errorf("duplicate evidence id %q", built.ID)
		}
		seen[built.ID] = struct{}{}
		if !reportEvidenceCanonical(item, built) {
			return fmt.Errorf("evidence %q is not canonical", built.ID)
		}
		canonical = append(canonical, built)
	}

	canonicalSortReportEvidence(canonical)
	for i := range canonical {
		if !reportEvidenceCanonical(report.Evidence[i], canonical[i]) {
			return errors.New("report evidence ordering is not canonical")
		}
	}
	if report.OverallState != canonicalAggregateReportState(canonical) {
		return errors.New("report overall state mismatch")
	}
	if report.EvidenceDigest != digestReportEvidence(canonical) {
		return errors.New("report evidence digest mismatch")
	}
	return nil
}

// BuildValidatedEvidenceDrawer is the required consumer boundary for reports
// obtained from JSON, persistence, IPC or any other untrusted boundary.
func BuildValidatedEvidenceDrawer(report OperationalReport, resourceKind, resourceID string) (EvidenceDrawer, error) {
	if err := ValidateOperationalReport(report); err != nil {
		return EvidenceDrawer{}, err
	}
	return BuildEvidenceDrawer(report, resourceKind, resourceID)
}

func reportEvidenceCanonical(got, want ReportEvidence) bool {
	if got.ID != want.ID ||
		got.Kind != want.Kind ||
		got.ResourceKind != want.ResourceKind ||
		got.ResourceID != want.ResourceID ||
		got.ObservedState != want.ObservedState ||
		got.EffectiveState != want.EffectiveState ||
		got.Freshness != want.Freshness ||
		!sameInstant(got.ObservedAt, want.ObservedAt) ||
		got.ReasonCode != want.ReasonCode ||
		got.RunbookRef != want.RunbookRef ||
		got.EvidenceDigest != want.EvidenceDigest ||
		len(got.EvidenceRefs) != len(want.EvidenceRefs) {
		return false
	}
	for i := range got.EvidenceRefs {
		if got.EvidenceRefs[i] != want.EvidenceRefs[i] {
			return false
		}
	}
	return true
}

func sameInstant(left, right time.Time) bool {
	return left.Equal(right)
}

func canonicalSortReportEvidence(evidence []ReportEvidence) {
	sort.Slice(evidence, func(i, j int) bool {
		left, right := evidence[i], evidence[j]
		if reportStateRank(left.EffectiveState) != reportStateRank(right.EffectiveState) {
			return reportStateRank(left.EffectiveState) > reportStateRank(right.EffectiveState)
		}
		if left.ResourceKind != right.ResourceKind {
			return left.ResourceKind < right.ResourceKind
		}
		if left.ResourceID != right.ResourceID {
			return left.ResourceID < right.ResourceID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.ID < right.ID
	})
}

func canonicalAggregateReportState(evidence []ReportEvidence) ReportHealthState {
	if len(evidence) == 0 {
		return ReportHealthUnknown
	}
	state := ReportHealthHealthy
	for _, item := range evidence {
		if reportStateRank(item.EffectiveState) > reportStateRank(state) {
			state = item.EffectiveState
		}
	}
	return state
}
