package guards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryAuditSummaryCarriesTheSameWindow keeps the status page and the gate
// answering the same question (#436, #524). Both run `cidx security summary`;
// the gate fails on the findings past their window, and the page names how many
// there are. If one call dropped the flags the page would list a different
// population from the one the audit just failed on, and a red run would name
// nothing — which is how a gate starts being ignored.
func TestEveryAuditSummaryCarriesTheSameWindow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(projectRoot, ".github", "workflows", "security-audit.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// Join the backslash continuations so a call split over several lines is one.
	lines := strings.Split(strings.ReplaceAll(string(raw), "\\\n", " "), "\n")

	found := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") || !strings.Contains(line, "bin/cidx security summary") {
			continue
		}
		found++
		for _, flag := range []string{"--grace-days", "--first-seen"} {
			if !strings.Contains(line, flag) {
				t.Errorf("security-audit.yml runs `cidx security summary` without %s:\n  %s\nthe page and the gate must be judged by the same window", flag, strings.TrimSpace(line))
			}
		}
	}
	if found < 2 {
		t.Fatalf("expected the page and the gate to run `cidx security summary`, found %d call(s): the guard compares nothing", found)
	}
}
