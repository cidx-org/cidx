package config

import (
	"strings"
	"testing"
)

// TestPhaseArtifactsError pins what a phase may upload: a name that is safe to
// write into a step and that is not the bootstrap hand-off, at least one path
// inside the workspace, and a retention GitHub accepts.
func TestPhaseArtifactsError(t *testing.T) {
	ok := func(mod func(*PhaseArtifacts)) PhaseArtifacts {
		a := PhaseArtifacts{Name: "evidence", Paths: []string{".probatum/runs/*/"}, RetentionDays: 7}
		if mod != nil {
			mod(&a)
		}
		return a
	}
	cases := []struct {
		name string
		a    PhaseArtifacts
		want string
	}{
		{"valid", ok(nil), ""},
		{"default retention", ok(func(a *PhaseArtifacts) { a.RetentionDays = 0 }), ""},
		{"no name", ok(func(a *PhaseArtifacts) { a.Name = "" }), "name"},
		{"unsafe name", ok(func(a *PhaseArtifacts) { a.Name = "a\nb" }), "name"},
		{"the bootstrap artifact", ok(func(a *PhaseArtifacts) { a.Name = BootstrapArtifact }), "hands the cidx binary"},
		{"no paths", ok(func(a *PhaseArtifacts) { a.Paths = nil }), "no paths"},
		{"path outside the workspace", ok(func(a *PhaseArtifacts) { a.Paths = []string{"../x"} }), "workspace"},
		{"absolute path", ok(func(a *PhaseArtifacts) { a.Paths = []string{"/etc"} }), "workspace"},
		{"retention past the maximum", ok(func(a *PhaseArtifacts) { a.RetentionDays = 91 }), "retention_days"},
		{"retention that was not a number", ok(func(a *PhaseArtifacts) { a.RetentionDays = -1 }), "retention_days"},
	}
	for _, c := range cases {
		err := c.a.Error()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected error: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: error %v, want one mentioning %q", c.name, err, c.want)
		}
	}
}

// TestToArtifactsReadsTheInlineTable is the shape lithair wrote in the issue.
func TestToArtifactsReadsTheInlineTable(t *testing.T) {
	a := toArtifacts(map[string]any{
		"name":           "cluster-evidence",
		"paths":          []any{".probatum/runs/x-*/"},
		"retention_days": int64(7),
	})
	if a == nil || a.Name != "cluster-evidence" || len(a.Paths) != 1 || a.RetentionDays != 7 {
		t.Fatalf("read %+v", a)
	}
	if got := toArtifacts(map[string]any{"name": "n", "paths": []any{"p"}, "retention_days": "7"}); got.RetentionDays != -1 {
		t.Errorf(`retention_days = "7" must read as invalid (-1), got %d`, got.RetentionDays)
	}
	if toArtifacts(nil) != nil {
		t.Error("no table must read as no artifacts")
	}
}
