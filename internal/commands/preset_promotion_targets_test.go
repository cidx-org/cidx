package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromotionTargetsDescribeTheExactPullRequestCandidate(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.toml")
	current := filepath.Join(dir, "current.toml")
	write := func(path, image string) {
		t.Helper()
		data := "[presets.lint]\nname = \"lint\"\nimage = \"" + image + "\"\n" +
			"[presets.test]\nname = \"test\"\nimage = \"" + image + "\"\n"
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	old := "example/tool:1.0.0@sha256:" + zeroDigest
	next := "example/tool:1.1.0@sha256:" + strings.Repeat("1", 64)
	write(base, old)
	write(current, next)

	targets, err := promotionTargets(base, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want one deduplicated image change", len(targets))
	}
	got := targets[0]
	if got.CurrentImage != old || got.ScanImage != next || !got.IsUpdate {
		t.Errorf("target = %+v, want exact base and proposed references", got)
	}
	if len(got.Presets) != 2 || got.Presets[0] != "lint" || got.Presets[1] != "test" {
		t.Errorf("presets = %v, want [lint test]", got.Presets)
	}
}

func TestPromotionTargetsRejectMutableCandidate(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.toml")
	current := filepath.Join(dir, "current.toml")
	if err := os.WriteFile(base, []byte("[presets.tool]\nimage = \"example/tool:1.0@sha256:"+zeroDigest+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("[presets.tool]\nimage = \"example/tool:1.1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := promotionTargets(base, current); err == nil {
		t.Fatal("mutable candidate was accepted")
	}
}
