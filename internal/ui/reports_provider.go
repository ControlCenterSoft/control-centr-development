package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// OperationalReportProvider exposes one already-authorized, read-only report
// source. Authorization deliberately remains outside this interface so Audit,
// infrastructure/health and other report sources can keep their own RBAC
// boundaries instead of being flattened into one weaker permission.
type OperationalReportProvider interface {
	OperationalReport(context.Context) (OperationalReport, error)
}

// ValidateOperationalReport rejects non-canonical or tampered report values at
// a storage/transport boundary. The canonical report is rebuilt from the
// evidence fields that are allowed to originate upstream and must match the
// supplied projection exactly, including ordering, effective state and digest.
//
// This function is read-only and never grants mutation or execution authority.
func ValidateOperationalReport(report OperationalReport) error {
	if report.Schema != OperationalReportSchema {
		return fmt.Errorf("unsupported operational report schema %q", report.Schema)
	}
	if report.MutationAuthorized {
		return errors.New("operational report unexpectedly carries mutation authority")
	}
	if report.GeneratedAt.IsZero() {
		return errors.New("operational report generated_at is required")
	}
	if report.Evidence == nil {
		return errors.New("operational report evidence must be an explicit array")
	}

	input := make([]ReportEvidenceInput, 0, len(report.Evidence))
	for _, item := range report.Evidence {
		input = append(input, ReportEvidenceInput{
			ID:             item.ID,
			Kind:           item.Kind,
			ResourceKind:   item.ResourceKind,
			ResourceID:     item.ResourceID,
			ObservedState:  item.ObservedState,
			Freshness:      item.Freshness,
			ObservedAt:     item.ObservedAt,
			ReasonCode:     item.ReasonCode,
			RunbookRef:     item.RunbookRef,
			EvidenceRefs:   append([]string(nil), item.EvidenceRefs...),
			EvidenceDigest: item.EvidenceDigest,
		})
	}

	canonical, err := BuildOperationalReport(report.GeneratedAt, report.DataState, input)
	if err != nil {
		return fmt.Errorf("invalid operational report: %w", err)
	}
	if !reflect.DeepEqual(report, canonical) {
		return errors.New("operational report is not canonical")
	}
	return nil
}
