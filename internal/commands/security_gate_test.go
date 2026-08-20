package commands

import (
	"flag"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/urfave/cli/v2"
)

// The audit gate, issue #436.
//
// It used to fail on every unaccepted HIGH/CRITICAL, which included the ones a
// fix already exists for — and the policy forbids excepting those, so the only
// route to green was the one thing it refuses. The audit stayed red for weeks
// and stopped meaning anything.
//
// gateOn asks the page's own question instead, which is the question the
// Security tab already publishes.

func gateContext(t *testing.T, fail bool) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("summary", flag.ContinueOnError)
	set.Bool("fail-if-waiting", fail, "")
	return cli.NewContext(nil, set, nil)
}

func TestGateOn_FailsOnFindingsThatNeedAJudgement(t *testing.T) {
	summary := presets.CatalogueSummary{Triage: presets.Triage{Actionable: 152}, Unanswered: 152}

	err := gateOn(gateContext(t, true), summary)
	if err == nil {
		t.Fatal("152 findings with no fix at any version is exactly what a human has to answer, " +
			"and the gate let the run pass")
	}
	if !strings.Contains(err.Error(), "152") {
		t.Errorf("the verdict has to carry the number the Security tab shows, got: %v", err)
	}
}

// The case the old gate got wrong: findings a fix exists for are the monitor's
// job, not a decision. Carried is high, Actionable is zero, and the audit is
// green — otherwise it is red forever and nobody reads it.
func TestGateOn_IgnoresFindingsAFixAlreadyExistsFor(t *testing.T) {
	summary := presets.CatalogueSummary{Triage: presets.Triage{Carried: 637, Fixable: 427}}

	if err := gateOn(gateContext(t, true), summary); err != nil {
		t.Fatalf("a catalogue whose findings are all waiting on a repin needs no judgement: %v", err)
	}
}

// Without the flag the command is a renderer, and a page that exits non-zero
// wherever it is run is unusable locally.
func TestGateOn_IsSilentUnlessAsked(t *testing.T) {
	summary := presets.CatalogueSummary{Unanswered: 152}

	if err := gateOn(gateContext(t, false), summary); err != nil {
		t.Fatalf("rendering the page is not a verdict: %v", err)
	}
}

// An accepted finding is one somebody has already argued, with a date on it. It
// stays in Carried — the baseline reports what an image ships — and it must
// leave the queue, or writing the argument changes nothing and the gate can
// never be reached by doing the work (#439).
//
// Nobody hit this before: every exception on file was a kernel-header one, an
// exempt class that was never Actionable, so acceptance and actionability had
// not once overlapped.
func TestUnaccepted_DropsWhatAnExceptionAlreadyAnswers(t *testing.T) {
	carried := map[string][]presets.Finding{
		"rust:1.97.1-slim": {
			{ID: "CVE-2026-1111", Severity: "HIGH"},
			{ID: "CVE-2026-2222", Severity: "HIGH"},
		},
	}
	accepted := []Vulnerability{{CVE: "cve-2026-1111", Repository: "rust"}}

	left := unaccepted(carried, accepted)["rust:1.97.1-slim"]
	if len(left) != 1 || left[0].ID != "CVE-2026-2222" {
		t.Fatalf("the argued finding should have left the queue and the other stayed, got %v", left)
	}
}

// The match is per repository: the same CVE argued on one image says nothing
// about another that also carries it.
func TestUnaccepted_IsPerRepository(t *testing.T) {
	carried := map[string][]presets.Finding{
		"rust:1.97.1-slim":  {{ID: "CVE-2026-1111"}},
		"pyfound/black:1.0": {{ID: "CVE-2026-1111"}},
	}
	accepted := []Vulnerability{{CVE: "CVE-2026-1111", Repository: "rust"}}

	left := unaccepted(carried, accepted)
	if len(left["rust:1.97.1-slim"]) != 0 {
		t.Error("argued on rust, so it must leave rust's queue")
	}
	if len(left["pyfound/black:1.0"]) != 1 {
		t.Error("nobody argued it on black -- collapsing the two would accept an image nobody looked at")
	}
}

// And the gate reads that number, not the classification.
func TestGateOn_GoesGreenOnceEverythingIsArgued(t *testing.T) {
	summary := presets.CatalogueSummary{
		Triage:     presets.Triage{Carried: 637, Actionable: 152},
		Unanswered: 0,
	}

	if err := gateOn(gateContext(t, true), summary); err != nil {
		t.Fatalf("every finding is argued, so nothing is waiting on a human: %v", err)
	}
}
