package drift

import "testing"

// TestMajorOf pins what counts as a comparable action ref: a v-prefixed
// version, whole or partial. A SHA a project pinned on purpose, or a branch,
// is not a version and must never read as "behind".
func TestMajorOf(t *testing.T) {
	cases := map[string]struct {
		major int
		ok    bool
	}{
		"v6":     {6, true},
		"v6.1":   {6, true},
		"v7.0.1": {7, true},
		"3d3c42e5aac5ba805825da76410c181273ba90b1": {0, false},
		"main": {0, false},
		"6":    {0, false},
	}
	for ref, want := range cases {
		major, ok := MajorOf(ref)
		if ok != want.ok || (ok && major != want.major) {
			t.Errorf("MajorOf(%q) = %d, %v; want %d, %v", ref, major, ok, want.major, want.ok)
		}
	}
}
