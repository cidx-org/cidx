package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/environment"
	"github.com/cidx-org/cidx/pkg/executor"
	"github.com/cidx-org/cidx/pkg/presets"
	"github.com/sirupsen/logrus"
)

// RunnerOptions configures runner behavior
type RunnerOptions struct {
	Backend     executor.BackendType
	Parallel    bool
	Concurrency int
}

// Runner orchestrates pipeline execution
type Runner struct {
	config      *config.Config
	selector    *executor.Selector
	backend     executor.BackendType
	logger      *logrus.Logger
	env         *environment.Environment
	parallel    bool
	concurrency int
}

// NewRunner creates a new pipeline runner
// Deprecated: Use NewRunnerWithSelector instead
func NewRunner(cfg *config.Config, exec *executor.DockerExecutor) *Runner {
	logger := logrus.New()
	env := environment.Detect()

	// Log environment detection
	if env.IsCI {
		logger.Infof("🔍 Environment: %s (CI mode)", env.Provider)
	} else {
		logger.Infof("🔍 Environment: Local (safe mode)")
	}

	// Create selector with docker executor for backwards compatibility
	selector, _ := executor.NewSelector(false, false, false)

	return &Runner{
		config:   cfg,
		selector: selector,
		backend:  executor.BackendDocker,
		logger:   logger,
		env:      env,
	}
}

// NewRunnerWithSelector creates a new pipeline runner with executor selector
// Deprecated: Use NewRunnerWithOptions instead
func NewRunnerWithSelector(cfg *config.Config, selector *executor.Selector, backend executor.BackendType) *Runner {
	return NewRunnerWithOptions(cfg, selector, RunnerOptions{
		Backend:     backend,
		Parallel:    false,
		Concurrency: 2,
	})
}

// NewRunnerWithOptions creates a new pipeline runner with full options
func NewRunnerWithOptions(cfg *config.Config, selector *executor.Selector, opts RunnerOptions) *Runner {
	logger := logrus.New()
	env := environment.Detect()

	// Log environment detection
	if env.IsCI {
		logger.Infof("🔍 Environment: %s (CI mode)", env.Provider)
	} else {
		logger.Infof("🔍 Environment: Local (safe mode)")
	}

	// Log backend selection
	if opts.Backend == executor.BackendAuto {
		if selector.DockerAvailable() {
			logger.Infof("🐳 Backend: Docker (auto-detected)")
		} else if selector.PodmanAvailable() {
			logger.Infof("🦭 Backend: Podman (auto-detected)")
		} else {
			logger.Warnf("⚠️  No container runtime available")
		}
	} else {
		logger.Infof("🔧 Backend: %s (forced)", opts.Backend)
	}

	// Warn if parallel mode in CI
	parallel := opts.Parallel
	if parallel && env.IsCI {
		logger.Warnf("⚠️  Parallel mode disabled in CI environment")
		parallel = false
	}

	// Log parallel mode
	if parallel {
		logger.Infof("⚡ Parallel: enabled (concurrency: %d)", opts.Concurrency)
	}

	return &Runner{
		config:      cfg,
		selector:    selector,
		backend:     opts.Backend,
		logger:      logger,
		env:         env,
		parallel:    parallel,
		concurrency: opts.Concurrency,
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

// RunPhase executes all containers in a phase
func (r *Runner) RunPhase(ctx context.Context, phaseName string, phase config.Phase) error {
	r.logger.Infof("========================================")
	r.logger.Infof("▶ PHASE: %s", strings.ToUpper(phaseName))
	r.logger.Infof("========================================")

	if len(phase.Containers) == 0 {
		r.logger.Warnf("No containers in phase: %s", phaseName)
		return nil
	}

	// Use parallel execution if enabled and multiple containers
	if r.parallel && len(phase.Containers) > 1 {
		return r.runPhaseParallel(ctx, phaseName, phase)
	}

	// Sequential execution (default)
	for _, containerName := range phase.Containers {
		if err := r.RunTool(ctx, containerName); err != nil {
			return fmt.Errorf("container %s failed: %w", containerName, err)
		}
	}

	r.logger.Infof("✓ Phase %s completed successfully", phaseName)
	r.logger.Infof("")
	return nil
}

// runPhaseParallel executes containers in parallel with concurrency limit
func (r *Runner) runPhaseParallel(ctx context.Context, phaseName string, phase config.Phase) error {
	r.logger.Infof("⚡ Running %d containers in parallel (max %d concurrent)", len(phase.Containers), r.concurrency)

	// Create semaphore for concurrency limit
	sem := make(chan struct{}, r.concurrency)

	// Error channel to collect errors
	errChan := make(chan error, len(phase.Containers))

	// WaitGroup to track completion
	var wg sync.WaitGroup

	// Result tracking
	var mu sync.Mutex
	completed := 0
	failed := 0

	for _, containerName := range phase.Containers {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check if context cancelled
			if ctx.Err() != nil {
				errChan <- ctx.Err()
				return
			}

			// Run the container
			err := r.RunTool(ctx, name)

			mu.Lock()
			if err != nil {
				failed++
				r.logger.Errorf("  ✗ %s failed: %v", name, err)
				errChan <- fmt.Errorf("container %s failed: %w", name, err)
			} else {
				completed++
			}
			mu.Unlock()
		}(containerName)
	}

	// Wait for all to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// Report results
	if len(errors) > 0 {
		r.logger.Errorf("✗ Phase %s: %d/%d failed", phaseName, failed, len(phase.Containers))
		return fmt.Errorf("phase %s had %d failures", phaseName, len(errors))
	}

	r.logger.Infof("✓ Phase %s completed successfully (%d containers)", phaseName, completed)
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
	var overrides map[string]any
	if r.config.Overrides != nil {
		overrides = r.config.Overrides[toolName]
	}

	mergedPreset := preset.MergeWith(overrides)

	// Expand ${WORKSPACE} in volumes
	volumes := r.expandWorkspace(mergedPreset.Volumes)

	// Convert to ContainerConfig
	containerConfig := &config.ContainerConfig{
		Name:        mergedPreset.Name,
		Phase:       mergedPreset.Phase,
		Image:       mergedPreset.Image,
		Command:     mergedPreset.Command,
		Entrypoint:  mergedPreset.Entrypoint,
		Workdir:     mergedPreset.Workdir,
		Volumes:     volumes,
		Env:         mergedPreset.Env,
		ConfigFiles: mergedPreset.ConfigFiles,
		Privileged:  mergedPreset.Privileged,
	}

	// If execution mode forces dry-run (local safety), show what would be done
	if execMode.IsDryRun {
		r.printLocalSafetyDryRun(containerConfig)
		return nil
	}

	// Select executor based on backend preference
	exec, err := r.selector.Select(toolName, r.backend)
	if err != nil {
		return fmt.Errorf("executor selection failed: %w", err)
	}

	// Execute
	return exec.Run(ctx, containerConfig)
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

// printLocalSafetyDryRun shows what would be executed in local safety mode
func (r *Runner) printLocalSafetyDryRun(containerConfig *config.ContainerConfig) {
	r.logger.Infof("  ⚠️  Would execute (local safety - not running):")
	r.logger.Infof("     Image: %s", containerConfig.Image)
	r.logger.Infof("     Command: %s", containerConfig.Command)
	r.logger.Infof("     Workdir: %s", containerConfig.Workdir)

	if len(containerConfig.Volumes) > 0 {
		r.logger.Infof("     Volumes:")
		for _, vol := range containerConfig.Volumes {
			r.logger.Infof("       - %s", vol)
		}
	}

	if len(containerConfig.Env) > 0 {
		r.logger.Infof("     Environment:")
		for k, v := range containerConfig.Env {
			r.logger.Infof("       %s=%s", k, v)
		}
	}

	r.logger.Infof("  ✓ %s (dry-run - local safety)", containerConfig.Name)
}
