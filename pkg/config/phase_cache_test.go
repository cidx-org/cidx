package config

import (
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
