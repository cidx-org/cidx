package vcs

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestGitPinsTheLocale states the guarantee on any machine, including the
// English ones where the bug is invisible.
func TestGitPinsTheLocale(t *testing.T) {
	// A locale the user might well have, and the one issue #364 was found on.
	t.Setenv("LC_ALL", "fr_FR.UTF-8")

	cmd := Git(t.TempDir(), "status")

	var got string
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "LC_ALL=") {
			got = kv // the last assignment is the one that takes effect
		}
	}
	if got != "LC_ALL=C" {
		t.Errorf("LC_ALL = %q, want \"LC_ALL=C\" -- git would answer in the user's "+
			"language and every message CIDX matches on would miss (#364)", got)
	}
}

// TestGitDirIsTheWorkingDirectory: the helper took over setting Dir from the
// call sites, and it has to set it before reading the environment, since PWD is
// derived from it.
func TestGitDirIsTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	cmd := Git(dir, "status")

	if cmd.Dir != dir {
		t.Errorf("Dir = %q, want %q", cmd.Dir, dir)
	}
	var pwd string
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "PWD=") {
			pwd = strings.TrimPrefix(kv, "PWD=")
		}
	}
	if pwd != dir {
		t.Errorf("PWD = %q, want %q -- Dir has to be set before Environ reads it", pwd, dir)
	}
}

// TestGitAnswersInEnglishUnderAForeignLocale is the end of the chain: it runs
// the real git and reads what it says.
//
// It can only assert that on a machine whose git is translated, so it first
// asks this one whether it is -- a git without the French catalogue answers in
// English no matter what, and asserting English there would prove nothing while
// looking like it proved everything. GitHub runners are that machine, which is
// exactly why #364 reached main.
func TestGitAnswersInEnglishUnderAForeignLocale(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	if err := exec.Command("git", "init", "-q", dir).Run(); err != nil {
		t.Skipf("cannot init a fixture repository: %v", err)
	}

	// The same failing command, asked for in French, with nothing pinning it.
	raw := exec.Command("git", "checkout", "no-such-branch")
	raw.Dir = dir
	raw.Env = append(os.Environ(), "LC_ALL=fr_FR.UTF-8")
	french, _ := raw.CombinedOutput()

	if strings.Contains(string(french), "did not match any file") {
		t.Skipf("this git has no French catalogue, so there is no translation to pin: %s", french)
	}

	t.Setenv("LC_ALL", "fr_FR.UTF-8")
	output, err := Git(dir, "checkout", "no-such-branch").CombinedOutput()
	if err == nil {
		t.Fatalf("expected git to refuse the checkout, it printed: %s", output)
	}

	if !strings.Contains(string(output), "did not match any file") {
		t.Errorf("git answered %q under LC_ALL=fr_FR.UTF-8.\n"+
			"Untranslated it says \"pathspec ... did not match any file(s) known to git\",\n"+
			"which is the wording CIDX matches on (#364).", output)
	}
}
