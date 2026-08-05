package branch

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/remote"
)

// TestFailedChecks_MirrorsTheProviderCount covers #347: the names printed under
// "4/5 passed" have to be the four-versus-five the provider counted, or the
// block contradicts itself. skipped and neutral count as passed, a completed
// run with any other conclusion does not, and a run still going is neither.
func TestFailedChecks_MirrorsTheProviderCount(t *testing.T) {
	checks := &remote.PRChecks{
		Checks: []remote.CheckRun{
			{Name: "Bootstrap", Status: "completed", Conclusion: "success"},
			{Name: "Build", Status: "completed", Conclusion: "skipped"},
			{Name: "Lint", Status: "completed", Conclusion: "neutral"},
			{Name: "Docs", Status: "in_progress"},
			{Name: "Test", Status: "completed", Conclusion: "failure", FailedStep: "Run tests"},
			{Name: "E2E", Status: "completed", Conclusion: "cancelled"},
		},
		StatusChecks: []remote.StatusCheck{
			{Context: "codecov", State: "success"},
			{Context: "netlify", State: "pending"},
			{Context: "legacy-ci", State: "error"},
		},
	}

	got := failedChecks(checks)

	want := []string{"Test", "E2E", "legacy-ci"}
	if len(got) != len(want) {
		t.Fatalf("expected %d failing checks, got %d: %+v", len(want), len(got), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("failing check %d = %q, want %q", i, got[i].Name, name)
		}
	}
	if got[0].Step != "Run tests" {
		t.Errorf("expected the failing step to be carried over, got %q", got[0].Step)
	}
}

// TestFailedChecks_GreenPullRequestNamesNothing: nothing is added under a count
// that has nothing behind it.
func TestFailedChecks_GreenPullRequestNamesNothing(t *testing.T) {
	checks := &remote.PRChecks{
		Checks: []remote.CheckRun{{Name: "Test", Status: "completed", Conclusion: "success"}},
	}

	if got := failedChecks(checks); len(got) != 0 {
		t.Errorf("expected no failing check, got %+v", got)
	}
}

func TestFormatFailedChecks_NamesCheckStepAndExcerpt(t *testing.T) {
	out := FormatFailedChecks([]FailedCheck{
		{Name: "Test", Step: "Run tests", Log: "Process completed with exit code 1.\nmore noise"},
		{Name: "codecov"},
	}, "  ")

	for _, want := range []string{
		"  ✗ Test — failed step: Run tests\n",
		"    Process completed with exit code 1.\n",
		"  ✗ codecov\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}

	// A check the provider says nothing more about gets exactly one line.
	if strings.Contains(out, "✗ codecov —") {
		t.Errorf("an empty step should not be rendered:\n%s", out)
	}
}

func TestFormatFailedChecks_NothingToSayPrintsNothing(t *testing.T) {
	if out := FormatFailedChecks(nil, "  "); out != "" {
		t.Errorf("expected no output, got %q", out)
	}
}

// TestFormatPRInfo_NamesTheFailingCheckUnderTheCount is the regression for
// #347: `cidx pr status` stopped at "✗ 4/5 passed", which named neither the
// check nor the reason, so the next step was always the web UI.
func TestFormatPRInfo_NamesTheFailingCheckUnderTheCount(t *testing.T) {
	out := FormatPRInfo(&PRInfo{
		Number: 354,
		Title:  "fix(cli): name the failing check",
		Status: PRStatusOpen,
		Checks: &PRChecksInfo{
			Total: 5, Success: 4, Failure: 1, Status: "failure",
			Failed: []FailedCheck{{Name: "Test", Step: "Run tests"}},
		},
	})

	for _, want := range []string{"4/5 passed", "(1 failed)", "✗ Test — failed step: Run tests"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}
}

// TestFormatPRInfo_GreenChecksStayOneLine: the block a passing PR shows is
// unchanged -- no empty "failed" section under a green count.
func TestFormatPRInfo_GreenChecksStayOneLine(t *testing.T) {
	out := FormatPRInfo(&PRInfo{
		Number:    354,
		Status:    PRStatusOpen,
		Mergeable: true,
		Checks:    &PRChecksInfo{Total: 5, Success: 5, Status: "success"},
	})

	if strings.Contains(out, "✗") {
		t.Errorf("a green pull request should carry no failure marker:\n%s", out)
	}
}
