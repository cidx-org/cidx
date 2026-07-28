package executor

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/sirupsen/logrus"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
// printDryRun writes with fmt.Printf — it is user-facing output, not logging.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w

	fn()

	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

// TestPrintDryRun_ShowsPullPolicyAndTimeout covers #211: both fields drive the
// real run (pull decision and context deadline), so a dry-run that hides them
// describes a different execution than the one that would happen.
func TestPrintDryRun_ShowsPullPolicyAndTimeout(t *testing.T) {
	e := &DockerExecutor{logger: logrus.New(), dryRun: true}
	cfg := &config.ContainerConfig{
		Name:       "slow-scanner",
		Image:      "alpine:latest",
		Workdir:    "/work",
		Workspace:  "/tmp/project",
		PullPolicy: "always",
		Timeout:    "45m",
	}

	out := captureStdout(t, func() {
		e.printDryRun(cfg, []string{"/tmp/project:/work"}, "scan .")
	})

	for _, want := range []string{"Pull policy: always", "Timeout: 45m"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

// TestPrintDryRun_OmitsUnsetFields keeps the output honest: an unset field is
// not printed as an empty value.
func TestPrintDryRun_OmitsUnsetFields(t *testing.T) {
	e := &DockerExecutor{logger: logrus.New(), dryRun: true}
	cfg := &config.ContainerConfig{
		Name:      "plain",
		Image:     "alpine:latest",
		Workdir:   "/work",
		Workspace: "/tmp/project",
	}

	out := captureStdout(t, func() {
		e.printDryRun(cfg, []string{"/tmp/project:/work"}, "echo hi")
	})

	for _, unwanted := range []string{"Pull policy:", "Timeout:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("dry-run output should not mention %q when unset:\n%s", unwanted, out)
		}
	}
}

// TestRunLabel_OmitsEmptyPhaseBracket covers #230: a custom [containers.x]
// declaration carries no phase, and the run line rendered the bracket anyway —
// "▸ Running [] my-tool" reads like a bug.
func TestRunLabel_OmitsEmptyPhaseBracket(t *testing.T) {
	if got := runLabel(&config.ContainerConfig{Name: "my-tool"}); got != "my-tool" {
		t.Errorf("a container with no phase must not print an empty bracket, got %q", got)
	}

	withPhase := &config.ContainerConfig{Name: "trivy", Phase: "security"}
	if got := runLabel(withPhase); got != "[security] trivy" {
		t.Errorf("a phased container keeps its bracket, got %q", got)
	}
}

// TestPrintDryRun_EnvironmentOrderIsStable covers #230: the environment block
// used to be printed in map iteration order, so two dry-runs of the same
// config differed and diffing them to review a change showed phantom edits.
func TestPrintDryRun_EnvironmentOrderIsStable(t *testing.T) {
	e := &DockerExecutor{logger: logrus.New(), dryRun: true}
	cfg := &config.ContainerConfig{
		Name:      "envy",
		Image:     "alpine:latest",
		Workdir:   "/work",
		Workspace: "/tmp/project",
		Env: map[string]string{
			"ZULU": "1", "ALPHA": "2", "MIKE": "3", "BRAVO": "4",
			"YANKEE": "5", "CHARLIE": "6", "DELTA": "7", "ECHO": "8",
		},
	}

	first := captureStdout(t, func() { e.printDryRun(cfg, nil, "run") })

	// Map iteration is randomized per range statement, so a handful of repeats
	// reliably catches an unsorted printer.
	for i := range 20 {
		if again := captureStdout(t, func() { e.printDryRun(cfg, nil, "run") }); again != first {
			t.Fatalf("dry-run %d differs from the first on identical input:\n%s\n---\n%s", i, first, again)
		}
	}

	want := "    ALPHA=2\n    BRAVO=4\n    CHARLIE=6\n    DELTA=7\n    ECHO=8\n    MIKE=3\n    YANKEE=5\n    ZULU=1\n"
	if !strings.Contains(first, want) {
		t.Errorf("environment should be printed in key order:\n%s", first)
	}
}
