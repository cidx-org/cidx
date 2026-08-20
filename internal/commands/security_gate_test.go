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
	summary := presets.CatalogueSummary{Triage: presets.Triage{Actionable: 152}}

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
	summary := presets.CatalogueSummary{Triage: presets.Triage{Actionable: 152}}

	if err := gateOn(gateContext(t, false), summary); err != nil {
		t.Fatalf("rendering the page is not a verdict: %v", err)
	}
}
