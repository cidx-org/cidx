package config

import (
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/cidx-org/cidx/v3/pkg/semver"
)

// ValidationResult contains validation results
type ValidationResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

// Validate validates the configuration
func Validate(cfg *Config) ValidationResult {
	result := ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check if any phases are defined
	if len(cfg.Phases) == 0 {
		result.Errors = append(result.Errors, "no phases defined")
		result.Valid = false
		return result
	}

	// Validate each phase and its containers
	for phaseName, phase := range cfg.Phases {
		if len(phase.Containers) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("phase '%s' has no containers", phaseName))
		}

		// Validate each container exists. A container is valid if it is either:
		//   (a) a built-in preset name (presets.Exists), or
		//   (b) a custom declaration in [containers.NAME] that has an `image` field
		//       — see #142 and examples/cidx-complete.toml.
		for _, containerName := range phase.Containers {
			if presets.Exists(containerName) {
				continue
			}
			if presets.IsCustomDeclaration(cfg.Overrides[containerName]) {
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("unknown container: %s in phase '%s' (no matching preset and no [containers.%s] declaration with an `image` field)", containerName, phaseName, containerName))
			result.Valid = false
		}
	}

	return result
}

// CheckVersion validates the running cidx against the config's required_version.
//
// required_version is a floor, not a pin: any cidx at or above it satisfies the
// config, so a new cidx release does not break every config that named the
// previous one. Both sides are compared numerically, which makes the "v2.1.4"
// spelling used by every tag and doc example equivalent to the "2.1.4" release
// binaries report (issue #212).
func CheckVersion(cfg *Config, currentVersion string) error {
	if cfg.RequiredVersion == "" {
		return nil
	}

	// Always allow dev builds (maybe with a warning in the future)
	if currentVersion == "dev" {
		return nil
	}

	if !semver.IsValid(cfg.RequiredVersion) {
		return fmt.Errorf("invalid required_version %q in config: expected a version like 2.1.4 or v2.1.4", cfg.RequiredVersion)
	}

	if semver.Compare(currentVersion, cfg.RequiredVersion) < 0 {
		return fmt.Errorf("cidx %s is too old: this config requires %s or newer\n   Update with: go install github.com/cidx-org/cidx/v3/cmd/cidx@latest", currentVersion, cfg.RequiredVersion)
	}

	return nil
}
