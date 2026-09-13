package releasecandidate

import "fmt"

const (
	Schema032V1             = "control-center.release-candidate-readiness.0.32.v1"
	Stable032BaseVersion    = "0.31.1"
	Stable032BaseTag        = "v0.31.1"
	Stable032ArtifactDigest = "sha256:b9d6467c7c95a6e7e8597398c1b6e7327d319058d248e9cd0416c5baf9699c97"
	Candidate032Version     = "0.32.0"

	ArtifactManifestSchema032V1 = "control-center.release-candidate-artifacts.0.32.v1"
)

const (
	GateHealthIncidentsAuditReports GateID = "health_incidents_audit_reports_integration"
	GateUpgradeFromStable0311       GateID = "upgrade_from_stable_0_31_1"
)

var release032RequiredGates = [...]GateID{
	GateHealthIncidentsAuditReports,
	GatePackaging,
	GateCleanInstall,
	GateUpgradeFromStable0311,
	GateRollbackRecovery,
	GatePostgresRestart,
	GateSecurityPrivacy,
	GateCommercialLegal,
	GateReleaseMetadata,
}

// release032ProductStableRequiredGates intentionally excludes only the
// commercial/legal launch gate. Product Public Stable still requires every
// technical correctness, upgrade, recovery, security and release-metadata gate.
var release032ProductStableRequiredGates = [...]GateID{
	GateHealthIncidentsAuditReports,
	GatePackaging,
	GateCleanInstall,
	GateUpgradeFromStable0311,
	GateRollbackRecovery,
	GatePostgresRestart,
	GateSecurityPrivacy,
	GateReleaseMetadata,
}

var requiredArtifactNames032 = [...]string{
	"control-center-0.32.0-linux-amd64.tar.gz",
	"control-center-0.32.0-linux-amd64.tar.gz.sha256",
	"control-center-0.32.0-source.tar.gz",
	"control-center-0.32.0.sbom.cdx.json",
	"THIRD_PARTY_NOTICES.md",
	"control-center-0.32.0.provenance.json",
	"control-center-0.32.0.qualification.json",
	"control-center-0.32.0.release-manifest.json",
	"SHA256SUMS",
}

// RequiredGates032 returns the complete 0.32 commercial-readiness gate set.
func RequiredGates032() []GateID {
	return append([]GateID(nil), release032RequiredGates[:]...)
}

// ProductStableRequiredGates032 returns the technical product-publication gate
// set for Control Center 0.32. Commercial/legal clearance remains a separate
// commercial-launch decision and is never converted into a synthetic PASS.
func ProductStableRequiredGates032() []GateID {
	return append([]GateID(nil), release032ProductStableRequiredGates[:]...)
}

// RequiredArtifactNames032 returns the immutable expected artifact names for an
// exact Control Center 0.32 candidate bundle.
func RequiredArtifactNames032() []string {
	return append([]string(nil), requiredArtifactNames032[:]...)
}

func validateSnapshot032(snapshot Snapshot) (map[GateID]GateEvidence, error) {
	if snapshot.Schema != Schema032V1 {
		return nil, fmt.Errorf("unsupported 0.32 readiness schema %q", snapshot.Schema)
	}
	if snapshot.StableVersion != Stable032BaseVersion || snapshot.StableTag != Stable032BaseTag {
		return nil, fmt.Errorf("unexpected 0.32 stable base identity %q (%q)", snapshot.StableVersion, snapshot.StableTag)
	}
	if snapshot.StableArtifactDigest != Stable032ArtifactDigest {
		return nil, fmt.Errorf("unexpected 0.32 stable base artifact digest")
	}
	if snapshot.CandidateVersion != Candidate032Version {
		return nil, fmt.Errorf("unexpected 0.32 candidate version %q", snapshot.CandidateVersion)
	}
	if !shaRE.MatchString(snapshot.CandidateSHA) {
		return nil, fmt.Errorf("invalid 0.32 candidate sha")
	}

	known := make(map[GateID]struct{}, len(release032RequiredGates))
	for _, gate := range release032RequiredGates {
		known[gate] = struct{}{}
	}

	seen := make(map[GateID]GateEvidence, len(snapshot.Gates))
	for _, evidence := range snapshot.Gates {
		if _, ok := known[evidence.Gate]; !ok {
			return nil, fmt.Errorf("unknown 0.32 release gate %q", evidence.Gate)
		}
		if _, duplicate := seen[evidence.Gate]; duplicate {
			return nil, fmt.Errorf("duplicate 0.32 release gate %q", evidence.Gate)
		}
		if evidence.CandidateSHA != snapshot.CandidateSHA {
			return nil, fmt.Errorf("0.32 release gate %q is bound to a different candidate sha", evidence.Gate)
		}
		switch evidence.Status {
		case GatePass:
			if !digestRE.MatchString(evidence.EvidenceDigest) {
				return nil, fmt.Errorf("0.32 release gate %q pass is missing bounded evidence digest", evidence.Gate)
			}
		case GatePending, GateBlocked:
			if evidence.EvidenceDigest != "" && !digestRE.MatchString(evidence.EvidenceDigest) {
				return nil, fmt.Errorf("0.32 release gate %q has invalid evidence digest", evidence.Gate)
			}
		default:
			return nil, fmt.Errorf("0.32 release gate %q has invalid status %q", evidence.Gate, evidence.Status)
		}
		seen[evidence.Gate] = evidence
	}
	return seen, nil
}

func evaluate032Required(snapshot Snapshot, required []GateID) (Result, error) {
	seen, err := validateSnapshot032(snapshot)
	if err != nil {
		return Result{}, err
	}

	result := Result{}
	for _, gate := range required {
		evidence, ok := seen[gate]
		if !ok || evidence.Status != GatePass {
			result.Blockers = append(result.Blockers, gate)
		}
	}
	result.Ready = len(result.Blockers) == 0
	return result, nil
}

// Evaluate032 evaluates complete 0.32 release readiness including the separate
// commercial/legal launch gate. It never builds, publishes or deploys anything.
func Evaluate032(snapshot Snapshot) (Result, error) {
	return evaluate032Required(snapshot, release032RequiredGates[:])
}

// EvaluateProductStable032 evaluates only the technical 0.32 product Public
// Stable policy. It preserves fail-closed validation of any supplied evidence
// while keeping commercial/legal launch clearance as a distinct later track.
func EvaluateProductStable032(snapshot Snapshot) (Result, error) {
	return evaluate032Required(snapshot, release032ProductStableRequiredGates[:])
}

// ValidateArtifactManifest032 validates the exact identity and immutable file
// set expected from a future 0.32 candidate packaging step. It performs no I/O
// beyond validating the supplied manifest value and grants no publication
// authority.
func ValidateArtifactManifest032(manifest ArtifactManifest) error {
	if manifest.Schema != ArtifactManifestSchema032V1 {
		return fmt.Errorf("unsupported 0.32 artifact manifest schema %q", manifest.Schema)
	}
	if manifest.CandidateVersion != Candidate032Version {
		return fmt.Errorf("unexpected 0.32 candidate artifact version %q", manifest.CandidateVersion)
	}
	if !shaRE.MatchString(manifest.CandidateSHA) {
		return fmt.Errorf("invalid 0.32 candidate artifact sha")
	}

	required := make(map[string]struct{}, len(requiredArtifactNames032))
	for _, name := range requiredArtifactNames032 {
		required[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if _, ok := required[artifact.Name]; !ok {
			return fmt.Errorf("unexpected 0.32 candidate artifact %q", artifact.Name)
		}
		if _, duplicate := seen[artifact.Name]; duplicate {
			return fmt.Errorf("duplicate 0.32 candidate artifact %q", artifact.Name)
		}
		if !digestRE.MatchString(artifact.Digest) {
			return fmt.Errorf("0.32 candidate artifact %q has invalid digest", artifact.Name)
		}
		seen[artifact.Name] = struct{}{}
	}
	for _, name := range requiredArtifactNames032 {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("required 0.32 candidate artifact %q is missing", name)
		}
	}
	return nil
}
