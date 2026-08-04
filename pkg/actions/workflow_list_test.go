package actions

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// listedRuns is one page of what the API returns, with the two identifiers
// deliberately far apart: a run number that would look like a plausible ID is
// exactly how #291 went unnoticed.
func listedRuns() []WorkflowRun {
	return []WorkflowRun{
		{
			ID: 18234567890, RunNumber: 640,
			HeadBranch: "main", HeadSha: "9b577f5abcdef", Status: "completed",
			Conclusion: "success", CreatedAt: time.Date(2026, 7, 30, 9, 42, 0, 0, time.UTC),
			DisplayTitle: "fix(cli): say what was checked",
		},
		{
			ID: 18234500000, RunNumber: 639,
			HeadBranch: "main", HeadSha: "39ea54a123456", Status: "in_progress",
			CreatedAt: time.Date(2026, 7, 30, 9, 36, 0, 0, time.UTC),
		},
	}
}

// TestWorkflowListPrintsTheIdentifierWatchAccepts covers #291: `list` printed
// `#640`, which is the number GitHub shows in its UI and not the identifier the
// API addresses a run by, so `cidx repo workflow watch 640` — the obvious next
// command — answered 404. Both are printed now, labelled, so the pair works
// without translating anything in your head.
func TestWorkflowListPrintsTheIdentifierWatchAccepts(t *testing.T) {
	action := NewWorkflowList("ci", 10, false)

	simple := captureStdout(t, func() { action.displaySimple(listedRuns()) })
	verbose := captureStdout(t, func() {
		NewWorkflowList("ci", 10, true).displayVerbose(listedRuns())
	})

	for name, out := range map[string]string{"simple": simple, "verbose": verbose} {
		for _, run := range listedRuns() {
			id := strconv.FormatInt(run.ID, 10)
			if !strings.Contains(out, id) {
				t.Errorf("%s listing omits run ID %s, the only value watch accepts:\n%s", name, id, out)
			}
			// The UI number stays: it is what the GitHub web page shows, and
			// dropping it would only move the confusion.
			if !strings.Contains(out, "#"+strconv.Itoa(run.RunNumber)) {
				t.Errorf("%s listing dropped run number #%d:\n%s", name, run.RunNumber, out)
			}
		}

		// And it says which of the two to hand to the neighbouring command.
		if !strings.Contains(out, "cidx repo workflow watch") {
			t.Errorf("%s listing does not name the command that takes the id:\n%s", name, out)
		}
		if strings.Contains(out, "gh run view") {
			t.Errorf("%s listing still sends the reader to gh with the wrong number:\n%s", name, out)
		}
	}
}

// TestWorkflowListIDIsNotTruncated: a run ID is a 64-bit integer that already
// runs to eleven digits, and a padded column that clipped it would hand the
// reader a number the API does not know — the failure #291 is about, restored by
// formatting.
func TestWorkflowListIDIsNotTruncated(t *testing.T) {
	run := WorkflowRun{ID: 9223372036854775807, RunNumber: 1, CreatedAt: time.Now()}

	out := captureStdout(t, func() {
		NewWorkflowList("ci", 10, true).displayVerbose([]WorkflowRun{run})
	})
	if !strings.Contains(out, "9223372036854775807") {
		t.Errorf("the run ID was clipped by its column:\n%s", out)
	}
}

// captureStdout runs fn with os.Stdout redirected, so a listing that prints
// rather than returns can still be asserted on.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = write

	done := make(chan string)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := read.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()

	fn()
	_ = write.Close()
	os.Stdout = original
	return <-done
}
