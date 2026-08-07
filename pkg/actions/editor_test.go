package actions

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// editorRecorder replaces the launchEditor seam and records what was opened.
type editorRecorder struct {
	calls  [][]string // editor followed by the paths
	result error
}

func (e *editorRecorder) install(t *testing.T, isInteractive bool) {
	t.Helper()

	originalLaunch, originalInteractive := launchEditor, interactive
	launchEditor = func(editor string, paths ...string) error {
		e.calls = append(e.calls, append([]string{editor}, paths...))
		return e.result
	}
	interactive = func() bool { return isInteractive }
	t.Cleanup(func() {
		launchEditor, interactive = originalLaunch, originalInteractive
	})
}

func TestOpenInEditor_SkipsEditorWithoutTerminal(t *testing.T) {
	// Issue #186: without a terminal the editor blocks forever on a stdin that
	// never delivers a keystroke. It must not be launched at all.
	rec := &editorRecorder{}
	rec.install(t, false)
	t.Setenv("EDITOR", "vim")

	if err := openInEditor("", "/repo/.cidx/tag-version"); err != nil {
		t.Fatalf("skipping the editor must not be an error, got %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("no editor should be launched without a terminal, got %v", rec.calls)
	}
}

func TestOpenInEditor_LaunchesEditorWithTerminal(t *testing.T) {
	rec := &editorRecorder{}
	rec.install(t, true)
	t.Setenv("EDITOR", "my-editor")

	if err := openInEditor("", "/repo/.cidx/tag-version", "/repo/.cidx/tag-message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"my-editor", "/repo/.cidx/tag-version", "/repo/.cidx/tag-message"}
	if len(rec.calls) != 1 || !equalStrings(rec.calls[0], want) {
		t.Errorf("expected the editor to open both files, got %v", rec.calls)
	}
}

func TestOpenInEditor_ReportsEditorFailure(t *testing.T) {
	rec := &editorRecorder{result: errors.New("exit status 1")}
	rec.install(t, true)
	t.Setenv("EDITOR", "my-editor")

	if err := openInEditor("", "/repo/notes.md"); err == nil {
		t.Fatal("expected the editor failure to surface")
	}
}

func TestOpenInEditor_NoEditorAvailable(t *testing.T) {
	rec := &editorRecorder{}
	rec.install(t, true)
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	t.Setenv("PATH", t.TempDir()) // no vim/nano/vi/code/notepad in sight

	err := openInEditor("", "/repo/notes.md")
	if err == nil || !strings.Contains(err.Error(), "no editor found") {
		t.Fatalf("expected a 'no editor found' error, got %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("nothing should be launched, got %v", rec.calls)
	}
}

func TestResolveEditor_PrefersConfiguredOverEnvironment(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	t.Setenv("VISUAL", "emacs")

	if got := resolveEditor("nano"); got != "nano" {
		t.Errorf("configured editor must win, got %q", got)
	}
	if got := resolveEditor(""); got != "vim" {
		t.Errorf("$EDITOR must come next, got %q", got)
	}

	t.Setenv("EDITOR", "")
	if got := resolveEditor(""); got != "emacs" {
		t.Errorf("$VISUAL must come after $EDITOR, got %q", got)
	}
}

// TestTagPrepare_NonInteractiveCompletes is the issue #186 regression: the
// command must write both files and return instead of hanging on the editor.
func TestTagPrepare_NonInteractiveCompletes(t *testing.T) {
	workDir, _ := versionRepo(t, "2.1.3", "v2.1.3")
	initGitRepo(t, workDir)

	rec := &editorRecorder{}
	rec.install(t, false)

	repo, err := vcs.OpenRepository(workDir)
	if err != nil {
		t.Fatalf("could not open the temp repository: %v", err)
	}

	action := NewTagPrepare(repo, config.TagConfig{Prefix: "v"}, false)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("tag prepare failed: %v", err)
	}

	if len(rec.calls) != 0 {
		t.Errorf("the editor must stay closed without a terminal, got %v", rec.calls)
	}

	version, err := LoadPreparedTagVersion(workDir)
	if err != nil {
		t.Fatalf("%s was not written: %v", TagVersionFile, err)
	}
	if version != "2.1.4" {
		t.Errorf("expected the generated version 2.1.4 to be kept as-is, got %q", version)
	}
	if !HasTagMessage(workDir) {
		t.Errorf("%s was not written", TagMessageFile)
	}
}

// initGitRepo makes dir a real (empty) git repository so vcs.OpenRepository
// can resolve its worktree.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, output)
	}
	// Keep the temp repo out of any ambient git config surprises.
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), nil, 0644); err != nil {
		t.Logf("could not reset exclude file: %v", err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
