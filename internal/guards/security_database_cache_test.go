package guards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecurityAuditDatabasesAreFreshPerRun keeps the scanner cache as a
// fan-out mechanism, not a cross-run source of vulnerability data. A restore
// prefix once allowed Monday's audit to scan with whichever older database
// GitHub still had when the exact daily key was absent.
func TestSecurityAuditDatabasesAreFreshPerRun(t *testing.T) {
	path := filepath.Join(projectRoot, ".github", "workflows", "security-audit.yml")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read security audit workflow: %v", err)
	}
	workflow := string(source)

	for _, want := range []string{
		"trivy-db-${{ github.run_id }}",
		"grype-db-${{ github.run_id }}",
		"--method DELETE",
		"/repos/${{ github.repository }}/actions/caches/$CACHE_ID",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("security audit must contain %q so every run starts from fresh databases", want)
		}
	}

	if strings.Contains(workflow, "restore-keys: trivy-db-") || strings.Contains(workflow, "restore-keys: grype-db-") {
		t.Error("security audit must not restore a vulnerability database from an older run")
	}
}
