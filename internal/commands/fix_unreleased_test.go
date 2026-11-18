package commands

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// maxFixUnreleasedDays is how long a "fix-unreleased" decision may stand: a fix
// that exists upstream is a repin waiting for a release, so the wait is short and
// is re-argued, never renewed by default.
const maxFixUnreleasedDays = 30

// fixUnreleasedDecisions returns the decisions of the repository's own
// known-vulnerabilities.toml that carry the narrow "fix-unreleased" treatment,
// with the file they were read from.
func fixUnreleasedDecisions(t *testing.T) (*VulnerabilityFile, []Decision) {
	t.Helper()

	file, err := LoadVulnerabilityFile(filepath.Join("..", "..", defaultVulnFile))
	if err != nil {
		t.Fatalf("could not read the accepted findings: %v", err)
	}
	var found []Decision
	for _, d := range file.Decisions {
		if d.Treatment == "fix-unreleased" {
			found = append(found, d)
		}
	}
	return file, found
}

// TestFixUnreleasedIsANarrowException is the rule behind the category: a finding
// whose fix exists but whose tool has no image shipping it yet may be waived for
// a short, dated, named period — and only that. The general rule (a fix upstream
// never belongs in the file) is untouched; this is the one place it bends, and
// each condition below is what keeps the bend from widening.
func TestFixUnreleasedIsANarrowException(t *testing.T) {
	file, decisions := fixUnreleasedDecisions(t)
	if len(decisions) == 0 {
		t.Fatal("no fix-unreleased decision on file: the images that carry a fix with no release to repin to have no recorded exception")
	}

	images, err := catalogueImages()
	if err != nil {
		t.Fatalf("could not load the catalogue: %v", err)
	}

	for _, d := range decisions {
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
		if days := int(reviewBy.Sub(reviewed).Hours() / 24); days > maxFixUnreleasedDays {
			t.Errorf("%s stands %d days: a fix-unreleased decision is re-argued within %d", d.ID, days, maxFixUnreleasedDays)
		}

		// The finding is named, with the fix and the day it was scanned: the
		// exception says what it waits for, or it is an acceptance in disguise.
		if !strings.Contains(d.Reason, "fix") || !strings.Contains(d.Reason, d.Reviewed) {
			t.Errorf("%s: the reason must name the fix and the day it was scanned (%s)", d.ID, d.Reviewed)
		}

		// It is written against the pin the catalogue runs today. A repin moves
		// the reference, and the reason stops naming it: the decision then has to
		// be read again, which is the point.
		shipped := ""
		for image := range images {
			if imageRepository(image) == d.Repository {
				shipped, _, _ = strings.Cut(image, "@")
			}
		}
		if shipped == "" {
			t.Errorf("%s: the catalogue no longer runs %s, the decision waives nothing", d.ID, d.Repository)
		} else if !strings.Contains(d.Reason, shipped) {
			t.Errorf("%s: the reason does not name %s, the image the catalogue runs", d.ID, shipped)
		}

		members := 0
		for _, v := range file.Vulnerabilities {
			if v.Decision != d.ID {
				continue
			}
			members++
			if v.Repository != d.Repository {
				t.Errorf("%s: member %s is on %s, the decision is about %s", d.ID, v.CVE, v.Repository, d.Repository)
			}
			if !strings.Contains(d.Reason, v.CVE) {
				t.Errorf("%s: member %s is not named in the reason", d.ID, v.CVE)
			}
		}
		if members == 0 {
			t.Errorf("%s has no member: a decision that waives nothing is a stale record", d.ID)
		}
	}
}

// TestFixUnreleasedWaivesWhileItStandsAndNotAfter: the decision's capabilities
// must match what the catalogue's presets declare today, and the waiver lapses
// the day after review_by like every other decision.
func TestFixUnreleasedWaivesWhileItStandsAndNotAfter(t *testing.T) {
	file, decisions := fixUnreleasedDecisions(t)
	contexts, err := catalogueContexts(file)
	if err != nil {
		t.Fatalf("could not derive the contexts: %v", err)
	}

	for _, d := range decisions {
		reviewBy, err := time.Parse(time.DateOnly, d.ReviewBy)
		if err != nil {
			t.Fatalf("%s: review_by %q is not a date", d.ID, d.ReviewBy)
		}

		for _, tc := range []struct {
			day  time.Time
			want bool
		}{
			{reviewBy, true},
			{reviewBy.AddDate(0, 0, 1), false},
		} {
			for i, verdict := range ResolveWaivers(file, contexts, tc.day) {
				if file.Vulnerabilities[i].Decision != d.ID {
					continue
				}
				if verdict.Waived != tc.want {
					t.Errorf("%s on %s: %s waived = %v (%s), want %v",
						d.ID, tc.day.Format(time.DateOnly), verdict.CVE, verdict.Waived, verdict.Reason, tc.want)
				}
			}
		}
	}
}
