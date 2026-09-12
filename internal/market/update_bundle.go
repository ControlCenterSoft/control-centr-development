package market

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxUpdateBundleBytes      uint64 = 16 << 30
	maxMigrationArtifactBytes uint64 = 64 << 20
	maxMigrationArtifacts            = 128

	RollbackPreviousVersion = "RESTORE_PREVIOUS_VERSION"
	RollbackDataSnapshot    = "RESTORE_PREVIOUS_VERSION_AND_DATA_SNAPSHOT"
)

type ModuleMigrationArtifact struct {
	ID                string `json:"id"`
	FromSchemaVersion string `json:"from_schema_version"`
	ToSchemaVersion   string `json:"to_schema_version"`
	SHA256            string `json:"sha256"`
	SizeBytes         uint64 `json:"size_bytes"`
	Destructive       bool   `json:"destructive"`
	DownMigration     bool   `json:"down_migration"`
}

type ModuleUpdateBundleRequest struct {
	ModuleID             string                    `json:"module_id"`
	CurrentVersion       string                    `json:"current_version"`
	TargetVersion        string                    `json:"target_version"`
	CurrentSchemaVersion string                    `json:"current_schema_version"`
	TargetSchemaVersion  string                    `json:"target_schema_version"`
	BundleSHA256         string                    `json:"bundle_sha256"`
	BundleSizeBytes      uint64                    `json:"bundle_size_bytes"`
	Generation           uint64                    `json:"generation"`
	Migrations           []ModuleMigrationArtifact `json:"migrations"`
}

type ModuleUpdateBundlePlan struct {
	BundleID                    string                    `json:"bundle_id"`
	ModuleID                    string                    `json:"module_id"`
	CurrentVersion              string                    `json:"current_version"`
	TargetVersion               string                    `json:"target_version"`
	CurrentSchemaVersion        string                    `json:"current_schema_version"`
	TargetSchemaVersion         string                    `json:"target_schema_version"`
	BundleSHA256                string                    `json:"bundle_sha256"`
	BundleSizeBytes             uint64                    `json:"bundle_size_bytes"`
	Generation                  uint64                    `json:"generation"`
	Migrations                  []ModuleMigrationArtifact `json:"migrations"`
	IdempotencyKey              string                    `json:"idempotency_key"`
	RollbackMode                string                    `json:"rollback_mode"`
	RequirePreMigrationSnapshot bool                      `json:"require_pre_migration_snapshot"`
	PreserveUserData            bool                      `json:"preserve_user_data"`
	PreserveSecretMaterial      bool                      `json:"preserve_secret_material"`
	ExecutionAuthorized         bool                      `json:"execution_authorized"`
}

var (
	updateBundleModuleIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$`)
	updateBundleVersionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	updateBundleDigestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	migrationIDPattern          = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
)

func PlanModuleUpdateBundle(request ModuleUpdateBundleRequest) (ModuleUpdateBundlePlan, error) {
	normalized, err := normalizeModuleUpdateBundleRequest(request)
	if err != nil {
		return ModuleUpdateBundlePlan{}, err
	}

	plan := ModuleUpdateBundlePlan{
		ModuleID:               normalized.ModuleID,
		CurrentVersion:         normalized.CurrentVersion,
		TargetVersion:          normalized.TargetVersion,
		CurrentSchemaVersion:   normalized.CurrentSchemaVersion,
		TargetSchemaVersion:    normalized.TargetSchemaVersion,
		BundleSHA256:           normalized.BundleSHA256,
		BundleSizeBytes:        normalized.BundleSizeBytes,
		Generation:             normalized.Generation,
		Migrations:             append([]ModuleMigrationArtifact(nil), normalized.Migrations...),
		PreserveUserData:       true,
		PreserveSecretMaterial: true,
		ExecutionAuthorized:    false,
	}

	if len(plan.Migrations) > 0 {
		plan.RequirePreMigrationSnapshot = true
		plan.RollbackMode = RollbackDataSnapshot
	} else {
		plan.RollbackMode = RollbackPreviousVersion
	}
	plan.BundleID = updateBundleID(plan)
	plan.IdempotencyKey = updateBundleIdempotencyKey(plan)
	return plan, nil
}

func ValidateModuleUpdateBundlePlan(plan ModuleUpdateBundlePlan) error {
	if !plan.PreserveUserData {
		return fmt.Errorf("update bundle plan must preserve user data")
	}
	if !plan.PreserveSecretMaterial {
		return fmt.Errorf("update bundle plan must preserve secret material")
	}
	if plan.ExecutionAuthorized {
		return fmt.Errorf("update bundle plan must not authorize execution")
	}

	rebuilt, err := PlanModuleUpdateBundle(ModuleUpdateBundleRequest{
		ModuleID:             plan.ModuleID,
		CurrentVersion:       plan.CurrentVersion,
		TargetVersion:        plan.TargetVersion,
		CurrentSchemaVersion: plan.CurrentSchemaVersion,
		TargetSchemaVersion:  plan.TargetSchemaVersion,
		BundleSHA256:         plan.BundleSHA256,
		BundleSizeBytes:      plan.BundleSizeBytes,
		Generation:           plan.Generation,
		Migrations:           append([]ModuleMigrationArtifact(nil), plan.Migrations...),
	})
	if err != nil {
		return fmt.Errorf("invalid update bundle plan: %w", err)
	}
	if plan.BundleID != rebuilt.BundleID {
		return fmt.Errorf("update bundle plan identity mismatch")
	}
	if plan.IdempotencyKey != rebuilt.IdempotencyKey {
		return fmt.Errorf("update bundle idempotency key mismatch")
	}
	if plan.RollbackMode != rebuilt.RollbackMode {
		return fmt.Errorf("update bundle rollback mode mismatch")
	}
	if plan.RequirePreMigrationSnapshot != rebuilt.RequirePreMigrationSnapshot {
		return fmt.Errorf("update bundle snapshot requirement mismatch")
	}
	return nil
}

func VerifyUpdateBundlePayload(plan ModuleUpdateBundlePlan, payload []byte) error {
	if err := ValidateModuleUpdateBundlePlan(plan); err != nil {
		return err
	}
	if uint64(len(payload)) != plan.BundleSizeBytes {
		return fmt.Errorf("update bundle size mismatch: got %d want %d", len(payload), plan.BundleSizeBytes)
	}
	digest := sha256.Sum256(payload)
	actual := hex.EncodeToString(digest[:])
	if actual != plan.BundleSHA256 {
		return fmt.Errorf("update bundle checksum mismatch")
	}
	return nil
}

func VerifyMigrationArtifactPayload(plan ModuleUpdateBundlePlan, migrationID string, payload []byte) error {
	if err := ValidateModuleUpdateBundlePlan(plan); err != nil {
		return err
	}
	migrationID = strings.TrimSpace(migrationID)
	for _, migration := range plan.Migrations {
		if migration.ID != migrationID {
			continue
		}
		if uint64(len(payload)) != migration.SizeBytes {
			return fmt.Errorf("migration %s size mismatch: got %d want %d", migration.ID, len(payload), migration.SizeBytes)
		}
		digest := sha256.Sum256(payload)
		if hex.EncodeToString(digest[:]) != migration.SHA256 {
			return fmt.Errorf("migration %s checksum mismatch", migration.ID)
		}
		return nil
	}
	return fmt.Errorf("migration %q is not part of update bundle", migrationID)
}

func normalizeModuleUpdateBundleRequest(request ModuleUpdateBundleRequest) (ModuleUpdateBundleRequest, error) {
	request.ModuleID = strings.TrimSpace(request.ModuleID)
	request.CurrentVersion = strings.TrimSpace(request.CurrentVersion)
	request.TargetVersion = strings.TrimSpace(request.TargetVersion)
	request.CurrentSchemaVersion = strings.TrimSpace(request.CurrentSchemaVersion)
	request.TargetSchemaVersion = strings.TrimSpace(request.TargetSchemaVersion)
	request.BundleSHA256 = strings.TrimSpace(request.BundleSHA256)

	if !updateBundleModuleIDPattern.MatchString(request.ModuleID) {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("invalid module id %q", request.ModuleID)
	}
	if request.Generation == 0 {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("generation must be greater than zero")
	}
	if request.BundleSizeBytes == 0 || request.BundleSizeBytes > maxUpdateBundleBytes {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("bundle size must be between 1 and %d bytes", maxUpdateBundleBytes)
	}
	if !updateBundleDigestPattern.MatchString(request.BundleSHA256) {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("bundle sha256 must be lowercase hexadecimal")
	}
	if _, err := parseUpdateBundleVersion(request.CurrentVersion); err != nil {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("invalid current module version: %w", err)
	}
	if _, err := parseUpdateBundleVersion(request.TargetVersion); err != nil {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("invalid target module version: %w", err)
	}
	moduleComparison, err := compareUpdateBundleVersions(request.CurrentVersion, request.TargetVersion)
	if err != nil {
		return ModuleUpdateBundleRequest{}, err
	}
	if moduleComparison >= 0 {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("update target version must be newer than current version")
	}
	if _, err := parseUpdateBundleVersion(request.CurrentSchemaVersion); err != nil {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("invalid current schema version: %w", err)
	}
	if _, err := parseUpdateBundleVersion(request.TargetSchemaVersion); err != nil {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("invalid target schema version: %w", err)
	}
	schemaComparison, err := compareUpdateBundleVersions(request.CurrentSchemaVersion, request.TargetSchemaVersion)
	if err != nil {
		return ModuleUpdateBundleRequest{}, err
	}
	if schemaComparison > 0 {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("schema downgrade is not allowed")
	}
	if len(request.Migrations) > maxMigrationArtifacts {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("too many migration artifacts: %d", len(request.Migrations))
	}
	if schemaComparison == 0 && len(request.Migrations) != 0 {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("unchanged schema version must not include migrations")
	}
	if schemaComparison < 0 && len(request.Migrations) == 0 {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("schema upgrade requires an explicit migration chain")
	}

	expectedSchema := request.CurrentSchemaVersion
	seenIDs := make(map[string]struct{}, len(request.Migrations))
	for index := range request.Migrations {
		migration := &request.Migrations[index]
		migration.ID = strings.TrimSpace(migration.ID)
		migration.FromSchemaVersion = strings.TrimSpace(migration.FromSchemaVersion)
		migration.ToSchemaVersion = strings.TrimSpace(migration.ToSchemaVersion)
		migration.SHA256 = strings.TrimSpace(migration.SHA256)

		if !migrationIDPattern.MatchString(migration.ID) {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("invalid migration id %q", migration.ID)
		}
		if _, exists := seenIDs[migration.ID]; exists {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("duplicate migration id %q", migration.ID)
		}
		seenIDs[migration.ID] = struct{}{}
		if migration.Destructive {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("destructive migration %q is not allowed", migration.ID)
		}
		if migration.DownMigration {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("down migration %q is not allowed", migration.ID)
		}
		if migration.SizeBytes == 0 || migration.SizeBytes > maxMigrationArtifactBytes {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("migration %q size must be between 1 and %d bytes", migration.ID, maxMigrationArtifactBytes)
		}
		if !updateBundleDigestPattern.MatchString(migration.SHA256) {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("migration %q sha256 must be lowercase hexadecimal", migration.ID)
		}
		if migration.FromSchemaVersion != expectedSchema {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("migration %q starts at schema %s, expected %s", migration.ID, migration.FromSchemaVersion, expectedSchema)
		}
		comparison, err := compareUpdateBundleVersions(migration.FromSchemaVersion, migration.ToSchemaVersion)
		if err != nil {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("migration %q: %w", migration.ID, err)
		}
		if comparison >= 0 {
			return ModuleUpdateBundleRequest{}, fmt.Errorf("migration %q must move schema forward", migration.ID)
		}
		expectedSchema = migration.ToSchemaVersion
	}
	if expectedSchema != request.TargetSchemaVersion {
		return ModuleUpdateBundleRequest{}, fmt.Errorf("migration chain ends at schema %s, expected %s", expectedSchema, request.TargetSchemaVersion)
	}
	return request, nil
}

func updateBundleID(plan ModuleUpdateBundlePlan) string {
	parts := []string{
		plan.ModuleID,
		plan.CurrentVersion,
		plan.TargetVersion,
		plan.CurrentSchemaVersion,
		plan.TargetSchemaVersion,
		plan.BundleSHA256,
		strconv.FormatUint(plan.BundleSizeBytes, 10),
	}
	for _, migration := range plan.Migrations {
		parts = append(parts,
			migration.ID,
			migration.FromSchemaVersion,
			migration.ToSchemaVersion,
			migration.SHA256,
			strconv.FormatUint(migration.SizeBytes, 10),
		)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "market-update-bundle:" + hex.EncodeToString(digest[:])
}

func updateBundleIdempotencyKey(plan ModuleUpdateBundlePlan) string {
	material := strings.Join([]string{
		plan.BundleID,
		strconv.FormatUint(plan.Generation, 10),
		plan.CurrentVersion,
		plan.CurrentSchemaVersion,
	}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return "market-update:" + hex.EncodeToString(digest[:])
}

func compareUpdateBundleVersions(current, target string) (int, error) {
	currentParts, err := parseUpdateBundleVersion(current)
	if err != nil {
		return 0, err
	}
	targetParts, err := parseUpdateBundleVersion(target)
	if err != nil {
		return 0, err
	}
	for index := 0; index < len(currentParts); index++ {
		if currentParts[index] < targetParts[index] {
			return -1, nil
		}
		if currentParts[index] > targetParts[index] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseUpdateBundleVersion(version string) ([3]uint64, error) {
	var parsed [3]uint64
	if !updateBundleVersionPattern.MatchString(version) {
		return parsed, fmt.Errorf("invalid semantic version %q", version)
	}
	parts := strings.Split(version, ".")
	for index, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return parsed, fmt.Errorf("invalid semantic version %q: %w", version, err)
		}
		parsed[index] = value
	}
	return parsed, nil
}
