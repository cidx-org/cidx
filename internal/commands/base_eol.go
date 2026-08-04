package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// Reading the base of every catalogue image out of the scans that already ran,
// and asking endoflife.date how long it is still supported (#303).
//
// The whole network cost is one request per distinct OS family — three, for a
// catalogue of twenty-two images — and none of it is on the path of anything
// that matters: a failure is reported as `unchecked` and nothing downstream
// changes its verdict because of it.

// endoflifeAPI is where the release cycles are read from. It is a variable so a
// test can point it at an httptest server, exactly as `registryScheme` is; no
// test reaches the real endoflife.date.
var endoflifeAPI = "https://endoflife.date/api"

// eolTimeout bounds the whole exchange. A hung third-party API must not hold a
// security report open: the answer to "endoflife.date is not responding" is the
// same as the answer to "endoflife.date said no", and both take at most this
// long to reach.
const eolTimeout = 10 * time.Second

// eolCyclesFunc is the seam every test substitutes, the same convention as
// latestTagFunc and resolveDigestFunc. Nothing in the test suite performs a
// request to endoflife.date.
var eolCyclesFunc = fetchEOLCycles

// fetchEOLCycles reads one product's release lines from endoflife.date.
//
// Every failure is an ordinary error: unreachable host, a status that is not
// 200 (an unmapped product answers 404 with an HTML page), or a body that is
// not a cycle list. The caller turns all of them into `unchecked`.
func fetchEOLCycles(product string) ([]presets.EOLCycle, error) {
	client := &http.Client{Timeout: eolTimeout}

	resp, err := client.Get(fmt.Sprintf("%s/%s.json", endoflifeAPI, product))
	if err != nil {
		return nil, fmt.Errorf("endoflife.date could not be reached: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endoflife.date answered %s for %q", resp.Status, product)
	}

	// Bounded, because the answer is a small JSON array and an unbounded read
	// of a third-party body is a resource the caller never agreed to spend.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("endoflife.date response could not be read: %w", err)
	}

	var cycles []presets.EOLCycle
	if err := json.Unmarshal(body, &cycles); err != nil {
		return nil, fmt.Errorf("endoflife.date returned no cycle list for %q: %w", product, err)
	}
	return cycles, nil
}

// catalogueBases reads the base of every catalogue image out of the Trivy
// results the audit already produced.
//
// An image with no readable Trivy result is simply absent from the map, which
// the report prints as `not scanned` — the same distinction the carried counts
// make, and for the same reason: an unknown base is not the absence of one.
func catalogueBases(imagePresets map[string][]string, resultsDir string) map[string]presets.BaseOS {
	bases := make(map[string]presets.BaseOS)
	for image := range imagePresets {
		base, err := scanBaseOS(resultsDir, image)
		if err != nil {
			continue
		}
		bases[image] = base
	}
	return bases
}

// scanBaseOS reads `Metadata.OS` out of one image's Trivy result.
//
// Trivy only: Grype reports a `distro` too, but the two disagree on spelling
// for exactly the families that matter here (Grype says `alpine`/`debian` with
// its own version rounding), and a second source would have to be reconciled
// with the first for no gain. An image only Grype scanned has no base here, and
// says so.
//
// An image with no `Metadata.OS` at all is not an error: a scratch or static
// image genuinely has no distribution underneath it, and the zero BaseOS is
// what ClassifyBase reads as BaseNone.
func scanBaseOS(dir, image string) (presets.BaseOS, error) {
	data, err := readFirstResult(dir, scanResultFiles("trivy", image))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return presets.BaseOS{}, errNoScanResults
		}
		return presets.BaseOS{}, fmt.Errorf("trivy result could not be read: %w", err)
	}

	var report struct {
		Metadata struct {
			OS struct {
				Family string
				Name   string
			}
		}
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return presets.BaseOS{}, fmt.Errorf("trivy result could not be read: %w", err)
	}

	return presets.BaseOS{
		Family:  report.Metadata.OS.Family,
		Version: report.Metadata.OS.Name,
	}, nil
}

// resolveBaseSupport classifies every base, fetching each endoflife.date
// product exactly once however many images share it.
//
// The cache is per call rather than per process: this runs once per command,
// and a cache surviving between runs would be a lifecycle tracker, which is
// precisely what this is not.
func resolveBaseSupport(bases map[string]presets.BaseOS, now time.Time) map[string]presets.BaseSupport {
	type feed struct {
		cycles []presets.EOLCycle
		err    error
	}
	fetched := make(map[string]feed)

	support := make(map[string]presets.BaseSupport, len(bases))
	for image, base := range bases {
		product, mapped := presets.EOLProduct(base.Family)
		if !mapped {
			// An unmapped family and an image with no base at all are both
			// decided without asking anybody: ClassifyBase answers from the
			// family alone.
			support[image] = presets.ClassifyBase(base, nil, now)
			continue
		}

		got, seen := fetched[product]
		if !seen {
			got.cycles, got.err = eolCyclesFunc(product)
			fetched[product] = got
		}

		if got.err != nil {
			support[image] = presets.UncheckedBase(base, got.err)
			continue
		}
		support[image] = presets.ClassifyBase(base, got.cycles, now)
	}
	return support
}

// renderBaseSupport builds the end-of-support report.
//
// It returns a string for the same reason renderSecurityBaseline does: the
// report is one value, tested by reading it rather than by capturing a writer.
//
// It goes to stdout and to the workflow summary, never into
// SECURITY-BASELINE.md: every number here is relative to the day it was
// computed, and a committed file whose content changes daily makes its own diff
// worthless — the same reason that file carries no generation date. What the
// baseline records is the base itself, which is a fact about what we ship and
// changes only when we change it.
//
// The order is the one a reader needs: what has already ended, what is about to,
// what could not be checked, then the rest.
func renderBaseSupport(support map[string]presets.BaseSupport) string {
	byState := make(map[string][]string)
	for image, s := range support {
		byState[s.State] = append(byState[s.State], image)
	}
	for state := range byState {
		sort.Strings(byState[state])
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\nBase end of support (endoflife.date, warning at %d days):\n", presets.BaseEOLWarningDays)

	sections := []struct {
		state string
		title string
	}{
		{presets.BaseEnded, "No longer supported"},
		{presets.BaseEndingSoon, "Support ending soon"},
		{presets.BaseUnknown, "Could not be checked"},
		{presets.BaseUnchecked, "endoflife.date did not answer"},
	}

	flagged := 0
	for _, section := range sections {
		images := byState[section.state]
		if len(images) == 0 {
			continue
		}
		flagged += len(images)
		fmt.Fprintf(&sb, "\n  %s:\n", section.title)

		// An outage is one event, not one event per image: printing the same
		// connection error twenty times buries the sections that do say
		// something about the catalogue. Every other state is per image,
		// because every other state is a separate decision.
		if section.state == presets.BaseUnchecked {
			for _, reason := range distinctReasons(support, images) {
				fmt.Fprintf(&sb, "    %s\n", reason)
			}
			fmt.Fprintf(&sb, "    %d image(s) affected; nothing is claimed about them either way.\n", len(images))
			continue
		}

		for _, image := range images {
			fmt.Fprintf(&sb, "    %s\n      %s\n", image, support[image].Reason)
		}
	}

	if flagged == 0 {
		fmt.Fprintf(&sb, "\n  Every base with a known support window is supported for more than %d days.\n",
			presets.BaseEOLWarningDays)
	}

	fmt.Fprintf(&sb, "\n  %d supported, %d with no distribution base, %d image(s) flagged above.\n",
		len(byState[presets.BaseSupported]), len(byState[presets.BaseNone]), flagged)
	return sb.String()
}

// distinctReasons collapses the images sharing one reason onto a single line,
// in a stable order.
func distinctReasons(support map[string]presets.BaseSupport, images []string) []string {
	seen := make(map[string]bool, len(images))
	var reasons []string
	for _, image := range images {
		if reason := support[image].Reason; !seen[reason] {
			seen[reason] = true
			reasons = append(reasons, reason)
		}
	}
	sort.Strings(reasons)
	return reasons
}

// baseSupportAnnotations turns the flagged states into GitHub Actions
// annotations, so the daily audit run carries them on the run itself rather
// than only in a summary somebody has to open.
//
// The severities follow the ones the supply-chain policy already uses: an image
// whose reference vanished fails the run (#245), a frozen variant line warns
// (#252). An expired base is the same class as a frozen line — a repin decision
// with a human behind it, not a broken build — so it annotates and does not
// gate. An unresolvable base is an error because it is a gap in this code, one
// line away from being fixed.
//
// endoflife.date being unreachable produces a notice and nothing else. It is
// the one outcome that says nothing about the catalogue.
func baseSupportAnnotations(support map[string]presets.BaseSupport) []string {
	images := make([]string, 0, len(support))
	for image := range support {
		images = append(images, image)
	}
	sort.Strings(images)

	var (
		out       []string
		unchecked []string
	)
	for _, image := range images {
		s := support[image]
		level := ""
		switch s.State {
		case presets.BaseEnded, presets.BaseUnknown:
			level = "error"
		case presets.BaseEndingSoon:
			level = "warning"
		case presets.BaseUnchecked:
			// One outage, one annotation. Twenty notices carrying the same
			// connection error would push the states that need a decision off
			// the top of the run.
			unchecked = append(unchecked, image)
			continue
		default:
			continue
		}
		out = append(out, fmt.Sprintf("::%s::%s — %s", level, image, s.Reason))
	}

	if len(unchecked) > 0 {
		out = append(out, fmt.Sprintf("::notice::end of support not checked for %d image(s): %s",
			len(unchecked), strings.Join(distinctReasons(support, unchecked), "; ")))
	}
	return out
}

// baseCell is how the committed baseline prints one image's base.
//
// Three answers, and they must stay apart: a base, an image that genuinely has
// none, and an image nothing scanned. Printing the last two the same way is the
// confusion SECURITY-BASELINE.md exists to remove.
func baseCell(base presets.BaseOS, scanned bool) string {
	switch {
	case !scanned:
		return "not scanned"
	case base.Family == "":
		return "none"
	default:
		return strings.TrimSpace(base.String())
	}
}
