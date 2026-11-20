package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnwritableSharedScanCacheFallsBackAndNamesTheHostPath: a shared cache the
// process cannot write to does not sink the scan, and the warning says where
// the cache is on the host, who owns it and how to repair it.
func TestUnwritableSharedScanCacheFallsBackAndNamesTheHostPath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere: the cache cannot be made unwritable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, ".cache", "cidx", "scan")
	// MkdirAll applies the mode to every directory it creates, so the parents
	// would be unwritable too: create them open, then close only the cache.
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shared, 0o755) })

	work := t.TempDir()
	fallback := filepath.Join(work, ".cidx", "scan")

	// The container sees the cache at another path than the host does.
	cache, hostCache, warning, err := prepareScanCache(shared, shared, "", fallback, fallback)
	if err != nil {
		t.Fatalf("the scan must go on with the fallback, got %v", err)
	}
	if cache != fallback || hostCache != fallback {
		t.Errorf("cache = %q, host = %q, want the fallback %q", cache, hostCache, fallback)
	}
	for _, want := range []string{shared, "owner:", "chown -R"} {
		if !strings.Contains(warning, want) {
			t.Errorf("the warning lacks %q:\n%s", want, warning)
		}
	}
}

func TestSharedScanCacheWarningNamesHostPathNotContainerPath(t *testing.T) {
	dir := t.TempDir()
	got := sharedCacheWarning(dir, "/home/arcker/.cache/cidx/scan", ".cidx/scan", os.ErrPermission)
	if !strings.Contains(got, "/home/arcker/.cache/cidx/scan") {
		t.Errorf("the warning does not name the host path:\n%s", got)
	}
}

func TestWritableSharedScanCacheGivesNoWarning(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "scan")
	cache, _, warning, err := prepareScanCache(shared, shared, "", "", "")
	if err != nil || warning != "" || cache != shared {
		t.Errorf("cache=%q warning=%q err=%v, want the shared cache and no warning", cache, warning, err)
	}
}
