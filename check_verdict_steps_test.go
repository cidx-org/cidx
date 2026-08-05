package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/internal/commands"
	"github.com/cucumber/godog"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// RegisterCheckVerdictSteps registers the steps for the channel the verdict of
// `cidx check workflow` goes out on (issue #345).
//
// The command runs for real, against the tree cidx ships (#317), over a project
// the scenario writes in a temporary directory. What reaches stdout and what
// reaches logrus are collected apart, which is the whole point: the defect was
// the same line arriving on both.
func RegisterCheckVerdictSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a project whose "([^"]*)" pipeline runs phases "([^"]*)"$`, tc.aProjectWhosePipelineRunsPhases)
	ctx.Given(`^its workflow runs phases "([^"]*)"$`, tc.itsWorkflowRunsPhases)
	ctx.Given(`^it has no workflow at all$`, tc.itHasNoWorkflowAtAll)

	ctx.When(`^I run cidx check workflow$`, func() error { return tc.runCheckWorkflow("") })
	ctx.When(`^I run cidx check workflow (\S+)$`, tc.runCheckWorkflow)

	ctx.Then(`^stdout says "([^"]*)" once$`, tc.stdoutSaysOnce)
	ctx.Then(`^nothing is logged$`, tc.nothingIsLogged)
	ctx.Then(`^the command fails$`, tc.theCommandFails)
}

func (tc *TestContext) aProjectWhosePipelineRunsPhases(pipeline, phases string) error {
	dir, err := os.MkdirTemp("", "cidx-check-verdict-")
	if err != nil {
		return err
	}
	tc.GitRepo = dir // removed by the scenario cleanup

	var config strings.Builder
	names := splitPhases(phases)
	for _, phase := range names {
		fmt.Fprintf(&config, "[%s]\ncontainers = []\n\n", phase)
	}
	fmt.Fprintf(&config, "[pipelines.%s]\nphases = [\"%s\"]\n", pipeline, strings.Join(names, "\", \""))

	if err := os.WriteFile(filepath.Join(dir, "cidx.toml"), []byte(config.String()), 0644); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0755)
}

// itsWorkflowRunsPhases writes the ci.yml the pipeline is compared against: one
// job per phase, chained, each invoking cidx the way a real workflow does.
func (tc *TestContext) itsWorkflowRunsPhases(phases string) error {
	var workflow strings.Builder
	workflow.WriteString("name: CI\njobs:\n")

	names := splitPhases(phases)
	for i, phase := range names {
		fmt.Fprintf(&workflow, "  %s:\n", phase)
		if i > 0 {
			fmt.Fprintf(&workflow, "    needs: [%s]\n", names[i-1])
		}
		fmt.Fprintf(&workflow, "    steps:\n      - run: cidx run %s\n", phase)
	}

	return os.WriteFile(tc.workflowPath("ci.yml"), []byte(workflow.String()), 0644)
}

func (tc *TestContext) itHasNoWorkflowAtAll() error {
	return os.RemoveAll(tc.workflowPath("ci.yml"))
}

func (tc *TestContext) workflowPath(name string) string {
	return filepath.Join(tc.GitRepo, ".github", "workflows", name)
}

// runCheckWorkflow runs the command over the staged project and splits its
// output by channel. The project is addressed by flag rather than by changing
// directory, so a scenario never leaves the suite in a directory it deletes.
//
// ExitErrHandler is neutralised because `check workflow` signals a difference
// with cli.Exit, and urfave's default handler would take the test binary down
// with the process.
func (tc *TestContext) runCheckWorkflow(pipeline string) error {
	if tc.GitRepo == "" {
		return fmt.Errorf("no project was staged")
	}

	args := []string{
		"cidx",
		"--config", filepath.Join(tc.GitRepo, "cidx.toml"),
		"check", "workflow",
		"--workflow-dir", filepath.Join(tc.GitRepo, ".github", "workflows"),
	}
	if pipeline != "" {
		args = append(args, pipeline)
	}

	var log bytes.Buffer
	logrus.SetOutput(&log)
	defer logrus.SetOutput(os.Stderr)

	app := commands.NewApp()
	app.ExitErrHandler = func(*cli.Context, error) {}

	stdout, runErr := captureOutput(func() error { return app.Run(args) })

	tc.Output = stdout
	tc.Config["logged"] = log.String()
	tc.ExitCode = 0
	if runErr != nil {
		tc.ExitCode = 1
	}
	return nil
}

func (tc *TestContext) stdoutSaysOnce(text string) error {
	if got := strings.Count(tc.Output, text); got != 1 {
		return fmt.Errorf("expected %q once on stdout, got %d:\n%s", text, got, tc.Output)
	}
	return nil
}

func (tc *TestContext) nothingIsLogged() error {
	logged, _ := tc.Config["logged"].(string)
	if logged != "" {
		return fmt.Errorf("the verdict has one channel; logrus wrote:\n%s", logged)
	}
	return nil
}

func (tc *TestContext) theCommandFails() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("the command succeeded:\n%s", tc.Output)
	}
	return nil
}

func splitPhases(phases string) []string {
	fields := strings.Split(phases, ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}
