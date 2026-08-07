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
	Jobs map[string]struct {
		Name  string   `yaml:"name"`
		Needs []string `yaml:"needs"`
		Steps []struct {
			Name string `yaml:"name"`
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
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
