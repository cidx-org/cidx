package commands

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// maxUnknownFixDays is how long an acceptance of a finding whose fix state the
// scanners could not name may stand: the next review looks for a fix and repins.
const maxUnknownFixDays = 30

// auditFindings are the 29 findings audit run 44620523008 still counted as past
// their window: no fix in either scanner, no acceptance. Each is judged on its
// own, per CVE and per image.
var auditFindings = map[string][]string{
	"commitlint/commitlint": {
		"CVE-2026-54873", "CVE-2026-72897", "CVE-2026-84782", "CVE-2026-84784",
	},
	"dhi.io/alpine-base": {
		"CVE-2026-54873", "CVE-2026-72897", "CVE-2026-84782", "CVE-2026-84784",
	},
	"dhi.io/python": {
		"CVE-2026-54873", "CVE-2026-72897", "CVE-2026-84782", "CVE-2026-84784",
	},
	"ghcr.io/probatum-org/probatum": {
		"CVE-2026-54873", "CVE-2026-72897", "CVE-2026-84782", "CVE-2026-84784",
	},
	"golangci/golangci-lint": {
		"CVE-2026-54873", "CVE-2026-72897", "CVE-2026-84782", "CVE-2026-84784",
	},
	"zricethezav/gitleaks": {
		"CVE-2026-54873", "CVE-2026-72897", "CVE-2026-84782", "CVE-2026-84784",
		"CVE-2026-107496", "CVE-2026-88823", "CVE-2026-95691", "CVE-2026-95692", "CVE-2026-111497",
	},
}

// TestEveryAuditFindingHasADecisionOfItsOwn: the 29 findings are accepted one by
// one — one member, one decision, one reason that names the CVE — never under a
// shared reason that renews them in bulk, and each stands for a month at most.
func TestEveryAuditFindingHasADecisionOfItsOwn(t *testing.T) {
	file, err := LoadVulnerabilityFile(filepath.Join("..", "..", defaultVulnFile))
	if err != nil {
		t.Fatalf("could not read the accepted findings: %v", err)
	}
	decisions := map[string]Decision{}
	membersOf := map[string]int{}
	for _, d := range file.Decisions {
		decisions[d.ID] = d
	}
	for _, v := range file.Vulnerabilities {
		membersOf[v.Decision]++
	}

	total := 0
	reasons := map[string]string{}
	for repository, cves := range auditFindings {
		for _, cve := range cves {
			total++
			var matches []string
			for _, v := range file.Vulnerabilities {
				if v.Repository == repository && v.CVE == cve {
					matches = append(matches, v.Decision)
				}
			}
			if len(matches) != 1 || matches[0] == "" {
				t.Errorf("%s on %s: want exactly one accepted entry with a decision, got %q", cve, repository, matches)
				continue
			}
			d, ok := decisions[matches[0]]
			if !ok {
				t.Errorf("%s on %s names decision %q, which does not exist", cve, repository, matches[0])
				continue
			}
			if membersOf[d.ID] != 1 {
				t.Errorf("%s covers %d findings: an audit finding is accepted on its own, not in bulk", d.ID, membersOf[d.ID])
			}
			if d.Repository != repository {
				t.Errorf("%s is about %s, its member is on %s", d.ID, d.Repository, repository)
			}
			if d.Treatment != "accepted-risk" {
				t.Errorf("%s: treatment %q, want accepted-risk (no fix is published, so no repin exists)", d.ID, d.Treatment)
			}
			if !strings.Contains(d.Reason, cve) {
				t.Errorf("%s: the reason does not name %s", d.ID, cve)
			}
			if other, dup := reasons[d.Reason]; dup {
				t.Errorf("%s repeats the reason of %s", d.ID, other)
			}
			reasons[d.Reason] = d.ID

			reviewed, err := time.Parse(time.DateOnly, d.Reviewed)
			if err != nil {
				t.Errorf("%s: reviewed %q is not a date", d.ID, d.Reviewed)
				continue
			}
			reviewBy, err := time.Parse(time.DateOnly, d.ReviewBy)
			if err != nil {
				t.Errorf("%s: review_by %q is not a date", d.ID, d.ReviewBy)
				continue
			}
			if days := int(reviewBy.Sub(reviewed).Hours() / 24); days <= 0 || days > maxUnknownFixDays {
				t.Errorf("%s stands %d days: a finding whose fix state is unknown is re-read within %d", d.ID, days, maxUnknownFixDays)
			}
		}
	}
	if total != 29 {
		t.Fatalf("the batch lists %d findings, the audit counted 29", total)
	}
}
