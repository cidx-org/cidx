package commands

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/branch"
)

// redPullRequest is the state a watch closes on when CI has just gone red:
// every check finished, one of them did not pass.
func redPullRequest() *branch.PRInfo {
	return &branch.PRInfo{
		Number: 354,
		Title:  "fix(cli): name the failing check",
		URL:    "https://github.com/cidx-org/cidx/pull/354",
		Checks: &branch.PRChecksInfo{
			Total: 5, WorkflowChecks: 5, Success: 4, Failure: 1, Status: "failure",
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
		Checks: &branch.PRChecksInfo{Total: 5, WorkflowChecks: 5, Success: 5, Status: "success"},
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

// stagePRInfoSequence makes the watch loops read a different state on each
// poll, which is what "waiting for CI to start" needs: an empty check list
// first, a populated one afterwards.
func stagePRInfoSequence(t *testing.T, states ...*branch.PRInfo) {
	t.Helper()

	original := readPRInfo
	i := 0
	readPRInfo = func(*branch.Manager, string) (*branch.PRInfo, error) {
		state := states[i]
		if i < len(states)-1 {
			i++
		}
		return state, nil
	}
	t.Cleanup(func() { readPRInfo = original })
}

// noChecksYet is the state a pull request is in for the seconds between a push
// and the first check run appearing — and the permanent state of a PR opened
// with the workflow token, for which GitHub starts no workflows at all (#382).
func noChecksYet() *branch.PRInfo {
	return &branch.PRInfo{
		Number: 381,
		Title:  "chore(deps): update container images",
		URL:    "https://github.com/cidx-org/cidx/pull/381",
		Checks: &branch.PRChecksInfo{Total: 0, WorkflowChecks: 0, Status: "success"},
	}
}

// TestWatchPRChecksQuiet_AnEmptyCheckListIsNotAGreenRun covers issue #382.
//
// `cidx pr watch -q` printed "0/0 checks passed — All checks passed" and
// exited 0 seconds after a push: Complete() is true of a list with nothing in
// it, and nothing distinguished "finished" from "not started". `cpw` never had
// the problem because it waits through the provider first.
func TestWatchPRChecksQuiet_AnEmptyCheckListIsNotAGreenRun(t *testing.T) {
	original := ciStartTimeout
	ciStartTimeout = 0 // the deadline has already passed
	t.Cleanup(func() { ciStartTimeout = original })

	info := noChecksYet()
	stagePRInfo(t, info)

	var err error
	out := captureStdout(t, func() { err = watchPRChecksQuiet(nil, "chore/update-containers", info) })

	if !errors.Is(err, errNoCIStarted) {
		t.Fatalf("expected the watch to report that nothing started, got err=%v and:\n%s", err, out)
	}
	if strings.Contains(out, "All checks passed") {
		t.Errorf("an empty check list was called a green run:\n%s", out)
	}
}

// TestWatchPRChecksQuiet_WaitsForTheChecksToAppear: the same emptiness, this
// time because CI has not started *yet*. The watch has to sit through it and
// report on what arrives, not conclude on the gap.
func TestWatchPRChecksQuiet_WaitsForTheChecksToAppear(t *testing.T) {
	original := checksPollInterval
	checksPollInterval = time.Millisecond
	t.Cleanup(func() { checksPollInterval = original })

	stagePRInfoSequence(t, noChecksYet(), redPullRequest())

	var err error
	out := captureStdout(t, func() { err = watchPRChecksQuiet(nil, "fix/naming", noChecksYet()) })
	if err != nil {
		t.Fatalf("watchPRChecksQuiet: %v", err)
	}

	if !strings.Contains(out, "Some checks failed.") {
		t.Errorf("expected the verdict of the checks that did appear, got:\n%s", out)
	}
}
