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
