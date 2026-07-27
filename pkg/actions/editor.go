package actions

import (
	"fmt"
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"
	"golang.org/x/term"
)

// interactive reports whether the process is attached to a terminal on both
// ends. Package-level so tests can drive either path.
var interactive = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// launchEditor runs the editor on the given files. Package-level seam for tests.
var launchEditor = func(editor string, paths ...string) error {
	cmd := exec.Command(editor, paths...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// openInEditor opens paths for review in the user's editor.
//
// Without a terminal there is nobody to drive the editor: vim (the usual
// fallback) keeps reading a stdin that never delivers a keystroke and paints
// escape sequences nobody sees, so the command hangs with no output at all
// (issue #186). In that case the editor is skipped and the generated files are
// kept as-is -- which is exactly what a CI run or a scripted release wants.
func openInEditor(preferred string, paths ...string) error {
	if !interactive() {
		log.Info("⏭️  No terminal detected, skipping the editor (generated content kept as-is)")
		return nil
	}

	editor := resolveEditor(preferred)
	if editor == "" {
		return fmt.Errorf("no editor found")
	}

	return launchEditor(editor, paths...)
}

// resolveEditor picks the editor: the configured one, then $EDITOR / $VISUAL,
// then the first common editor found in PATH.
func resolveEditor(preferred string) string {
	for _, candidate := range []string{preferred, os.Getenv("EDITOR"), os.Getenv("VISUAL")} {
		if candidate != "" {
			return candidate
		}
	}

	for _, candidate := range []string{"vim", "nano", "vi", "code", "notepad"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}

	return ""
}
