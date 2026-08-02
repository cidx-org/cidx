package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// No test in this file reaches endoflife.date. The cycle lists come either from
// the eolCyclesFunc seam — the same convention as latestTagFunc and
// resolveDigestFunc — or from a loopback httptest server, which is what proves
// the HTTP path itself without leaving the machine.

// stubEOL replaces the fetch for the duration of one test and records which
// products were asked for, so the caching can be asserted on.
func stubEOL(t *testing.T, answer func(product string) ([]presets.EOLCycle, error)) *[]string {
	t.Helper()

	original := eolCyclesFunc
	t.Cleanup(func() { eolCyclesFunc = original })

	var asked []string
	eolCyclesFunc = func(product string) ([]presets.EOLCycle, error) {
		asked = append(asked, product)
		return answer(product)
	}
	return &asked
}

func alpineFeed() []presets.EOLCycle {
	return decodeCycles(`[{"cycle":"3.24","eol":"2028-06-01"},{"cycle":"3.21","eol":"2026-11-01"},{"cycle":"3.20","eol":"2026-04-01"}]`)
}

func debianFeed() []presets.EOLCycle {
	return decodeCycles(`[{"cycle":"13","eol":"2028-08-09"},{"cycle":"12","eol":"2026-07-11"}]`)
}

// writeTrivyBase writes the one field this signal reads out of a Trivy report:
// the base the image was built on. The file name is the same contract the
// findings use.
func writeTrivyBase(t *testing.T, dir, image, family, version string) {
	t.Helper()

	report := map[string]any{
		"ArtifactName": image,
		"Metadata":     map[string]any{"OS": map[string]any{"Family": family, "Name": version}},
	}
	writeJSON(t, filepath.Join(dir, scanResultFile("trivy", image)), report)
}

// TestScanBaseOSReadsTheBaseTrivyAlreadyReports: the whole input to this signal
// is a field the scans have been producing and the catalogue has been throwing
// away (#303).
func TestScanBaseOSReadsTheBaseTrivyAlreadyReports(t *testing.T) {
	dir := t.TempDir()
	writeTrivyBase(t, dir, runningRef, "alpine", "3.20.10")

	base, err := scanBaseOS(dir, runningRef)
	if err != nil {
		t.Fatalf("scanBaseOS: %v", err)
	}
	if base.Family != "alpine" || base.Version != "3.20.10" {
		t.Errorf("scanBaseOS = %+v, want alpine 3.20.10", base)
	}
}

// TestScanBaseOSSeparatesNoBaseFromNoScan: an image built on scratch reports no
// OS and that is an answer; an image nobody scanned reports nothing and that is
// a gap. The baseline prints them differently, so they must not arrive the same.
func TestScanBaseOSSeparatesNoBaseFromNoScan(t *testing.T) {
	dir := t.TempDir()
	writeTrivyBase(t, dir, runningRef, "", "")

	base, err := scanBaseOS(dir, runningRef)
	if err != nil {
		t.Fatalf("an image with no OS metadata is not an error: %v", err)
	}
	if base.Family != "" {
		t.Errorf("scanBaseOS = %+v, want an empty base", base)
	}

	if _, err := scanBaseOS(dir, candidateRef); !errors.Is(err, errNoScanResults) {
		t.Errorf("scanBaseOS for an unscanned image = %v, want %v", err, errNoScanResults)
	}
}

// TestResolveBaseSupportFetchesEachProductOnce: twenty-two catalogue images sit
// on three distributions, and this must cost three requests rather than
// twenty-two. A minimal client, not a lifecycle tracker.
func TestResolveBaseSupportFetchesEachProductOnce(t *testing.T) {
	asked := stubEOL(t, func(product string) ([]presets.EOLCycle, error) {
		switch product {
		case "alpine-linux":
			return alpineFeed(), nil
		case "debian":
			return debianFeed(), nil
		}
		return nil, errors.New("unexpected product " + product)
	})

	bases := map[string]presets.BaseOS{
		"a": {Family: "alpine", Version: "3.24.1"},
		"b": {Family: "alpine", Version: "3.20.10"},
		"c": {Family: "alpine", Version: "3.21.4"},
		"d": {Family: "debian", Version: "13.6"},
		"e": {Family: "debian", Version: "12.11"},
		"f": {}, // scratch: asks nobody
	}

	// 2026-08-03 is exactly BaseEOLWarningDays before Alpine 3.21 ends, so `c`
	// is the boundary case: one day earlier it is still quiet.
	support := resolveBaseSupport(bases, day("2026-08-03"))

	if len(*asked) != 2 {
		t.Errorf("endoflife.date was asked %d times for 2 distinct products: %v", len(*asked), *asked)
	}
	for image, want := range map[string]string{
		"a": presets.BaseSupported,
		"b": presets.BaseEnded,
		"c": presets.BaseEndingSoon,
		"d": presets.BaseSupported,
		"e": presets.BaseEnded,
		"f": presets.BaseNone,
	} {
		if got := support[image].State; got != want {
			t.Errorf("%s: state = %q, want %q (%s)", image, got, want, support[image].Reason)
		}
	}
}

// TestResolveBaseSupportNeverAsksAboutAnUnmappedFamily: the fail-closed path is
// decided from the family alone. Asking endoflife.date for a product nothing
// maps would spend a request to be told 404, and a 404 is indistinguishable
// from an outage — which would file our own gap under "the API was down".
func TestResolveBaseSupportNeverAsksAboutAnUnmappedFamily(t *testing.T) {
	asked := stubEOL(t, func(string) ([]presets.EOLCycle, error) {
		return nil, errors.New("no request should have been made")
	})

	support := resolveBaseSupport(map[string]presets.BaseOS{
		"ubuntu-image": {Family: "ubuntu", Version: "24.04"},
	}, day("2026-08-02"))

	if len(*asked) != 0 {
		t.Errorf("an unmapped family cost %d request(s): %v", len(*asked), *asked)
	}
	got := support["ubuntu-image"]
	if got.State != presets.BaseUnknown {
		t.Fatalf("state = %q, want %q", got.State, presets.BaseUnknown)
	}
	if !strings.Contains(got.Reason, "ubuntu") {
		t.Errorf("reason %q does not name the family that could not be mapped", got.Reason)
	}
}

// TestResolveBaseSupportDegradesWhenTheAPIIsSilent is the requirement that
// outranks the feature: an outage at endoflife.date must never fail a scan or a
// monitor run. Every base comes back `unchecked`, with the reason, and nothing
// asks for action.
func TestResolveBaseSupportDegradesWhenTheAPIIsSilent(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) {
		return nil, errors.New("dial tcp 151.101.1.195:443: i/o timeout")
	})

	support := resolveBaseSupport(map[string]presets.BaseOS{
		"alpine-image": {Family: "alpine", Version: "3.20.10"},
		"debian-image": {Family: "debian", Version: "13.6"},
	}, day("2026-08-02"))

	for image, got := range support {
		if got.State != presets.BaseUnchecked {
			t.Errorf("%s: state = %q, want %q", image, got.State, presets.BaseUnchecked)
		}
		if got.NeedsAttention() {
			t.Errorf("%s: an endoflife.date outage asked the catalogue to act", image)
		}
		if !strings.Contains(got.Reason, "i/o timeout") {
			t.Errorf("%s: reason %q does not say the check did not happen", image, got.Reason)
		}
	}

	// And the report says so out loud rather than printing an empty section
	// that reads like a clean bill of health.
	report := renderBaseSupport(support)
	if !strings.Contains(report, "endoflife.date did not answer") {
		t.Errorf("the report does not state that the check was skipped:\n%s", report)
	}
	if strings.Contains(report, "Every base with a known support window is supported") {
		t.Errorf("an unchecked catalogue was reported as healthy:\n%s", report)
	}
}

// TestFetchEOLCyclesReportsEveryFailureAsAnError walks the HTTP path itself
// against a loopback server: a product that does not exist answers 404 with an
// HTML page, and a body that is not a cycle list must not decode into an empty
// one — an empty list would classify every base as unresolvable rather than
// unchecked, which blames the catalogue for somebody else's outage.
func TestFetchEOLCyclesReportsEveryFailureAsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/alpine-linux.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"cycle":"3.20","eol":"2026-04-01"}]`))
		case "/html.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Page not Found</body></html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Page not Found</body></html>"))
		}
	}))
	t.Cleanup(server.Close)

	original := endoflifeAPI
	t.Cleanup(func() { endoflifeAPI = original })
	endoflifeAPI = server.URL

	cycles, err := fetchEOLCycles("alpine-linux")
	if err != nil {
		t.Fatalf("fetchEOLCycles on a good answer: %v", err)
	}
	if len(cycles) != 1 || cycles[0].Cycle != "3.20" {
		t.Errorf("fetchEOLCycles decoded %+v, want the single 3.20 line", cycles)
	}

	if _, err := fetchEOLCycles("wolfi"); err == nil {
		t.Error("a 404 was read as a cycle list")
	}
	if _, err := fetchEOLCycles("html"); err == nil {
		t.Error("an HTML page answered with 200 was read as a cycle list")
	}

	server.Close()
	if _, err := fetchEOLCycles("alpine-linux"); err == nil {
		t.Error("an unreachable endoflife.date was read as a cycle list")
	}
}

// TestBaseSupportAnnotationsMatchTheSeveritiesThePolicyUses: the workflow reads
// these, and their level is the difference between a run somebody looks at and
// one they scroll past. They follow what is already in place — a deleted image
// fails the run (#245), a frozen variant line warns (#252) — and an
// endoflife.date outage, which says nothing about the catalogue, is a notice.
func TestBaseSupportAnnotationsMatchTheSeveritiesThePolicyUses(t *testing.T) {
	support := map[string]presets.BaseSupport{
		"ended":     {State: presets.BaseEnded, Reason: "alpine 3.20 stopped being supported"},
		"soon":      {State: presets.BaseEndingSoon, Reason: "alpine 3.21 stops being supported"},
		"unknown":   {State: presets.BaseUnknown, Reason: "no endoflife.date product is mapped"},
		"unchecked": {State: presets.BaseUnchecked, Reason: "end of support not checked"},
		"fine":      {State: presets.BaseSupported, Reason: "debian 13 is supported"},
		"scratch":   {State: presets.BaseNone, Reason: "the image has no distribution base"},
	}

	got := strings.Join(baseSupportAnnotations(support), "\n")

	for _, want := range []string{
		"::error::ended —",
		"::warning::soon —",
		"::error::unknown —",
		// One outage, one annotation: the images are counted rather than
		// listed, so the states needing a decision stay at the top of the run.
		"::notice::end of support not checked for 1 image(s): end of support not checked",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("annotations do not contain %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"fine", "scratch"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a healthy image was annotated (%q):\n%s", unwanted, got)
		}
	}
}

// TestRenderBaseSupportLeadsWithWhatIsAlreadyDead: the report is read top-down
// and the section that costs money must not be under two screens of healthy
// images.
func TestRenderBaseSupportLeadsWithWhatIsAlreadyDead(t *testing.T) {
	support := map[string]presets.BaseSupport{
		"good":  {State: presets.BaseSupported},
		"soon":  {State: presets.BaseEndingSoon, Reason: "stops soon"},
		"dead":  {State: presets.BaseEnded, Reason: "stopped already"},
		"blind": {State: presets.BaseUnknown, Reason: "cannot be checked"},
	}

	report := renderBaseSupport(support)

	dead := strings.Index(report, "No longer supported")
	soon := strings.Index(report, "Support ending soon")
	blind := strings.Index(report, "Could not be checked")

	if dead < 0 || soon < 0 || blind < 0 {
		t.Fatalf("a section is missing:\n%s", report)
	}
	if dead >= soon || soon >= blind {
		t.Errorf("sections are out of order (%d, %d, %d):\n%s", dead, soon, blind, report)
	}
	if !strings.Contains(report, "3 image(s) flagged") {
		t.Errorf("the report does not count what it flagged:\n%s", report)
	}
}

// TestRenderBaseSupportSaysSoWhenNothingIsWrong: a report that prints nothing
// when everything is fine reads like a report that failed to run.
func TestRenderBaseSupportSaysSoWhenNothingIsWrong(t *testing.T) {
	report := renderBaseSupport(map[string]presets.BaseSupport{
		"good":    {State: presets.BaseSupported},
		"scratch": {State: presets.BaseNone},
	})

	if !strings.Contains(report, "Every base with a known support window is supported") {
		t.Errorf("a healthy catalogue produced no statement:\n%s", report)
	}
}

// day parses the dates the tables above are written in.
func day(date string) time.Time {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return parsed
}

func decodeCycles(document string) []presets.EOLCycle {
	var cycles []presets.EOLCycle
	if err := json.Unmarshal([]byte(document), &cycles); err != nil {
		panic(err)
	}
	return cycles
}
