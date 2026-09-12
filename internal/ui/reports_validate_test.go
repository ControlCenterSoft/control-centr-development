package ui

import (
	"testing"
	"time"
)

func validatedReportFixture(t *testing.T) OperationalReport {
	t.Helper()
	now := time.Date(2026, 9, 13, 1, 30, 0, 0, time.UTC)
	report, err := BuildOperationalReport(now, ReportDataLoaded, []ReportEvidenceInput{
		{
			ID: "restore-node-a", Kind: "restore-freshness", ResourceKind: "node", ResourceID: "node-a",
			ObservedState: ReportHealthHealthy, Freshness: ReportFreshnessStale, ObservedAt: now.Add(-time.Hour),
			ReasonCode: "restore.drill.stale", EvidenceRefs: []string{"evidence:a", "evidence:b"},
		},
		{
			ID: "audit-installation", Kind: "audit-integrity", ResourceKind: "installation", ResourceID: "local",
			ObservedState: ReportHealthHealthy, Freshness: ReportFreshnessCurrent, ObservedAt: now.Add(-time.Minute),
			ReasonCode: "audit.integrity.confirmed",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestValidateOperationalReportAcceptsCanonicalBuilderOutput(t *testing.T) {
	report := validatedReportFixture(t)
	if err := ValidateOperationalReport(report); err != nil {
		t.Fatalf("ValidateOperationalReport() error = %v", err)
	}
	if _, err := BuildValidatedEvidenceDrawer(report, "node", "node-a"); err != nil {
		t.Fatalf("BuildValidatedEvidenceDrawer() error = %v", err)
	}
}

func TestValidateOperationalReportRejectsTamperedEvidenceWithStaleDigest(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[0].ResourceID = "node-attacker"
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("tampered evidence accepted")
	}
}

func TestValidateOperationalReportRejectsForgedEffectiveHealthy(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[0].EffectiveState = ReportHealthHealthy
	report.EvidenceDigest = digestReportEvidence(report.Evidence)
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("forged effective state accepted")
	}
}

func TestValidateOperationalReportRejectsDuplicateIdentityEvenWithRecomputedDigest(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[1] = report.Evidence[0]
	report.EvidenceDigest = digestReportEvidence(report.Evidence)
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("duplicate evidence identity accepted")
	}
}

func TestValidateOperationalReportRejectsFutureObservation(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[0].ObservedAt = report.GeneratedAt.Add(time.Second)
	report.EvidenceDigest = digestReportEvidence(report.Evidence)
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("future evidence accepted")
	}
}

func TestValidateOperationalReportRejectsNonCanonicalReferenceOrder(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[0].EvidenceRefs = []string{"evidence:b", "evidence:a"}
	report.EvidenceDigest = digestReportEvidence(report.Evidence)
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("non-canonical evidence refs accepted")
	}
}

func TestValidateOperationalReportRejectsNonCanonicalEvidenceOrder(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[0], report.Evidence[1] = report.Evidence[1], report.Evidence[0]
	report.EvidenceDigest = digestReportEvidence(report.Evidence)
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("non-canonical evidence ordering accepted")
	}
}

func TestValidateOperationalReportRejectsForgedOverallState(t *testing.T) {
	report := validatedReportFixture(t)
	report.OverallState = ReportHealthHealthy
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("forged overall state accepted")
	}
}

func TestValidateOperationalReportRejectsUnavailableWithEvidence(t *testing.T) {
	report := validatedReportFixture(t)
	report.DataState = ReportDataUnavailable
	if err := ValidateOperationalReport(report); err == nil {
		t.Fatal("unavailable report carrying evidence accepted")
	}
}

func TestBuildValidatedEvidenceDrawerRejectsTamperedTransportReport(t *testing.T) {
	report := validatedReportFixture(t)
	report.Evidence[0].EffectiveState = ReportHealthHealthy
	report.EvidenceDigest = digestReportEvidence(report.Evidence)
	if _, err := BuildValidatedEvidenceDrawer(report, "node", "node-a"); err == nil {
		t.Fatal("drawer accepted transport-tampered report")
	}
}
