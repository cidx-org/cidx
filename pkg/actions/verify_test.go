package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

// stubVerification replaces the one function in the pre-push path that starts
// containers, and reports whether it was reached. Nothing here runs Docker or
// touches the network -- these tests are about the decision, not the phase.
func stubVerification(t *testing.T, result error) *bool {
	t.Helper()

	called := false
	original := verifyBeforePush
	verifyBeforePush = func(context.Context) error {
		called = true
		return result
	}
	t.Cleanup(func() { verifyBeforePush = original })

	return &called
}

// captureLogs collects what cpw told the user, so the tests can assert that a
// skipped check is announced rather than silent.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	return &buf
}

func TestPlanVerification_RunsByDefault(t *testing.T) {
	// The default is the whole point of #307: cpw pushed without ever running
	// the checks CI was about to run, so a prettier reflow cost a full cycle.
	run, _ := planVerification(true, false)
	if !run {
		t.Error("cpw must run the code phase by default -- opt-in would leave #307 exactly as it was")
	}
}

func TestPlanVerification_NoVerifyIsTheEscapeHatch(t *testing.T) {
	// Same escape hatch git gives its hooks, spelled the same way.
	run, reason := planVerification(false, false)
	if run {
		t.Error("--no-verify must skip the code phase")
	}
	if !strings.Contains(reason, "--no-verify") {
		t.Errorf("the skip must name the flag that caused it, got %q", reason)
	}
}

func TestPlanVerification_PreCommitHookIsNotRunTwice(t *testing.T) {
	// Commit() shells out to git so hooks fire, and this repository's hook runs
	// the same phase. Running it here as well is ~20s spent re-deriving an
	// answer the commit is about to derive anyway.
	run, reason := planVerification(true, true)
	if run {
		t.Error("the code phase must not run twice when a pre-commit hook already runs it")
	}
	if !strings.Contains(reason, "pre-commit hook") {
		t.Errorf("the skip must say the hook is what covers it, got %q", reason)
	}
}

func TestPlanVerification_NoVerifyWinsOverTheHook(t *testing.T) {
	// --no-verify reaches the hook too: Commit() passes it nothing, but git's
	// own --no-verify is what the flag is named after and the user asked for
	// no checks. What matters here is that cpw does not run the phase itself.
	if run, _ := planVerification(false, true); run {
		t.Error("--no-verify must skip the code phase whatever the hook situation is")
	}
}

func TestVerificationOutcome_PassLetsThePushThrough(t *testing.T) {
	if err := verificationOutcome(nil); err != nil {
		t.Fatalf("a passing code phase must not stop the push, got: %v", err)
	}
}

func TestVerificationOutcome_FailureStopsBeforeTheCommit(t *testing.T) {
	// The failure has to reach the caller: Execute runs this before Commit, so
	// returning nil here would commit and push the very slip the phase caught.
	err := verificationOutcome(fmt.Errorf("phase code failed: prettier"))
	if err == nil {
		t.Fatal("a failing code phase must stop cpw before it commits")
	}
	if !strings.Contains(err.Error(), "--no-verify") {
		t.Errorf("the failure must name the way out, got: %v", err)
	}
	if !strings.Contains(err.Error(), "prettier") {
		t.Errorf("the failure must carry what actually failed, got: %v", err)
	}
}

func TestVerificationOutcome_NoContainerRuntimeDoesNotBlockThePush(t *testing.T) {
	// A machine with no Docker daemon must still be able to push. `cidx
	// doctor` reads a Podman CLI without its socket as no usable runtime
	// (#190); this takes the same reading and steps over it rather than
	// standing between someone and their remote.
	buf := captureLogs(t)

	if err := verificationOutcome(errNoContainerRuntime); err != nil {
		t.Fatalf("a missing container runtime must not block the push, got: %v", err)
	}
	if !strings.Contains(buf.String(), "No container runtime") {
		t.Errorf("the skip must be said out loud, or the push looks verified when it is not: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "doctor") {
		t.Errorf("the warning must point at the command that explains it, got: %s", buf.String())
	}
}

func TestVerificationOutcome_NoCodePhaseDoesNotBlockThePush(t *testing.T) {
	// cpw is a workflow command usable in a repository with no cidx.toml at
	// all, or one that declares no code phase. Nothing to run is not a failure.
	buf := captureLogs(t)

	if err := verificationOutcome(errNoCodePhase); err != nil {
		t.Fatalf("no code phase configured must not block the push, got: %v", err)
	}
	if !strings.Contains(buf.String(), "No code phase") {
		t.Errorf("the skip must be said out loud, got: %s", buf.String())
	}
}

func TestVerificationOutcome_SentinelsAreRecognisedWhenWrapped(t *testing.T) {
	// runCodePhase returns the sentinels bare today. Wrapping one later must
	// not silently turn a graceful skip into a blocked push.
	for _, sentinel := range []error{errNoCodePhase, errNoContainerRuntime} {
		if err := verificationOutcome(fmt.Errorf("verify: %w", sentinel)); err != nil {
			t.Errorf("wrapped %v must still be recognised as a skip, got: %v", sentinel, err)
		}
	}
}

func TestRunVerification_NoVerifyNeverStartsThePhase(t *testing.T) {
	// The flag has to short-circuit before the phase is invoked -- skipping the
	// *result* while still paying the 20 seconds would be no escape hatch at
	// all. a.repo stays nil here, which is also how this asserts the
	// short-circuit happens before the hook is probed.
	called := stubVerification(t, errors.New("the phase must not have run"))
	buf := captureLogs(t)

	action := &CommitPushWatchAction{verify: false}
	if err := action.runVerification(context.Background()); err != nil {
		t.Fatalf("--no-verify must not fail, got: %v", err)
	}
	if *called {
		t.Error("--no-verify still ran the code phase")
	}
	if !strings.Contains(buf.String(), "--no-verify") {
		t.Errorf("a skipped check must be announced, got: %s", buf.String())
	}
}
