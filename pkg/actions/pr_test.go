package actions

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
)

func TestTitleToBranchName(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		// All conventional-commit types used in the repo
		{"feat", "feat: add auth system", "feat/add-auth-system"},
		{"fix", "fix: broken pipeline", "fix/broken-pipeline"},
		{"chore", "chore: bump dependencies", "chore/bump-dependencies"},
		{"docs", "docs: update readme", "docs/update-readme"},
		{"refactor", "refactor: split phases", "refactor/split-phases"},
		{"test", "test: add pr tests", "test/add-pr-tests"},
		{"ci", "ci: cache go modules", "ci/cache-go-modules"},
		{"perf", "perf: faster preset merge", "perf/faster-preset-merge"},
		{"build", "build: pin go version", "build/pin-go-version"},

		// Scoped title: type becomes prefix, scope stays in slug, no fix/fix- duplication
		{"scoped fix", "fix(generate): pin bootstrapped cidx", "fix/generate-pin-bootstrapped-cidx"},
		{"scoped feat", "feat(actions): add cpw command", "feat/actions-add-cpw-command"},

		// Breaking change marker
		{"breaking", "feat(api)!: drop v1 endpoints", "feat/api-drop-v1-endpoints"},
		{"breaking no scope", "fix!: remove legacy flag", "fix/remove-legacy-flag"},

		// No recognizable type: fall back to feat/
		{"no type", "Add Auth System", "feat/add-auth-system"},
		{"unknown type", "wip: something in progress", "feat/wip-something-in-progress"},

		// Case-insensitive type detection
		{"uppercase type", "Fix: Broken Thing", "fix/broken-thing"},

		// Special characters collapse into single hyphens
		{"special chars", "fix(actions): don't panic, really!", "fix/actions-don-t-panic-really"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titleToBranchName(tt.title); got != tt.want {
				t.Errorf("titleToBranchName(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestPRNextStepsSuggestCIDXCommands(t *testing.T) {
	steps := strings.Join(prNextSteps, "\n")

	// Must suggest current cidx commands (dogfooding)
	for _, want := range []string{"cidx cpw", "cidx pr ready"} {
		if !strings.Contains(steps, want) {
			t.Errorf("next steps should suggest %q, got:\n%s", want, steps)
		}
	}

	// Must not suggest deprecated aliases or raw git
	for _, forbidden := range []string{"cidx action", "git add", "git commit", "git push"} {
		if strings.Contains(steps, forbidden) {
			t.Errorf("next steps should not suggest %q, got:\n%s", forbidden, steps)
		}
	}
}

// TestPreMergeChecksSummary_NoWorkflowChecks covers #259: when the only checks
// on the commit come from another app -- GitHub's own dependabot config check,
// an external bot -- "All checks passed" claims a validation the repository's
// CI never performed.
func TestPreMergeChecksSummary_NoWorkflowChecks(t *testing.T) {
	summary := preMergeChecksSummary(&remote.PRChecks{
		TotalCount: 2, WorkflowChecks: 0, Success: 2, Status: "success",
	})

	if strings.Contains(summary, "All checks passed") {
		t.Errorf("must not claim CI passed when no workflow ran: %q", summary)
	}
	for _, want := range []string{"No workflow checks", "2 third-party"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary should mention %q, got: %q", want, summary)
		}
	}
}

// TestPreMergeChecksSummary_WorkflowChecks keeps the green path green, and says
// how many workflow checks it is talking about.
func TestPreMergeChecksSummary_WorkflowChecks(t *testing.T) {
	summary := preMergeChecksSummary(&remote.PRChecks{
		TotalCount: 4, WorkflowChecks: 3, Success: 4, Status: "success",
	})

	if !strings.Contains(summary, "All checks passed") {
		t.Errorf("expected a success line, got: %q", summary)
	}
	if !strings.Contains(summary, "3 workflow check") {
		t.Errorf("expected the workflow check count, got: %q", summary)
	}
}

// refusingProvider fails the test on any call that would reach the remote API.
// The two methods below are the only ones `pr create` can reach, so a dry run
// that touches the network stops here with the reason instead of hanging on a
// socket.
type refusingProvider struct {
	fakeProvider
	t *testing.T
}

func (p *refusingProvider) GetPullRequestByBranch(context.Context, string) (int, string, error) {
	p.t.Fatal("a dry run looked the branch's pull request up on the remote")
	return 0, "", nil
}

func (p *refusingProvider) CreatePullRequest(context.Context, string, string, string, string, bool) (int, string, error) {
	p.t.Fatal("a dry run created a pull request")
	return 0, "", nil
}

// repoOnMainWithUnreachableRemote builds a real one-commit repository on main
// whose origin is a path that does not exist. Nothing in it can reach a
// network, and any git command that tries to talk to origin fails immediately —
// which is exactly what makes it an offline probe rather than a slow one.
func repoOnMainWithUnreachableRemote(t *testing.T) (string, *vcs.Repository) {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"-c", "init.defaultBranch=main", "init", "-q", dir},
		{"-C", dir, "config", "user.email", "test@example.test"},
		{"-C", dir, "config", "user.name", "cidx test"},
		{"-C", dir, "commit", "-q", "--allow-empty", "-m", "chore: root commit"},
		{"-C", dir, "remote", "add", "origin", filepath.Join(dir, "there-is-no-remote-here.git")},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	repo, err := vcs.OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open the test repository: %v", err)
	}
	return dir, repo
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("failed to read HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// TestPRCreate_DryRunStaysOfflineAndLeavesTheRepositoryAlone is the whole
// contract of issue #276.
//
// `cidx pr create --dry-run` pulled before it reached the dry-run branch, so
// the preview needed the network and fast-forwarded the checked-out branch
// while claiming to change nothing. The repository here has a remote that does
// not exist: if anything reaches for it, the pull fails and this test fails
// with it. The provider fails the test on the calls that would query the API,
// and since #350 the resolver fails it earlier still: a preview must not so
// much as build a provider.
func TestPRCreate_DryRunStaysOfflineAndLeavesTheRepositoryAlone(t *testing.T) {
	dir, repo := repoOnMainWithUnreachableRemote(t)
	before := headSHA(t, dir)

	resolve := func() (remote.Provider, error) {
		t.Error("a dry run resolved a remote provider")
		return &refusingProvider{t: t}, nil
	}

	action := NewPR(repo, resolve, "feat: something", "", true, false)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("a dry run must not need the remote: %v", err)
	}

	if after := headSHA(t, dir); after != before {
		t.Errorf("a dry run moved the checked-out commit: %s -> %s", before, after)
	}
	if branches, _ := exec.Command("git", "-C", dir, "branch", "--list", "feat/something").Output(); len(branches) > 0 {
		t.Errorf("a dry run created the branch it was only asked to describe: %s", branches)
	}
}
