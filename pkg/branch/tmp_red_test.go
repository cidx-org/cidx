package branch

import "testing"

// TestDeliberateRed exists for one CI cycle, to capture what `cidx pr status`
// and `cidx pr watch` print on a pull request with a failing check (issue
// #347). Removed in the next commit.
func TestDeliberateRed(t *testing.T) {
	t.Fatal("deliberate failure: capturing the red-check report of issue #347")
}
