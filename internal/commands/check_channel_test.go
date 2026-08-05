package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// workflowProject writes a project whose "ci" pipeline runs the given phases
// and whose ci.yml runs workflowPhases, and makes it the working directory.
// Passing the same list twice describes a project in sync.
func workflowProject(t *testing.T, phases, workflowPhases []string) string {
	t.Helper()

	dir := t.TempDir()

	var config strings.Builder
	for _, phase := range phases {
		config.WriteString("[" + phase + "]\ncontainers = []\n\n")
	}
	config.WriteString("[pipelines.ci]\nphases = [\"" + strings.Join(phases, "\", \"") + "\"]\n")
	if err := os.WriteFile(filepath.Join(dir, "cidx.toml"), []byte(config.String()), 0644); err != nil {
		t.Fatal(err)
	}

	workflowDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		t.Fatal(err)
	}

	workflow := strings.Builder{}
	workflow.WriteString("name: CI\njobs:\n")
	for i, phase := range workflowPhases {
		workflow.WriteString("  " + phase + ":\n")
		if i > 0 {
			workflow.WriteString("    needs: [" + workflowPhases[i-1] + "]\n")
		}
		workflow.WriteString("    steps:\n      - run: cidx run " + phase + "\n")
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte(workflow.String()), 0644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)
	return dir
}

// runCheckWorkflow runs `cidx check workflow ...` against the real command tree
// and returns what reached stdout and what reached logrus. Nothing is allowed
// to exit the process: `check workflow` signals a difference with cli.Exit, and
// urfave's default handler would take the test binary down with it.
func runCheckWorkflow(t *testing.T, args ...string) (stdout, logged string) {
	t.Helper()

	var log bytes.Buffer
	logrus.SetOutput(&log)
	t.Cleanup(func() { logrus.SetOutput(os.Stderr) })

	app := NewApp()
	app.ExitErrHandler = func(_ *cli.Context, _ error) {}

	stdout = captureStdout(t, func() {
		_ = app.Run(append([]string{"cidx", "check", "workflow"}, args...))
	})
	return stdout, log.String()
}

// TestCheckWorkflow_VerdictReachesStdoutOnlyOnce is the regression for #345:
// the summary went out twice, through logrus to stderr and plain to stdout, in
// two different formats. The line a user has to read appeared in duplicate, and
// a script capturing stdout got a string that was not the one on screen.
func TestCheckWorkflow_VerdictReachesStdoutOnlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name           string
		phases         []string
		workflowPhases []string
		verdict        string
	}{
		{
			name:           "in sync",
			phases:         []string{"security", "code"},
			workflowPhases: []string{"security", "code"},
			verdict:        "Pipeline 'ci' is in sync with its workflow",
		},
		{
			name:           "drifted",
			phases:         []string{"security", "code"},
			workflowPhases: []string{"security"},
			verdict:        "Pipeline 'ci' has differences with its workflow",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflowProject(t, tc.phases, tc.workflowPhases)

			stdout, logged := runCheckWorkflow(t, "ci")

			if got := strings.Count(stdout, tc.verdict); got != 1 {
				t.Errorf("expected the verdict once on stdout, got %d:\n%s", got, stdout)
			}
			if logged != "" {
				t.Errorf("the verdict has one channel; logrus wrote:\n%s", logged)
			}
		})
	}
}

// TestCheckWorkflow_SweepVerdictReachesStdoutOnlyOnce: the flagless form
// summarises the whole repository and had the same duplication.
func TestCheckWorkflow_SweepVerdictReachesStdoutOnlyOnce(t *testing.T) {
	workflowProject(t, []string{"security", "code"}, []string{"security", "code"})

	stdout, logged := runCheckWorkflow(t)

	const verdict = "All 1 workflow(s) are in sync with pipelines"
	if got := strings.Count(stdout, verdict); got != 1 {
		t.Errorf("expected the verdict once on stdout, got %d:\n%s", got, stdout)
	}
	if logged != "" {
		t.Errorf("the verdict has one channel; logrus wrote:\n%s", logged)
	}
}

// TestCheckWorkflow_ErrorsAreNotAlsoLogged: an error is returned to urfave,
// which reports it. Logging it first printed the same failure twice, the same
// defect as the summary (issue #345).
func TestCheckWorkflow_ErrorsAreNotAlsoLogged(t *testing.T) {
	dir := workflowProject(t, []string{"security"}, []string{"security"})
	if err := os.Remove(filepath.Join(dir, ".github", "workflows", "ci.yml")); err != nil {
		t.Fatal(err)
	}

	_, logged := runCheckWorkflow(t, "ci")

	if logged != "" {
		t.Errorf("the error has one channel; logrus wrote:\n%s", logged)
	}
}

// TestCheckWorkflow_NoWorkflowFoundIsSaidOnce: the empty-sweep notice was a
// logrus warning and a printed line at the same time.
func TestCheckWorkflow_NoWorkflowFoundIsSaidOnce(t *testing.T) {
	dir := workflowProject(t, []string{"security"}, []string{"security"})
	if err := os.Remove(filepath.Join(dir, ".github", "workflows", "ci.yml")); err != nil {
		t.Fatal(err)
	}

	stdout, logged := runCheckWorkflow(t)

	if !strings.Contains(stdout, "No GitHub Actions workflows found") {
		t.Errorf("expected the notice on stdout, got:\n%s", stdout)
	}
	if logged != "" {
		t.Errorf("the notice has one channel; logrus wrote:\n%s", logged)
	}
}
