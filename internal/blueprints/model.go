// Package blueprints defines provider-neutral, versioned architecture templates.
// A blueprint describes required roles/capabilities/topology; it is not an
// installer and it grants no execution authority.
package blueprints

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"control-center/internal/corecontracts"
)

var (
	ErrInvalidBlueprint         = errors.New("invalid solution blueprint")
	ErrInvalidBlueprintRevision = errors.New("invalid blueprint revision")
	blueprintIDPattern          = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?$`)
	blueprintDigestPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type QualificationStatus string

const (
	QualificationDraft      QualificationStatus = "draft"
	QualificationQualified  QualificationStatus = "qualified"
	QualificationDeprecated QualificationStatus = "deprecated"
)

func (s QualificationStatus) Valid() bool {
	switch s {
	case QualificationDraft, QualificationQualified, QualificationDeprecated:
		return true
	default:
		return false
	}
}

type BlueprintRole struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	MinCount             uint32   `json:"min_count"`
	MaxCount             uint32   `json:"max_count"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
}

func (r BlueprintRole) Validate() error {
	if err := validateBlueprintID("role id", r.ID); err != nil {
		return err
	}
	if err := validateBlueprintID("role kind", r.Kind); err != nil {
		return err
	}
	if r.MinCount == 0 {
		return errors.New("role min_count must be positive")
	}
	if r.MaxCount != 0 && r.MaxCount < r.MinCount {
		return errors.New("role max_count must be zero/unbounded or >= min_count")
	}
	seen := map[string]struct{}{}
	for _, capability := range r.RequiredCapabilities {
		if err := validateBlueprintID("required capability", capability); err != nil {
			return err
		}
		if _, duplicate := seen[capability]; duplicate {
			return fmt.Errorf("duplicate required capability %q", capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

type TopologyRule struct {
	FromRole string `json:"from_role"`
	ToRole   string `json:"to_role"`
	Relation string `json:"relation"`
	Required bool   `json:"required"`
}

type SolutionBlueprint struct {
	corecontracts.ObjectMetadata
	DisplayName       string `json:"display_name"`
	CurrentRevisionID string `json:"current_revision_id,omitempty"`
}

func (b SolutionBlueprint) Validate() error {
	if err := b.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBlueprint, err)
	}
	if strings.TrimSpace(b.DisplayName) == "" || len(b.DisplayName) > 160 {
		return fmt.Errorf("%w: display_name must be 1..160 characters", ErrInvalidBlueprint)
	}
	if b.CurrentRevisionID != "" && strings.TrimSpace(b.CurrentRevisionID) != b.CurrentRevisionID {
		return fmt.Errorf("%w: current_revision_id has surrounding whitespace", ErrInvalidBlueprint)
	}
	return nil
}

type BlueprintRevision struct {
	corecontracts.ObjectMetadata
	BlueprintID         string              `json:"blueprint_id"`
	Ordinal             uint64              `json:"ordinal"`
	QualificationStatus QualificationStatus `json:"qualification_status"`
	BlueprintDigest     string              `json:"blueprint_digest"`
	SignatureRef        string              `json:"signature_ref,omitempty"`
	IntentClasses       []string            `json:"intent_classes,omitempty"`
	Roles               []BlueprintRole     `json:"roles"`
	TopologyRules       []TopologyRule      `json:"topology_rules,omitempty"`
	ExpansionStrategies []string            `json:"expansion_strategies,omitempty"`
}

func (r BlueprintRevision) Validate() error {
	if err := r.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBlueprintRevision, err)
	}
	if strings.TrimSpace(r.BlueprintID) == "" || strings.TrimSpace(r.BlueprintID) != r.BlueprintID {
		return fmt.Errorf("%w: blueprint_id is required", ErrInvalidBlueprintRevision)
	}
	if r.Ordinal == 0 {
		return fmt.Errorf("%w: ordinal must be positive", ErrInvalidBlueprintRevision)
	}
	if !r.QualificationStatus.Valid() {
		return fmt.Errorf("%w: qualification status %q is unsupported", ErrInvalidBlueprintRevision, r.QualificationStatus)
	}
	if !blueprintDigestPattern.MatchString(r.BlueprintDigest) {
		return fmt.Errorf("%w: blueprint_digest must be sha256:<64 lowercase hex>", ErrInvalidBlueprintRevision)
	}
	if r.QualificationStatus == QualificationQualified && strings.TrimSpace(r.SignatureRef) == "" {
		return fmt.Errorf("%w: qualified revision requires signature_ref", ErrInvalidBlueprintRevision)
	}
	if len(r.Roles) == 0 {
		return fmt.Errorf("%w: at least one role is required", ErrInvalidBlueprintRevision)
	}
	roles := make(map[string]struct{}, len(r.Roles))
	for _, role := range r.Roles {
		if err := role.Validate(); err != nil {
			return fmt.Errorf("%w: role %q: %v", ErrInvalidBlueprintRevision, role.ID, err)
		}
		if _, duplicate := roles[role.ID]; duplicate {
			return fmt.Errorf("%w: duplicate role %q", ErrInvalidBlueprintRevision, role.ID)
		}
		roles[role.ID] = struct{}{}
	}
	seenRules := map[string]struct{}{}
	for _, rule := range r.TopologyRules {
		if _, ok := roles[rule.FromRole]; !ok {
			return fmt.Errorf("%w: topology source role %q does not exist", ErrInvalidBlueprintRevision, rule.FromRole)
		}
		if _, ok := roles[rule.ToRole]; !ok {
			return fmt.Errorf("%w: topology target role %q does not exist", ErrInvalidBlueprintRevision, rule.ToRole)
		}
		if rule.FromRole == rule.ToRole {
			return fmt.Errorf("%w: topology rule cannot reference the same role twice", ErrInvalidBlueprintRevision)
		}
		if err := validateBlueprintID("topology relation", rule.Relation); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidBlueprintRevision, err)
		}
		key := rule.FromRole + "\x00" + rule.ToRole + "\x00" + rule.Relation
		if _, duplicate := seenRules[key]; duplicate {
			return fmt.Errorf("%w: duplicate topology rule", ErrInvalidBlueprintRevision)
		}
		seenRules[key] = struct{}{}
	}
	if err := validateUniqueIDs("intent class", r.IntentClasses); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBlueprintRevision, err)
	}
	if err := validateUniqueIDs("expansion strategy", r.ExpansionStrategies); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBlueprintRevision, err)
	}
	return nil
}

func validateUniqueIDs(field string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if err := validateBlueprintID(field, value); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate %s %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateBlueprintID(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || !blueprintIDPattern.MatchString(value) {
		return fmt.Errorf("%s must be a canonical lower-case identifier", field)
	}
	return nil
}
