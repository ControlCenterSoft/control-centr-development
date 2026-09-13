// Package providers defines the side-effect-free Managed Provider contract.
// Execution adapters consume these contracts through the Change/Job boundary;
// this package does not perform infrastructure mutations.
package providers

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"control-center/internal/corecontracts"
)

const ManifestSchemaV1 = "provider.manifest/v1"

var (
	ErrInvalidManifest = errors.New("invalid provider manifest")
	ErrInvalidBinding  = errors.New("invalid provider binding")
	idPattern          = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?$`)
)

type ManagementLevel string

const (
	ManagementObserved  ManagementLevel = "observed"
	ManagementConnected ManagementLevel = "connected"
	ManagementManaged   ManagementLevel = "managed"
)

func (m ManagementLevel) Valid() bool {
	switch m {
	case ManagementObserved, ManagementConnected, ManagementManaged:
		return true
	default:
		return false
	}
}

type RiskClass string

const (
	RiskReadOnly RiskClass = "read_only"
	RiskLow      RiskClass = "low"
	RiskMedium   RiskClass = "medium"
	RiskHigh     RiskClass = "high"
	RiskCritical RiskClass = "critical"
)

func (r RiskClass) Valid() bool {
	switch r {
	case RiskReadOnly, RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	default:
		return false
	}
}

type QualificationStatus string

const (
	QualificationUnverified QualificationStatus = "unverified"
	QualificationQualified  QualificationStatus = "qualified"
	QualificationDeprecated QualificationStatus = "deprecated"
)

func (q QualificationStatus) Valid() bool {
	switch q {
	case QualificationUnverified, QualificationQualified, QualificationDeprecated:
		return true
	default:
		return false
	}
}

// CapabilitySpec describes one typed provider operation. RequiredPermissions,
// RequiredSecretRefs and Locks are declarations consumed by higher-level policy
// and execution layers; they never authorize an operation by themselves.
type CapabilitySpec struct {
	ID                  string    `json:"id"`
	Mutation            bool      `json:"mutation"`
	RiskClass           RiskClass `json:"risk_class"`
	RequiredPermissions []string  `json:"required_permissions,omitempty"`
	RequiredSecretRefs  []string  `json:"required_secret_refs,omitempty"`
	Preconditions       []string  `json:"preconditions,omitempty"`
	Locks               []string  `json:"locks,omitempty"`
	Idempotency         string    `json:"idempotency"`
	Verification        []string  `json:"verification,omitempty"`
	FailureModel        string    `json:"failure_model,omitempty"`
	Rollback            string    `json:"rollback,omitempty"`
	Recovery            string    `json:"recovery,omitempty"`
}

func (c CapabilitySpec) Validate() error {
	if err := validateID("capability id", c.ID); err != nil {
		return err
	}
	if !c.RiskClass.Valid() {
		return fmt.Errorf("risk_class %q is unsupported", c.RiskClass)
	}
	if !c.Mutation && c.RiskClass != RiskReadOnly {
		return errors.New("read-only capability must use read_only risk class")
	}
	if c.Mutation && c.RiskClass == RiskReadOnly {
		return errors.New("mutation capability cannot use read_only risk class")
	}
	if strings.TrimSpace(c.Idempotency) == "" {
		return errors.New("idempotency contract is required")
	}
	if err := validateUniqueIDs("required permission", c.RequiredPermissions); err != nil {
		return err
	}
	if err := validateUniqueIDs("secret reference", c.RequiredSecretRefs); err != nil {
		return err
	}
	if err := validateUniqueIDs("lock", c.Locks); err != nil {
		return err
	}
	return nil
}

// Manifest is the signed/versioned contract exposed by one provider adapter.
type Manifest struct {
	SchemaVersion            string              `json:"schema_version"`
	ProviderID               string              `json:"provider_id"`
	ContractVersion          string              `json:"contract_version"`
	ProviderVersion          string              `json:"provider_version"`
	ProductFamily            string              `json:"product_family"`
	SupportedProductVersions []string            `json:"supported_product_versions"`
	ManagementLevels         []ManagementLevel   `json:"management_levels"`
	Capabilities             []CapabilitySpec    `json:"capabilities"`
	QualificationStatus      QualificationStatus `json:"qualification_status"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaV1 {
		return fmt.Errorf("%w: schema_version must be %q", ErrInvalidManifest, ManifestSchemaV1)
	}
	if err := validateID("provider_id", m.ProviderID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if strings.TrimSpace(m.ContractVersion) == "" || strings.TrimSpace(m.ProviderVersion) == "" {
		return fmt.Errorf("%w: contract_version and provider_version are required", ErrInvalidManifest)
	}
	if strings.TrimSpace(m.ProductFamily) == "" {
		return fmt.Errorf("%w: product_family is required", ErrInvalidManifest)
	}
	if len(m.SupportedProductVersions) == 0 {
		return fmt.Errorf("%w: at least one supported product version is required", ErrInvalidManifest)
	}
	if len(m.ManagementLevels) == 0 {
		return fmt.Errorf("%w: at least one management level is required", ErrInvalidManifest)
	}
	seenLevels := map[ManagementLevel]struct{}{}
	for _, level := range m.ManagementLevels {
		if !level.Valid() {
			return fmt.Errorf("%w: management level %q is unsupported", ErrInvalidManifest, level)
		}
		if _, duplicate := seenLevels[level]; duplicate {
			return fmt.Errorf("%w: duplicate management level %q", ErrInvalidManifest, level)
		}
		seenLevels[level] = struct{}{}
	}
	if !m.QualificationStatus.Valid() {
		return fmt.Errorf("%w: qualification status %q is unsupported", ErrInvalidManifest, m.QualificationStatus)
	}
	seenCapabilities := map[string]struct{}{}
	for _, capability := range m.Capabilities {
		if err := capability.Validate(); err != nil {
			return fmt.Errorf("%w: capability %q: %v", ErrInvalidManifest, capability.ID, err)
		}
		if _, duplicate := seenCapabilities[capability.ID]; duplicate {
			return fmt.Errorf("%w: duplicate capability %q", ErrInvalidManifest, capability.ID)
		}
		seenCapabilities[capability.ID] = struct{}{}
	}
	return nil
}

func (m Manifest) Supports(capabilityID string, level ManagementLevel) bool {
	if !level.Valid() {
		return false
	}
	levelSupported := false
	for _, candidate := range m.ManagementLevels {
		if candidate == level {
			levelSupported = true
			break
		}
	}
	if !levelSupported {
		return false
	}
	for _, capability := range m.Capabilities {
		if capability.ID == capabilityID {
			return true
		}
	}
	return false
}

type BindingHealth string

const (
	BindingHealthy     BindingHealth = "healthy"
	BindingDegraded    BindingHealth = "degraded"
	BindingUnavailable BindingHealth = "unavailable"
	BindingUnknown     BindingHealth = "unknown"
)

func (h BindingHealth) Valid() bool {
	switch h {
	case BindingHealthy, BindingDegraded, BindingUnavailable, BindingUnknown:
		return true
	default:
		return false
	}
}

// Binding is the durable relationship between a provider contract and one
// discovered/adopted/managed product instance.
type Binding struct {
	corecontracts.ObjectMetadata
	ProviderID       string          `json:"provider_id"`
	ProviderVersion  string          `json:"provider_version"`
	ContractVersion  string          `json:"contract_version"`
	ProductFamily    string          `json:"product_family"`
	ProductVersion   string          `json:"product_version"`
	ManagementLevel ManagementLevel `json:"management_level"`
	TargetRef        string          `json:"target_ref"`
	Health           BindingHealth   `json:"health"`
}

func (b Binding) Validate() error {
	if err := b.ObjectMetadata.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	if err := validateID("provider_id", b.ProviderID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	if strings.TrimSpace(b.ProviderVersion) == "" || strings.TrimSpace(b.ContractVersion) == "" {
		return fmt.Errorf("%w: provider_version and contract_version are required", ErrInvalidBinding)
	}
	if strings.TrimSpace(b.ProductFamily) == "" || strings.TrimSpace(b.ProductVersion) == "" {
		return fmt.Errorf("%w: product family/version are required", ErrInvalidBinding)
	}
	if !b.ManagementLevel.Valid() {
		return fmt.Errorf("%w: management level %q is unsupported", ErrInvalidBinding, b.ManagementLevel)
	}
	if strings.TrimSpace(b.TargetRef) == "" {
		return fmt.Errorf("%w: target_ref is required", ErrInvalidBinding)
	}
	if !b.Health.Valid() {
		return fmt.Errorf("%w: health %q is unsupported", ErrInvalidBinding, b.Health)
	}
	return nil
}

func validateID(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || !idPattern.MatchString(value) {
		return fmt.Errorf("%s must be a canonical lower-case identifier", field)
	}
	return nil
}

func validateUniqueIDs(field string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if err := validateID(field, value); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate %s %q", field, value)
		}
		seen[value] = struct{}{}
	}
	sort.Strings(values)
	return nil
}
