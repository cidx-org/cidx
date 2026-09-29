package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/generate"
	"github.com/cucumber/godog"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// RegisterGenerateSteps registers step definitions for generate scenarios.
//
// The workflow under assertion is the one pkg/generate produced from a real
// config, parsed as YAML rather than pattern-matched as text (issue #265).
func RegisterGenerateSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^cidx\.toml defines pipeline "([^"]*)" with phases "([^"]*)"$`, tc.configDefinesPipeline)
	ctx.Given(`^cidx\.toml defines pipeline "([^"]*)"$`, tc.configDefinesPipelineOnly)
	ctx.Given(`^cidx\.toml has no pipelines defined$`, tc.configHasNoPipelines)

	ctx.Then(`^the output should be valid YAML$`, tc.outputShouldBeValidYAML)
	ctx.Then(`^the output should contain a "([^"]*)" job$`, tc.outputShouldContainJob)
	ctx.Then(`^each phase should have its own job$`, tc.eachPhaseShouldHaveJob)
	ctx.Then(`^jobs (.+) should depend on "([^"]*)"$`, tc.jobsShouldDependOn)
	ctx.Then(`^jobs (.+) should NOT depend on each other$`, tc.jobsShouldNotDependOnEachOther)
	ctx.Then(`^"([^"]*)" pipeline should trigger on "([^"]*)"$`, tc.pipelineShouldTriggerOn)
	ctx.Then(`^"([^"]*)" pipeline should trigger on "([^"]*)" to "([^"]*)" branch$`, tc.pipelineShouldTriggerOnBranch)
	ctx.Given(`^the "([^"]*)" phase caches "([^"]*)" keyed on "([^"]*)"$`, tc.phaseCachesKeyedOn)
	ctx.Given(`^the "([^"]*)" phase caches "([^"]*)" keyed on nothing$`, tc.phaseCachesKeyedOnNothing)
	ctx.Given(`^the "([^"]*)" phase caches "([^"]*)" keyed on "([^"]*)" without falling back to an older cache$`, tc.phaseCachesWithoutFallback)
	ctx.Then(`^the cache of the "([^"]*)" job should not fall back to an older cache$`, tc.cacheShouldNotFallBack)
	ctx.Given(`^the "([^"]*)" phase uploads "([^"]*)" from "([^"]*)" for (\d+) days$`, tc.phaseUploads)
	ctx.Then(`^the "([^"]*)" job should upload "([^"]*)" after the phase runs, even when it fails$`, tc.jobShouldUploadAfterPhase)
	ctx.Then(`^that upload should keep hidden files and only warn when nothing matches$`, tc.uploadShouldKeepHiddenAndWarn)
	ctx.Then(`^that upload should keep the artifact for (\d+) days$`, tc.uploadShouldKeepDays)
	ctx.Then(`^the "([^"]*)" job should have no upload step$`, tc.jobShouldHaveNoUploadStep)
	ctx.Then(`^the "([^"]*)" job should cache "([^"]*)"$`, tc.jobShouldCache)
	ctx.Then(`^the cache key of the "([^"]*)" job should hash "([^"]*)"$`, tc.cacheKeyShouldHash)
	ctx.Then(`^the cache of the "([^"]*)" job should fall back to an older cache of that phase$`, tc.cacheShouldFallBack)
	ctx.Then(`^the cache keys of the "([^"]*)" and "([^"]*)" jobs should differ$`, tc.cacheKeysShouldDiffer)
	ctx.Then(`^the "([^"]*)" job should have no cache step$`, tc.jobShouldHaveNoCacheStep)
	ctx.Then(`^generating should fail mentioning "([^"]*)"$`, tc.generatingShouldFailMentioning)
	ctx.Then(`^the workflow should group runs by pull request number$`, tc.workflowGroupsByPullRequest)
	ctx.Then(`^the workflow should cancel a run its group supersedes$`, tc.workflowCancelsSuperseded)
	ctx.Then(`^the workflow should give a run that is not a pull request a group of its own$`, tc.workflowGivesOtherRunsOwnGroup)
	ctx.Then(`^the output should be printed to stdout$`, tc.outputShouldBePrintedToStdout)
	ctx.Then(`^the file "([^"]*)" should be created$`, tc.fileShouldBeCreated)
}

// generatedWorkflow is the part of a generated GitHub Actions workflow the
// scenarios talk about, read back from the YAML the generator emitted.
type generatedWorkflow struct {
	Name string `yaml:"name"`
	On   struct {
		Push        *triggerYAML `yaml:"push"`
		PullRequest *triggerYAML `yaml:"pull_request"`
	} `yaml:"on"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Name  string   `yaml:"name"`
		Needs []string `yaml:"needs"`
		Steps []struct {
			Name string `yaml:"name"`
			If   string `yaml:"if"`
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
			With struct {
				Name          string `yaml:"name"`
				Path          string `yaml:"path"`
				Key           string `yaml:"key"`
				RestoreKeys   string `yaml:"restore-keys"`
				IncludeHidden string `yaml:"include-hidden-files"`
				IfNoFiles     string `yaml:"if-no-files-found"`
				Retention     string `yaml:"retention-days"`
			} `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

type triggerYAML struct {
	Branches []string `yaml:"branches"`
	Tags     []string `yaml:"tags"`
}

func (tc *TestContext) configDefinesPipeline(name, phasesStr string) error {
	var phases []string
	for _, phase := range strings.Split(phasesStr, ",") {
		phases = append(phases, strings.TrimSpace(phase))
	}
	tc.declarePipeline(name, phases)
	return nil
}

func (tc *TestContext) configDefinesPipelineOnly(name string) error {
	// Phases a pipeline of that name carries by convention, the ones `cidx init`
	// writes for it.
	defaults := map[string][]string{
		"pr":      {"security", "code", "test"},
		"main":    {"security", "code", "test", "build"},
		"ci":      {"security", "code", "test", "build"},
		"release": {"security", "code", "test", "build", "release", "docker"},
	}
	phases := defaults[name]
	if phases == nil {
		phases = []string{"security", "code"}
	}
	tc.declarePipeline(name, phases)
	return nil
}

func (tc *TestContext) declarePipeline(name string, phases []string) {
	pipelines, ok := tc.Config["pipelines"].(map[string][]string)
	if !ok {
		pipelines = make(map[string][]string)
		tc.Config["pipelines"] = pipelines
	}
	pipelines[name] = phases
}

func (tc *TestContext) configHasNoPipelines() error {
	tc.Config["no_pipelines"] = true
	return nil
}

// runGenerate produces the CI configuration for the platform named on the
// command line, from the staged cidx.toml.
func (tc *TestContext) runGenerate(args []string) error {
	platform := ""
	if len(args) >= 3 {
		platform = args[2]
	}
	if platform != "github" && platform != "gitlab" {
		return tc.rejectUnknownPlatform(platform)
	}

	cfg, err := tc.loadStagedConfig()
	if err != nil {
		return err
	}

	outputPath := outputFlag(args)

	var output string
	if platform == "github" {
		output, err = generate.GitHub(cfg)
	} else {
		output, err = generate.GitLab(cfg, outputPath)
	}
	if err != nil {
		tc.Output = fmt.Sprintf("Error: %v\n", err)
		tc.ExitCode = 1
		return nil
	}

	tc.Output = output
	tc.Config["generated_workflow"] = output

	if outputPath != "" {
		dir, err := tc.scenarioDir()
		if err != nil {
			return err
		}
		target := filepath.Join(dir, outputPath)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, []byte(output), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
	}
	return nil
}

// rejectUnknownPlatform answers an unknown platform the way the CLI does.
//
// `cidx generate` is a namespace whose only subcommands are the platforms it
// can produce, so an unknown one never reaches an action of ours: urfave/cli
// rejects it first. The tree below mirrors that shape — the real one is built
// in cmd/cidx/generate.go.
func (tc *TestContext) rejectUnknownPlatform(platform string) error {
	var out strings.Builder
	app := &cli.App{
		Name:           "cidx",
		Writer:         &out,
		ErrWriter:      &out,
		ExitErrHandler: func(*cli.Context, error) {},
		Commands: []*cli.Command{{
			Name: "generate",
			Subcommands: []*cli.Command{
				{Name: "github", Action: func(*cli.Context) error { return nil }},
				{Name: "gitlab", Action: func(*cli.Context) error { return nil }},
			},
		}},
	}

	err := app.Run([]string{"cidx", "generate", platform})
	tc.Output = out.String()
	if err == nil {
		return nil
	}

	tc.Output += err.Error() + "\n"
	tc.ExitCode = 1
	if coder, ok := err.(cli.ExitCoder); ok {
		tc.ExitCode = coder.ExitCode()
	}
	return nil
}

// outputFlag reads the -o/--output path off the command line.
func outputFlag(args []string) string {
	for i, arg := range args {
		if (arg == "-o" || arg == "--output") && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--output=") {
			return strings.TrimPrefix(arg, "--output=")
		}
	}
	return ""
}

// workflow parses the generated YAML — a workflow that does not parse is a
// workflow GitHub would reject.
func (tc *TestContext) workflow() (*generatedWorkflow, error) {
	output, ok := tc.Config["generated_workflow"].(string)
	if !ok {
		return nil, fmt.Errorf("no workflow was generated in this scenario (output: %s)", tc.Output)
	}
	var parsed generatedWorkflow
	if err := yaml.Unmarshal([]byte(output), &parsed); err != nil {
		return nil, fmt.Errorf("the generated workflow is not valid YAML: %w\n%s", err, output)
	}
	return &parsed, nil
}

func (tc *TestContext) outputShouldBeValidYAML() error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	if parsed.Name == "" {
		return fmt.Errorf("the generated workflow has no name:\n%s", tc.Output)
	}
	if len(parsed.Jobs) == 0 {
		return fmt.Errorf("the generated workflow has no jobs:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) outputShouldContainJob(jobName string) error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	if _, ok := parsed.Jobs[jobName]; !ok {
		return fmt.Errorf("no %q job in the generated workflow (jobs: %s)", jobName, jobNames(parsed))
	}
	return nil
}

// eachPhaseShouldHaveJob checks every declared phase runs as its own job, and
// that the job actually runs that phase.
func (tc *TestContext) eachPhaseShouldHaveJob() error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}

	for _, phases := range tc.declaredPipelines() {
		for _, phase := range phases {
			job, ok := parsed.Jobs[phase]
			if !ok {
				return fmt.Errorf("phase %q has no job (jobs: %s)", phase, jobNames(parsed))
			}
			wanted := "cidx run " + phase
			found := false
			for _, step := range job.Steps {
				if strings.Contains(step.Run, wanted) {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("the %q job never runs %q", phase, wanted)
			}
		}
	}
	return nil
}

func (tc *TestContext) jobsShouldDependOn(jobsStr, dependency string) error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	for _, name := range quotedList(jobsStr) {
		job, ok := parsed.Jobs[name]
		if !ok {
			return fmt.Errorf("no %q job in the generated workflow (jobs: %s)", name, jobNames(parsed))
		}
		if !contains(job.Needs, dependency) {
			return fmt.Errorf("job %q needs %v, want it to depend on %q", name, job.Needs, dependency)
		}
	}
	return nil
}

// jobsShouldNotDependOnEachOther is what makes the phases run in parallel: a
// phase job waits for the bootstrap, never for another phase.
func (tc *TestContext) jobsShouldNotDependOnEachOther(jobsStr string) error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	names := quotedList(jobsStr)
	for _, name := range names {
		job, ok := parsed.Jobs[name]
		if !ok {
			return fmt.Errorf("no %q job in the generated workflow (jobs: %s)", name, jobNames(parsed))
		}
		for _, other := range names {
			if other != name && contains(job.Needs, other) {
				return fmt.Errorf("job %q depends on %q, so they cannot run in parallel", name, other)
			}
		}
	}
	return nil
}

// pipelineShouldTriggerOn checks the event a pipeline name maps to is one the
// generated workflow actually declares.
func (tc *TestContext) pipelineShouldTriggerOn(pipeline, event string) error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	if _, declared := tc.declaredPipelines()[pipeline]; !declared {
		return fmt.Errorf("the scenario never declared pipeline %q", pipeline)
	}

	switch event {
	case "push":
		if parsed.On.Push == nil {
			return fmt.Errorf("pipeline %q should trigger on push, the workflow declares no push trigger:\n%s", pipeline, tc.Output)
		}
	case "pull_request":
		if parsed.On.PullRequest == nil {
			return fmt.Errorf("pipeline %q should trigger on pull_request, the workflow declares no pull_request trigger:\n%s", pipeline, tc.Output)
		}
	default:
		return fmt.Errorf("unsupported trigger %q", event)
	}
	return nil
}

func (tc *TestContext) pipelineShouldTriggerOnBranch(pipeline, event, branch string) error {
	if err := tc.pipelineShouldTriggerOn(pipeline, event); err != nil {
		return err
	}
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}

	branches := []string(nil)
	if event == "push" {
		branches = parsed.On.Push.Branches
	} else {
		branches = parsed.On.PullRequest.Branches
	}
	if !contains(branches, branch) {
		return fmt.Errorf("the %s trigger targets %v, want the %q branch", event, branches, branch)
	}
	return nil
}

// outputShouldBePrintedToStdout checks the workflow came back to the caller
// rather than being written somewhere. The stdout plumbing itself belongs to
// cmd/cidx (writeGeneratedOutput).
func (tc *TestContext) outputShouldBePrintedToStdout() error {
	if _, err := tc.workflow(); err != nil {
		return err
	}
	if tc.Output == "" {
		return fmt.Errorf("nothing was printed")
	}
	return nil
}

func (tc *TestContext) fileShouldBeCreated(path string) error {
	dir, err := tc.scenarioDir()
	if err != nil {
		return err
	}
	written, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		return fmt.Errorf("expected %s to be created: %w", path, err)
	}

	generated, _ := tc.Config["generated_workflow"].(string)
	if string(written) != generated {
		return fmt.Errorf("%s does not hold the generated workflow:\n%s", path, written)
	}
	return nil
}

// quotedList reads a Gherkin enumeration such as `"security", "code", "test"`.
func quotedList(list string) []string {
	var names []string
	for _, item := range strings.Split(list, ",") {
		names = append(names, strings.Trim(strings.TrimSpace(item), `"`))
	}
	return names
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func jobNames(parsed *generatedWorkflow) string {
	names := make([]string, 0, len(parsed.Jobs))
	for name := range parsed.Jobs {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

func (tc *TestContext) workflowGroupsByPullRequest() error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	if g := parsed.Concurrency.Group; !strings.Contains(g, "github.workflow") || !strings.Contains(g, "github.event.pull_request.number") {
		return fmt.Errorf("the concurrency group %q does not key on the workflow and the pull request number", g)
	}
	return nil
}

func (tc *TestContext) workflowCancelsSuperseded() error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	if !parsed.Concurrency.CancelInProgress {
		return fmt.Errorf("the workflow does not cancel a run its group supersedes")
	}
	return nil
}

// workflowGivesOtherRunsOwnGroup: a push or a tag has no pull request number,
// so the group must fall back to something unique to the run.
func (tc *TestContext) workflowGivesOtherRunsOwnGroup() error {
	parsed, err := tc.workflow()
	if err != nil {
		return err
	}
	if g := parsed.Concurrency.Group; !strings.Contains(g, "github.event.pull_request.number || github.run_id") {
		return fmt.Errorf("the concurrency group %q falls back to a value runs share, so a push could be cancelled or queued behind another", g)
	}
	return nil
}

type phaseCache struct {
	path, key  string
	noFallback bool
}

// phaseCaches is what the scenario said each phase caches; writeStagedConfig
// writes it into the phase's table.
func (tc *TestContext) phaseCaches() map[string]phaseCache {
	caches, _ := tc.Config["phase_caches"].(map[string]phaseCache)
	return caches
}

func (tc *TestContext) phaseCachesKeyedOn(phase, path, key string) error {
	caches := tc.phaseCaches()
	if caches == nil {
		caches = map[string]phaseCache{}
		tc.Config["phase_caches"] = caches
	}
	caches[phase] = phaseCache{path: path, key: key}
	return nil
}

func (tc *TestContext) phaseCachesKeyedOnNothing(phase, path string) error {
	return tc.phaseCachesKeyedOn(phase, path, "")
}

// cacheStep finds the actions/cache step of a job.
func (tc *TestContext) cacheStep(job string) (path, key, restore string, found bool, err error) {
	parsed, err := tc.workflow()
	if err != nil {
		return "", "", "", false, err
	}
	j, ok := parsed.Jobs[job]
	if !ok {
		return "", "", "", false, fmt.Errorf("no %q job (jobs: %s)", job, jobNames(parsed))
	}
	for _, step := range j.Steps {
		if strings.HasPrefix(step.Uses, "actions/cache@") {
			return step.With.Path, step.With.Key, step.With.RestoreKeys, true, nil
		}
	}
	return "", "", "", false, nil
}

func (tc *TestContext) jobShouldCache(job, path string) error {
	got, _, _, found, err := tc.cacheStep(job)
	if err != nil {
		return err
	}
	if !found || !strings.Contains(got, path) {
		return fmt.Errorf("the %q job does not cache %q (cache step found: %v, path: %q)", job, path, found, got)
	}
	return nil
}

func (tc *TestContext) cacheKeyShouldHash(job, file string) error {
	_, key, _, found, err := tc.cacheStep(job)
	if err != nil {
		return err
	}
	if !found || !strings.Contains(key, "hashFiles(") || !strings.Contains(key, file) {
		return fmt.Errorf("the cache key %q of %q does not hash %q", key, job, file)
	}
	return nil
}

func (tc *TestContext) cacheShouldFallBack(job string) error {
	_, key, restore, found, err := tc.cacheStep(job)
	if err != nil {
		return err
	}
	prefix := strings.TrimSpace(restore)
	if !found || prefix == "" || !strings.HasPrefix(key, prefix) || !strings.Contains(prefix, "-"+job+"-") {
		return fmt.Errorf("restore-keys %q is not a prefix of the key %q that names the %q phase", restore, key, job)
	}
	return nil
}

func (tc *TestContext) cacheKeysShouldDiffer(a, b string) error {
	_, keyA, _, foundA, err := tc.cacheStep(a)
	if err != nil {
		return err
	}
	_, keyB, _, foundB, err := tc.cacheStep(b)
	if err != nil {
		return err
	}
	if !foundA || !foundB || keyA == keyB {
		return fmt.Errorf("the %q and %q jobs share a cache key (%q): debug and release artefacts would evict each other", a, b, keyA)
	}
	return nil
}

func (tc *TestContext) jobShouldHaveNoCacheStep(job string) error {
	_, _, _, found, err := tc.cacheStep(job)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("the %q job has a cache step it never declared", job)
	}
	return nil
}

func (tc *TestContext) generatingShouldFailMentioning(fragment string) error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("generating succeeded, expected a failure mentioning %q:\n%s", fragment, tc.Output)
	}
	if !strings.Contains(tc.Output, fragment) {
		return fmt.Errorf("the failure %q does not mention %q", tc.Output, fragment)
	}
	return nil
}

type phaseArtifact struct {
	name, path string
	days       int
}

// phaseArtifacts is what the scenario said each phase uploads; writeStagedConfig
// writes it into the phase's table.
func (tc *TestContext) phaseArtifacts() map[string]phaseArtifact {
	artifacts, _ := tc.Config["phase_artifacts"].(map[string]phaseArtifact)
	return artifacts
}

func (tc *TestContext) phaseUploads(phase, name, path string, days int) error {
	artifacts := tc.phaseArtifacts()
	if artifacts == nil {
		artifacts = map[string]phaseArtifact{}
		tc.Config["phase_artifacts"] = artifacts
	}
	artifacts[phase] = phaseArtifact{name: name, path: path, days: days}
	return nil
}

// uploadStep finds the actions/upload-artifact step of a job that is not the
// bootstrap hand-off, and where it sits relative to the step that runs the phase.
func (tc *TestContext) uploadStep(job string) (idx, runIdx int, found bool, err error) {
	parsed, err := tc.workflow()
	if err != nil {
		return 0, 0, false, err
	}
	j, ok := parsed.Jobs[job]
	if !ok {
		return 0, 0, false, fmt.Errorf("no %q job (jobs: %s)", job, jobNames(parsed))
	}
	idx, runIdx = -1, -1
	for i, step := range j.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			idx = i
		}
		if strings.HasPrefix(step.Run, "./bin/cidx run ") {
			runIdx = i
		}
	}
	tc.Config["upload_job"] = job
	return idx, runIdx, idx >= 0, nil
}

func (tc *TestContext) jobShouldUploadAfterPhase(job, name string) error {
	idx, runIdx, found, err := tc.uploadStep(job)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("the %q job uploads nothing", job)
	}
	parsed, _ := tc.workflow()
	step := parsed.Jobs[job].Steps[idx]
	switch {
	case step.With.Name != name:
		return fmt.Errorf("the %q job uploads %q, not %q", job, step.With.Name, name)
	case idx < runIdx:
		return fmt.Errorf("the upload comes before the step that runs the phase")
	case step.If != "always()":
		return fmt.Errorf("the upload runs `if: %s`, so a failed phase would upload nothing", step.If)
	}
	return nil
}

func (tc *TestContext) uploadWith() (hidden, ifNone, retention string, err error) {
	job, _ := tc.Config["upload_job"].(string)
	idx, _, found, err := tc.uploadStep(job)
	if err != nil || !found {
		return "", "", "", fmt.Errorf("no upload step to check (job %q): %v", job, err)
	}
	parsed, _ := tc.workflow()
	with := parsed.Jobs[job].Steps[idx].With
	return with.IncludeHidden, with.IfNoFiles, with.Retention, nil
}

func (tc *TestContext) uploadShouldKeepHiddenAndWarn() error {
	hidden, ifNone, _, err := tc.uploadWith()
	if err != nil {
		return err
	}
	if hidden != "true" || ifNone != "warn" {
		return fmt.Errorf("include-hidden-files=%q if-no-files-found=%q, want true and warn", hidden, ifNone)
	}
	return nil
}

func (tc *TestContext) uploadShouldKeepDays(days int) error {
	_, _, retention, err := tc.uploadWith()
	if err != nil {
		return err
	}
	if retention != fmt.Sprint(days) {
		return fmt.Errorf("retention-days=%q, want %d", retention, days)
	}
	return nil
}

func (tc *TestContext) jobShouldHaveNoUploadStep(job string) error {
	_, _, found, err := tc.uploadStep(job)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("the %q job uploads something it never declared", job)
	}
	return nil
}

func (tc *TestContext) phaseCachesWithoutFallback(phase, path, key string) error {
	if err := tc.phaseCachesKeyedOn(phase, path, key); err != nil {
		return err
	}
	c := tc.phaseCaches()[phase]
	c.noFallback = true
	tc.phaseCaches()[phase] = c
	return nil
}

func (tc *TestContext) cacheShouldNotFallBack(job string) error {
	_, _, restore, found, err := tc.cacheStep(job)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("the %q job has no cache step", job)
	}
	if strings.TrimSpace(restore) != "" {
		return fmt.Errorf("the %q job still restores an older cache (restore-keys %q): it would stack a generation per key change", job, restore)
	}
	return nil
}
