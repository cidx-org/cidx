package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
// phaseResult tracks the outcome of a single phase execution.
type phaseResult struct {
	Name       string
	Duration   time.Duration
	Containers []string
	Passed     bool
	Error      string
}

func (r *Runner) RunPipeline(ctx context.Context, name string, pipeline config.Pipeline) error {
	r.logger.Infof("Running pipeline: %s", name)
	pipelineStart := time.Now()

	var results []phaseResult

	for _, phaseName := range pipeline.Phases {
		phase, exists := r.config.Phases[phaseName]
		if !exists {
			return fmt.Errorf("pipeline %s references unknown phase: %s", name, phaseName)
		}

		phaseStart := time.Now()
		err := r.RunPhase(ctx, phaseName, phase)
		elapsed := time.Since(phaseStart)

		result := phaseResult{
			Name:       phaseName,
			Duration:   elapsed,
			Containers: phase.Containers,
			Passed:     err == nil,
		}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)

		if err != nil {
			r.printPipelineSummary(name, results, time.Since(pipelineStart))
			return fmt.Errorf("pipeline %s failed at phase %s: %w", name, phaseName, err)
		}
	}

	r.printPipelineSummary(name, results, time.Since(pipelineStart))
	return nil
}

// printPipelineSummary displays a summary table after pipeline execution.
func (r *Runner) printPipelineSummary(name string, results []phaseResult, total time.Duration) {
	r.logger.Infof("========================================")
	r.logger.Infof("Pipeline: %s — completed in %s", name, formatDuration(total))
	r.logger.Infof("========================================")

	for _, res := range results {
		icon := "✓"
		if !res.Passed {
			icon = "✗"
		}

		containers := strings.Join(res.Containers, " ✓, ")
		if res.Passed && len(res.Containers) > 0 {
			containers += " ✓"
		}

		r.logger.Infof("  %s %s (%s) — %s", icon, res.Name, formatDuration(res.Duration), containers)
	}

	r.logger.Infof("")
}

// formatDuration formats a duration as human-readable (e.g., "1m32s", "42s").
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
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
	// Resolve the override section once — we need it for both code paths
	// below (custom declaration vs. preset override).
	var overrides map[string]any
	if r.config.Overrides != nil {
		overrides = r.config.Overrides[toolName]
	}

	// Get preset. If no built-in preset matches, fall back to a custom
	// container declaration from [containers.NAME] (must have an `image`
	// field). See #142.
	preset, err := presets.Get(toolName)
	if err != nil {
		if !presets.IsCustomDeclaration(overrides) {
			return fmt.Errorf("container %q is not a built-in preset and has no [containers.%s] declaration with an `image` field", toolName, toolName)
		}
		preset = presets.PresetFromOverrides(toolName, overrides)
		// Custom containers skip the preset-merge step below — the declaration
		// IS the full definition. Clear `overrides` so MergeWith is a no-op.
		overrides = nil
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

	mergedPreset := preset.MergeWith(overrides)

	// Expand ${WORKSPACE} in volumes
	volumes := r.expandWorkspace(mergedPreset.Volumes)

	// Guardrail (#151): if the resolved workdir is not the mount point of any
	// volume, the container will start in a directory that does not exist or
	// has no project files. The tool then runs against an empty path and emits
	// confusing "no files found" errors. Fail fast with an actionable message.
	if err := checkWorkdirCoveredByVolumes(mergedPreset.Workdir, volumes); err != nil {
		return fmt.Errorf("preset %q: %w", toolName, err)
	}

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
		PullPolicy:  r.resolvePullPolicy(mergedPreset.PullPolicy),
		Timeout:     mergedPreset.Timeout,
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

// resolvePullPolicy determines the effective pull policy.
// If explicitly set on the preset, use it. Otherwise, default based on environment.
func (r *Runner) resolvePullPolicy(presetPolicy string) string {
	if presetPolicy != "" {
		return presetPolicy
	}
	if r.env.IsCI {
		return "always"
	}
	return "if-not-present"
}

// checkWorkdirCoveredByVolumes verifies the resolved workdir lies inside one
// of the volume mount targets. A mismatch is the most common silent-failure
// mode reported by consumers (#151): override workdir to "/src/sub" while the
// preset still mounts at "/work", and the tool runs against a non-existent
// directory. We refuse to execute and tell the user exactly what to do.
//
// Volumes are in "host:target[:opts]" form after ${WORKSPACE} expansion.
// Empty workdir or empty volumes list short-circuits to nil — those are
// fine (no bind = no expectation).
func checkWorkdirCoveredByVolumes(workdir string, volumes []string) error {
	if workdir == "" || len(volumes) == 0 {
		return nil
	}
	for _, vol := range volumes {
		parts := strings.Split(vol, ":")
		if len(parts) < 2 {
			continue
		}
		target := parts[1]
		if target == "" {
			continue
		}
		if workdir == target || strings.HasPrefix(workdir, target+"/") {
			return nil
		}
	}
	// Build a helpful error: list the actual mount targets the user can
	// pick from, and remind them they can override `volumes` to match
	// their custom workdir.
	mounts := make([]string, 0, len(volumes))
	for _, vol := range volumes {
		parts := strings.Split(vol, ":")
		if len(parts) >= 2 && parts[1] != "" {
			mounts = append(mounts, parts[1])
		}
	}
	return fmt.Errorf(
		"workdir %q is not inside any mounted volume (mounts: %s); "+
			"either set workdir to a path under one of these mounts, "+
			"or override `volumes` so the workdir is covered",
		workdir,
		strings.Join(mounts, ", "),
	)
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
