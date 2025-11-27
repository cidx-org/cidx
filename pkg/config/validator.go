package config

import (
	"fmt"

	"github.com/arcker/cidx/pkg/presets"
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

		// Validate each container exists
		for _, containerName := range phase.Containers {
			if !presets.Exists(containerName) {
				result.Errors = append(result.Errors, fmt.Sprintf("unknown container: %s in phase '%s'", containerName, phaseName))
				result.Valid = false
			}
		}
	}

	return result
}
