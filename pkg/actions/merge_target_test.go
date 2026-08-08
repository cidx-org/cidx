package actions

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// mergeTargetProvider reports a fixed head SHA and nothing else.
type mergeTargetProvider struct {
	remote.Provider
	headSHA string
	err     error
}

func (p *mergeTargetProvider) GetPullRequestChecks(context.Context, int) (*remote.PRChecks, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &remote.PRChecks{HeadSHA: p.headSHA}, nil
}

// repoOnOneCommit is a real repository with a single commit, so GetHeadSHA
// answers what git would answer.
func repoOnOneCommit(t *testing.T) (*vcs.Repository, string) {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "guard@example.test"},
		{"config", "user.name", "guard"},
		{"commit", "--allow-empty", "-m", "root"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable or refusing to init here (%v): %s", err, out)
		}
	}

	repo, err := vcs.OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open the temporary repository: %v", err)
	}

	sha, err := repo.GetHeadSHA()
	if err != nil {
		t.Fatalf("failed to read HEAD of the temporary repository: %v", err)
	}

	return repo, sha
}

// TestCheckMergeTarget_RefusesACommitTheRemoteHasNotSeen is the behaviour behind
// features/cli/merge_target.feature, wired to the two things the merge actually
// reads: git's HEAD, and the provider's head SHA.
func TestCheckMergeTarget_RefusesACommitTheRemoteHasNotSeen(t *testing.T) {
	repo, localSHA := repoOnOneCommit(t)
	action := &PRAction{repo: repo, provider: &mergeTargetProvider{headSHA: "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"}}

	err := action.checkMergeTarget(context.Background(), 414)
	if err == nil {
		t.Fatal("a merge whose pull request sits on a different commit returned nil — the branch is " +
			"deleted afterwards with `git branch -D`, so the local commit would go with it")
	}
	if !strings.Contains(err.Error(), localSHA[:7]) {
		t.Errorf("the refusal must name the commit in hand (%s), got: %v", localSHA[:7], err)
	}
}

func TestCheckMergeTarget_ProceedsOnTheCommitInHand(t *testing.T) {
	repo, localSHA := repoOnOneCommit(t)
	action := &PRAction{repo: repo, provider: &mergeTargetProvider{headSHA: localSHA}}

	if err := action.checkMergeTarget(context.Background(), 414); err != nil {
		t.Fatalf("the pull request is on the commit in hand, so the merge must proceed: %v", err)
	}
}

// A provider that cannot answer must not block the merge -- replacing a rare
// accident with a permanent one.
func TestCheckMergeTarget_AProviderThatCannotAnswerDoesNotBlock(t *testing.T) {
	repo, _ := repoOnOneCommit(t)
	action := &PRAction{repo: repo, provider: &mergeTargetProvider{err: context.DeadlineExceeded}}

	if err := action.checkMergeTarget(context.Background(), 414); err != nil {
		t.Fatalf("an unreadable head SHA must not stop a merge: %v", err)
	}
}

// TestMergePRConsultsTheGuard fails when the guard is written but not called.
//
// Removing the call from mergePR breaks nothing else: every other test in this
// package still passes, the merge still works, and the protection is simply
// absent. That is the same silent degradation TestGetPRInfoCarriesTheHeadSHA
// exists for on the watch side -- a guard that stops being consulted reports
// nothing at all, which is indistinguishable from a guard that found nothing.
//
// Driving mergePR itself would need a repository, a pull request and a merge to
// really happen, so the call is asserted where it is visible: in the AST.
func TestMergePRConsultsTheGuard(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "pr.go", nil, 0)
	if err != nil {
		t.Fatalf("failed to parse pr.go: %v", err)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "mergePR" {
			continue
		}

		if callsMethod(fn, "checkMergeTarget") {
			return
		}

		t.Fatalf("mergePR never calls checkMergeTarget — nothing else fails when that call goes, "+
			"and a merge then lands whatever the remote holds while `git branch -D` deletes the rest (%s)",
			fset.Position(fn.Pos()))
	}

	t.Fatal("no mergePR found in pr.go — this test would pass by watching nothing")
}

// callsMethod reports whether fn contains a call to a method of the receiver.
func callsMethod(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}
