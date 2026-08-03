package branch

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The branch set of the incident behind issue #269: a repository whose merged
// branches have piled up, one of which is where the user is standing.
func incidentBranches() []Info {
	return []Info{
		{Name: "main", Location: LocationBoth, Status: StatusProtected, IsProtected: true},
		{Name: "feat/one", Location: LocationLocal, Status: StatusMerged},
		{Name: "feat/two", Location: LocationLocal, Status: StatusMerged},
		{Name: "feat/three", Location: LocationLocal, Status: StatusMerged},
	}
}

func names(branches []Info) []string {
	out := make([]string, 0, len(branches))
	for _, b := range branches {
		out = append(out, b.Name)
	}
	return out
}

func TestSelectForCleanupDefaultsToTheCurrentBranchAlone(t *testing.T) {
	// Standing on a merged branch with two more merged ones next to it: the
	// run must not reach past the branch the user is on.
	selected, skipped, err := SelectForCleanup(incidentBranches(), CleanupOptions{}, "feat/one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("the checked-out branch cannot be deleted, got %v", names(selected))
	}
	if len(skipped) != 1 || skipped[0].Name != "feat/one" {
		t.Fatalf("expected feat/one skipped, got %+v", skipped)
	}
	if !strings.Contains(skipped[0].Reason, "current branch") {
		t.Errorf("reason must say why: %q", skipped[0].Reason)
	}
}

func TestSelectForCleanupOnMainDeletesNothing(t *testing.T) {
	selected, skipped, err := SelectForCleanup(incidentBranches(), CleanupOptions{}, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("main is protected, got %v", names(selected))
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "protected") {
		t.Fatalf("expected main skipped as protected, got %+v", skipped)
	}
	// Even --force must not take main out.
	selected, _, err = SelectForCleanup(incidentBranches(), CleanupOptions{Force: true}, "main")
	if err != nil || len(selected) != 0 {
		t.Fatalf("--force must not delete a protected branch, got %v (err %v)", names(selected), err)
	}
}

func TestSelectForCleanupBranchFlagTakesOneBranch(t *testing.T) {
	opts := CleanupOptions{Branch: "feat/two"}
	selected, skipped, err := SelectForCleanup(incidentBranches(), opts, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 || selected[0].Name != "feat/two" {
		t.Fatalf("expected feat/two alone, got %v", names(selected))
	}
	if len(skipped) != 0 {
		t.Fatalf("nothing else should be reported, got %+v", skipped)
	}
}

func TestSelectForCleanupUnknownBranchIsAnError(t *testing.T) {
	_, _, err := SelectForCleanup(incidentBranches(), CleanupOptions{Branch: "feat/nope"}, "main")
	if err == nil {
		t.Fatal("expected an error naming the missing branch")
	}
	if !strings.Contains(err.Error(), "feat/nope") {
		t.Errorf("error must name the branch: %v", err)
	}
}

func TestSelectForCleanupRefusesABranchWithAnOpenPR(t *testing.T) {
	branches := []Info{{
		Name: "feat/wip", Location: LocationBoth, Status: StatusActive,
		PRNumber: 42, PRStatus: PRStatusOpen,
	}}

	selected, skipped, err := SelectForCleanup(branches, CleanupOptions{Branch: "feat/wip"}, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("a branch with an open PR must not go, got %v", names(selected))
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "#42") {
		t.Fatalf("the refusal must name the open PR, got %+v", skipped)
	}
	if !strings.Contains(skipped[0].Reason, "--force") {
		t.Errorf("the refusal must name the way out: %q", skipped[0].Reason)
	}

	// --force is that way out.
	selected, _, err = SelectForCleanup(branches, CleanupOptions{Branch: "feat/wip", Force: true}, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("--force must delete it anyway, got %v", names(selected))
	}
}

func TestSelectForCleanupAcceptsBranchesTheRepositoryIsFinishedWith(t *testing.T) {
	cases := []struct {
		name      string
		info      Info
		deletable bool
	}{
		{"merged into main", Info{Name: "b", Status: StatusMerged}, true},
		{"PR closed without merge", Info{Name: "b", Status: StatusOrphan, PRNumber: 7, PRStatus: PRStatusClosed}, true},
		{"PR merged", Info{Name: "b", Status: StatusMerged, PRNumber: 7, PRStatus: PRStatusMerged}, true},
		{"active, no PR", Info{Name: "b", Status: StatusActive}, false},
		{"stale, no PR", Info{Name: "b", Status: StatusStale}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selected, skipped, err := SelectForCleanup([]Info{tc.info}, CleanupOptions{Branch: "b"}, "main")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(selected) == 1; got != tc.deletable {
				t.Fatalf("deletable = %v, want %v (skipped: %+v)", got, tc.deletable, skipped)
			}
		})
	}
}

func TestSelectForCleanupAllStillSweeps(t *testing.T) {
	selected, skipped, err := SelectForCleanup(incidentBranches(), CleanupOptions{All: true}, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 3 {
		t.Fatalf("--all keeps the sweep, got %v", names(selected))
	}
	if len(skipped) != 0 {
		t.Fatalf("protected branches are not swept candidates, got %+v", skipped)
	}
}

func TestSelectForCleanupAllLeavesTheCurrentBranchAlone(t *testing.T) {
	selected, skipped, err := SelectForCleanup(incidentBranches(), CleanupOptions{All: true}, "feat/one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected the two other merged branches, got %v", names(selected))
	}
	if len(skipped) != 1 || skipped[0].Name != "feat/one" {
		t.Fatalf("expected feat/one skipped, got %+v", skipped)
	}
}

func TestSelectForCleanupAllTakesStaleAndOrphanOnlyWhenAsked(t *testing.T) {
	branches := []Info{
		{Name: "old", Status: StatusStale},
		{Name: "abandoned", Status: StatusOrphan},
	}

	selected, _, _ := SelectForCleanup(branches, CleanupOptions{All: true}, "main")
	if len(selected) != 0 {
		t.Fatalf("neither is swept by default, got %v", names(selected))
	}

	selected, _, _ = SelectForCleanup(branches, CleanupOptions{All: true, IncludeStale: true}, "main")
	if len(selected) != 1 || selected[0].Name != "old" {
		t.Fatalf("--stale sweeps the stale one, got %v", names(selected))
	}

	selected, _, _ = SelectForCleanup(branches, CleanupOptions{All: true, IncludeOrphan: true}, "main")
	if len(selected) != 1 || selected[0].Name != "abandoned" {
		t.Fatalf("--orphan sweeps the orphan one, got %v", names(selected))
	}
}

// TestCleanupDeletesOnlyTheBranchItWasGiven runs the real thing -- Manager.Cleanup,
// real git, real deletions -- inside a throwaway repository, which is the only
// place a test may delete a branch.
func TestCleanupDeletesOnlyTheBranchItWasGiven(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")
	if err := os.WriteFile(dir+"/f", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "f")
	git("commit", "-m", "root")
	for _, b := range []string{"feat/one", "feat/two", "feat/three"} {
		git("branch", b) // merged into main: same commit
	}

	t.Chdir(dir)

	// No remote here, so no GitHub client and no network: the statuses come
	// from git alone, which is what the incident had too.
	manager := NewManager(Config{Protected: []string{"main"}})
	result, err := manager.Cleanup(CleanupOptions{Branch: "feat/two", DryRun: true})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if result.TotalDeleted != 1 || result.Deleted[0].Name != "feat/two" {
		t.Fatalf("dry run should name one branch, got %+v", result.Deleted)
	}
	if remaining := localBranches(t, dir); len(remaining) != 4 {
		t.Fatalf("a dry run deletes nothing, got %v", remaining)
	}

	result, err = manager.Cleanup(CleanupOptions{Branch: "feat/two", DryRun: false})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if result.TotalDeleted != 1 {
		t.Fatalf("expected one deletion, got %+v", result.Deleted)
	}

	remaining := strings.Join(localBranches(t, dir), " ")
	if want := "feat/one feat/three main"; remaining != want {
		t.Fatalf("remaining branches = %q, want %q", remaining, want)
	}
}

func localBranches(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing branches: %v", err)
	}
	return strings.Fields(string(out))
}
