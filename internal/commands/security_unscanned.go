package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// unscannedFile is where `cidx security scan` leaves, inside the results
// directory, the catalogue images it could not scan in full and why: one
// `image<TAB>cause` line each. It is how a scan that fails still tells the
// baseline which images to leave out, and the triage index what is missing.
//
// It is absent when the scan was complete, and it holds every image until the
// scan has said otherwise, so that a run that dies half way is never mistaken
// for a complete one.
const unscannedFile = "unscanned.tsv"

// catalogueImageNames lists the catalogue's images in a fixed order.
func catalogueImageNames(imagePresets map[string][]string) []string {
	images := make([]string, 0, len(imagePresets))
	for image := range imagePresets {
		images = append(images, image)
	}
	sort.Strings(images)
	return images
}

func unscannedAll(images []string, reason string) map[string]string {
	all := make(map[string]string, len(images))
	for _, image := range images {
		all[image] = reason
	}
	return all
}

// unscannedImages is every image the scan could not answer for in full: the gaps,
// and the images only one scanner answered for. Their sheets would be built on a
// measurement known to be partial, so they get none.
func unscannedImages(scans []presets.ImageScan, gaps []presets.ScanGap) map[string]string {
	out := make(map[string]string)
	for _, scan := range scans {
		var causes []string
		for _, m := range scan.Missing {
			causes = append(causes, fmt.Sprintf("%s did not answer: %s", m.Scanner, m.Reason))
		}
		if len(causes) > 0 {
			out[scan.Image] = strings.Join(causes, "; ")
		}
	}
	for _, gap := range gaps {
		out[gap.Image] = gap.Reason
	}
	return out
}

// writeUnscanned records the unscanned images, or removes the file when there
// are none.
func writeUnscanned(resultsDir string, unscanned map[string]string) error {
	path := filepath.Join(resultsDir, unscannedFile)
	if len(unscanned) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
		return nil
	}
	images := make([]string, 0, len(unscanned))
	for image := range unscanned {
		images = append(images, image)
	}
	sort.Strings(images)

	var sb strings.Builder
	for _, image := range images {
		sb.WriteString(image + "\t" + oneLine(unscanned[image]) + "\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// readUnscanned reads what the scan recorded, keeping the catalogue's images.
// No file is a complete scan, or results that did not come from `security scan`.
func readUnscanned(resultsDir string, imagePresets map[string][]string) (map[string]string, error) {
	path := filepath.Join(resultsDir, unscannedFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		image, reason, _ := strings.Cut(line, "\t")
		if _, ok := imagePresets[image]; ok {
			out[image] = reason
		}
	}
	return out, nil
}

// cachedResult tells whether the scanner's result for the image, left by an
// earlier run, still stands: the image is pinned by digest, so it cannot have
// changed, and the result is not older than the advisory database in the cache,
// so no newer advisory is missing from it. Without a cache directory nothing is
// reused.
func (s scanRig) cachedResult(scanner, image, resultsDir string) bool {
	if s.cache == "" || !strings.Contains(image, "@sha256:") {
		return false
	}
	path := filepath.Join(resultsDir, scanFileName(scanner, image))
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil || !json.Valid(data) {
		return false
	}
	built, ok := databaseTime(s.cache, scanner)
	return ok && !info.ModTime().Before(built)
}

// databaseTime is when the advisory database in the cache was last written.
func databaseTime(cache, scanner string) (time.Time, bool) {
	if scanner == "trivy" {
		info, err := os.Stat(filepath.Join(cache, "trivy", "db", "trivy.db"))
		if err != nil {
			return time.Time{}, false
		}
		return info.ModTime(), true
	}
	var newest time.Time
	_ = filepath.Walk(filepath.Join(cache, "grype", "db"), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, !newest.IsZero()
}
