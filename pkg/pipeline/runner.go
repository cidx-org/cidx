package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/arcker/cidx/pkg/config"
	"github.com/arcker/cidx/pkg/environment"
	"github.com/arcker/cidx/pkg/executor"
	"github.com/arcker/cidx/pkg/presets"
	"github.com/sirupsen/logrus"
)

// Runner orchestrates pipeline execution
type Runner struct {
	config   *config.Config
	executor *executor.DockerExecutor
	logger   *logrus.Logger
	env      *environment.Environment
}

// NewRunner creates a new pipeline runner
func NewRunner(cfg *config.Config, exec *executor.DockerExecutor) *Runner {
	logger := logrus.New()
	env := environment.Detect()

	// Log environment detection
	if env.IsCI {
		logger.Infof("🔍 Environment: %s (CI mode)", env.Provider)
	} else {
		logger.Infof("🔍 Environment: Local (safe mode)")
	}

	return &Runner{
		config:   cfg,
		executor: exec,
		logger:   logger,
		env:      env,
	}
}

// Run executes a phase, tool, or pipeline
func (r *Runner) Run(ctx context.Context, target string) error {
	// Check if target is a named pipeline
	if pipeline, isPipeline := r.config.Pipelines[target]; isPipeline {
		return r.RunPipeline(ctx, target, pipeline)
	}

	// Check if target is a phase
	if phase, isPhase := r.config.Phases[target]; isPhase {
		return r.RunPhase(ctx, target, phase)
	}

	// Check if target is a tool
	if presets.Exists(target) {
		return r.RunTool(ctx, target)
	}

	// Check if target is "all" → run all phases
	if target == "all" {
		return r.RunAll(ctx)
	}

	return fmt.Errorf("unknown target: %s (not a phase, tool, or pipeline)", target)
}

// RunPipeline executes a named pipeline
func (r *Runner) RunPipeline(ctx context.Context, name string, pipeline config.Pipeline) error {
	r.logger.Infof("Running pipeline: %s", name)

	for _, phaseName := range pipeline.Phases {
		phase, exists := r.config.Phases[phaseName]
		if !exists {
			return fmt.Errorf("pipeline %s references unknown phase: %s", name, phaseName)
		}

		if err := r.RunPhase(ctx, phaseName, phase); err != nil {
			return fmt.Errorf("pipeline %s failed at phase %s: %w", name, phaseName, err)
		}
	}

	r.logger.Infof("✓ Pipeline %s completed successfully", name)
	return nil
}

// RunAll executes all phases
func (r *Runner) RunAll(ctx context.Context) error {
	r.logger.Infof("Running all phases")

	// Run in order: security, code, test, build, then others
	orderedPhases := []string{"security", "code", "test", "build"}

	for _, phaseName := range orderedPhases {
		if phase, exists := r.config.Phases[phaseName]; exists {
			if err := r.RunPhase(ctx, phaseName, phase); err != nil {
				return fmt.Errorf("phase %s failed: %w", phaseName, err)
			}
		}
	}

	// Run remaining phases
	for phaseName, phase := range r.config.Phases {
		isOrdered := false
		for _, ordered := range orderedPhases {
			if phaseName == ordered {
				isOrdered = true
				break
			}
		}
		if !isOrdered {
			if err := r.RunPhase(ctx, phaseName, phase); err != nil {
				return fmt.Errorf("phase %s failed: %w", phaseName, err)
			}
		}
	}

	r.logger.Infof("All phases completed successfully")
	return nil
}

// RunPhase executes all tools in a phase
func (r *Runner) RunPhase(ctx context.Context, phaseName string, phase config.Phase) error {
	r.logger.Infof("========================================")
	r.logger.Infof("▶ PHASE: %s", strings.ToUpper(phaseName))
	r.logger.Infof("========================================")

	if len(phase.Tools) == 0 {
		r.logger.Warnf("No tools in phase: %s", phaseName)
		return nil
	}

	for _, toolName := range phase.Tools {
		if err := r.RunTool(ctx, toolName); err != nil {
			return fmt.Errorf("tool %s failed: %w", toolName, err)
		}
	}

	r.logger.Infof("✓ Phase %s completed successfully", phaseName)
	r.logger.Infof("")
	return nil
}

// RunTool executes a single tool
func (r *Runner) RunTool(ctx context.Context, toolName string) error {
	// Get preset
	preset, err := presets.Get(toolName)
	if err != nil {
		return err
	}

	// Validate preset against environment and get execution mode
	execMode, err := environment.ValidatePreset(preset, r.env)
	if err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}

	// Display execution mode info
	if !r.env.IsCI && execMode.Mode != environment.BehaviorProduction {
		r.logger.Infof("  🛡️  Local safety: %s - %s", execMode.Mode, execMode.Reason)
	}

	// Apply execution mode modifications to preset
	preset = environment.ApplyExecutionMode(preset, execMode)

	// Merge with user overrides
	var overrides map[string]interface{}
	if r.config.Overrides != nil {
		overrides = r.config.Overrides[toolName]
	}

	mergedPreset := preset.MergeWith(overrides)

	// Expand ${WORKSPACE} in volumes
	volumes := r.expandWorkspace(mergedPreset.Volumes)

	// Convert to ToolConfig
	toolConfig := &config.ToolConfig{
		Name:        mergedPreset.Name,
		Phase:       mergedPreset.Phase,
		Image:       mergedPreset.Image,
		Command:     mergedPreset.Command,
		Workdir:     mergedPreset.Workdir,
		Volumes:     volumes,
		Env:         mergedPreset.Env,
		ConfigFiles: mergedPreset.ConfigFiles,
	}

	// Execute
	return r.executor.Run(ctx, toolConfig)
}

// expandWorkspace replaces ${WORKSPACE} with the actual workspace path
func (r *Runner) expandWorkspace(volumes []string) []string {
	workspace := r.config.Workspace
	expanded := make([]string, len(volumes))

	for i, vol := range volumes {
		// Replace ${WORKSPACE} with actual workspace
		expanded[i] = strings.ReplaceAll(vol, "${WORKSPACE}", workspace)
	}

	return expanded
}
