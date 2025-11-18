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

	// Validate each phase and its tools
	for phaseName, phase := range cfg.Phases {
		if len(phase.Tools) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("phase '%s' has no tools", phaseName))
		}

		// Validate each tool exists
		for _, toolName := range phase.Tools {
			if !presets.Exists(toolName) {
				result.Errors = append(result.Errors, fmt.Sprintf("unknown tool: %s in phase '%s'", toolName, phaseName))
				result.Valid = false
			}
		}
	}

	return result
}
