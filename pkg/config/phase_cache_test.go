package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPhaseCacheError pins what a phase may declare: a cache needs a key and a
// key needs a cache, paths stay inside the workspace, and nothing that could
// leave the YAML scalar or the hashFiles('...') expression it is written into.
func TestPhaseCacheError(t *testing.T) {
	cases := []struct {
		name  string
		phase Phase
		want  string // fragment of the error, "" for none
	}{
		{"no cache is fine", Phase{}, ""},
		{"a keyed cache", Phase{Cache: []string{"target"}, CacheKey: []string{"**/Cargo.lock"}}, ""},
		{"cache without a key", Phase{Cache: []string{"target"}}, "cache_key"},
		{"key without a cache", Phase{CacheKey: []string{"Cargo.lock"}}, "cache"},
		{"absolute path", Phase{Cache: []string{"/root/.cargo"}, CacheKey: []string{"Cargo.lock"}}, "workspace"},
		{"parent path", Phase{Cache: []string{"../outside"}, CacheKey: []string{"Cargo.lock"}}, "workspace"},
		{"nested parent path", Phase{Cache: []string{"a/../../b"}, CacheKey: []string{"Cargo.lock"}}, "workspace"},
		{"quote in a key file", Phase{Cache: []string{"target"}, CacheKey: []string{"a') }} ${{ secrets.X"}}, "only letters"},
		{"newline in a path", Phase{Cache: []string{"target\nkey: x"}, CacheKey: []string{"Cargo.lock"}}, "only letters"},
	}
	for _, c := range cases {
		err := c.phase.CacheError()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected error: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: error %v, want one mentioning %q", c.name, err, c.want)
		}
	}
}

// TestCacheRestoreFallbackDeclarations pins the opt-out of the restore-keys
// fallback (#515): absent is the default, false is honoured, a value that is not
// a boolean is refused rather than taken for the default, and the key without a
// cache to apply to is refused like cache_key without one.
func TestCacheRestoreFallbackDeclarations(t *testing.T) {
	parse := func(toml string) (Phase, error) {
		path := filepath.Join(t.TempDir(), "cidx.toml")
		if err := os.WriteFile(path, []byte(toml+"\n[pipelines.ci]\nphases = [\"test\"]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			return Phase{}, err
		}
		return cfg.Phases["test"], nil
	}
	base := "[test]\ncontainers = [\"go-test\"]\ncache = [\"target\"]\ncache_key = [\"Cargo.lock\"]\n"

	absent, err := parse(base)
	if err != nil || absent.CacheRestoreFallback != nil || absent.CacheError() != nil {
		t.Fatalf("absent: %+v, %v", absent, err)
	}
	off, err := parse(base + "cache_restore_fallback = false\n")
	if err != nil || off.CacheRestoreFallback == nil || *off.CacheRestoreFallback || off.CacheError() != nil {
		t.Fatalf("false: %+v, %v", off, err)
	}
	if bad, err := parse(base + "cache_restore_fallback = \"false\"\n"); err != nil || bad.CacheError() == nil {
		t.Errorf(`the string "false" must be refused, not read as the default: %+v, %v`, bad, err)
	}
	orphan, err := parse("[test]\ncontainers = [\"go-test\"]\ncache_restore_fallback = false\n")
	if err != nil || orphan.CacheError() == nil || !strings.Contains(orphan.CacheError().Error(), "no cache") {
		t.Errorf("a fallback with no cache must be refused: %+v, %v", orphan, err)
	}
}
