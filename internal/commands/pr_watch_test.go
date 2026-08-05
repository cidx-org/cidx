package commands

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/branch"
)

// redPullRequest is the state a watch closes on when CI has just gone red:
// every check finished, one of them did not pass.
func redPullRequest() *branch.PRInfo {
	return &branch.PRInfo{
		Number: 354,
		Title:  "fix(cli): name the failing check",
		URL:    "https://github.com/cidx-org/cidx/pull/354",
		Checks: &branch.PRChecksInfo{
			Total: 5, Success: 4, Failure: 1, Status: "failure",
			Failed: []branch.FailedCheck{{Name: "Test", Step: "Run tests"}},
		},
	}
}

// stagePRInfo makes the watch loops read info instead of GitHub, so what they
// print can be asserted without a network.
func stagePRInfo(t *testing.T, info *branch.PRInfo) {
	t.Helper()

	original := readPRInfo
	readPRInfo = func(*branch.Manager, string) (*branch.PRInfo, error) { return info, nil }
	t.Cleanup(func() { readPRInfo = original })
}

// TestWatchPRChecksQuiet_NamesTheFailingCheck covers #347 for the form `cidx
// cpw` and `cidx pr watch -q` end on: "Some checks failed." said which of five
// had failed only in the web UI.
func TestWatchPRChecksQuiet_NamesTheFailingCheck(t *testing.T) {
	info := redPullRequest()
	stagePRInfo(t, info)

	var err error
	out := captureStdout(t, func() { err = watchPRChecksQuiet(nil, "fix/naming", info) })
	if err != nil {
		t.Fatalf("watchPRChecksQuiet: %v", err)
	}

	if !strings.Contains(out, "Some checks failed.") {
		t.Errorf("expected the verdict, got:\n%s", out)
	}
	if !strings.Contains(out, "✗ Test — failed step: Run tests") {
		t.Errorf("expected the failing check to be named, got:\n%s", out)
	}
}

// TestWatchPRChecks_NamesTheFailingCheck: the animated form -- the default of
// `cidx pr watch` -- closes on the same report.
func TestWatchPRChecks_NamesTheFailingCheck(t *testing.T) {
	info := redPullRequest()
	stagePRInfo(t, info)

	var err error
	out := captureStdout(t, func() { err = watchPRChecks(nil, "fix/naming", info, false) })
	if err != nil {
		t.Fatalf("watchPRChecks: %v", err)
	}

	if !strings.Contains(out, "Some checks failed") {
		t.Errorf("expected the verdict, got:\n%s", out)
	}
	if !strings.Contains(out, "✗ Test — failed step: Run tests") {
		t.Errorf("expected the failing check to be named, got:\n%s", out)
	}
}

// TestWatchPRChecksQuiet_GreenRunNamesNothing: the report is added to a red
// run, not to every run.
func TestWatchPRChecksQuiet_GreenRunNamesNothing(t *testing.T) {
	info := &branch.PRInfo{
		Number: 354,
		Checks: &branch.PRChecksInfo{Total: 5, Success: 5, Status: "success"},
	}
	stagePRInfo(t, info)

	var err error
	out := captureStdout(t, func() { err = watchPRChecksQuiet(nil, "fix/naming", info) })
	if err != nil {
		t.Fatalf("watchPRChecksQuiet: %v", err)
	}

	if !strings.Contains(out, "All checks passed.") {
		t.Errorf("expected the verdict, got:\n%s", out)
	}
	if strings.Contains(out, "✗") {
		t.Errorf("a green run should carry no failure marker, got:\n%s", out)
	}
}
