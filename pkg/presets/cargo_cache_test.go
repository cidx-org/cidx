package presets

import (
	"strings"
	"testing"
)

func TestCargoCacheIsReadOnlyAndCopiedPerRun(t *testing.T) {
	for _, name := range []string{"cargo-test", "cargo-build", "clippy", "rustfmt"} {
		base, err := Get(name)
		if err != nil {
			t.Fatal(err)
		}
		before := strings.Join(base.Volumes, ",")
		p := base.MergeWith(map[string]any{"cargo_cache": "/srv/seed"})

		if !strings.Contains(strings.Join(p.Volumes, ","), "/srv/seed:/cidx/cargo-seed:ro") {
			t.Errorf("%s: seed not mounted read-only: %v", name, p.Volumes)
		}
		if p.Env["CARGO_NET_OFFLINE"] != "true" || p.Env["CARGO_HOME"] != "/tmp/cargo-home" {
			t.Errorf("%s: env = %v", name, p.Env)
		}
		if !strings.HasPrefix(p.Command, `sh -c 'mkdir -p "$CARGO_HOME" && cp -a /cidx/cargo-seed/.`) {
			t.Errorf("%s: command = %s", name, p.Command)
		}
		// the catalogue entry must not be altered by a merge
		if strings.Join(base.Volumes, ",") != before || base.Env["CARGO_NET_OFFLINE"] != "" {
			t.Errorf("%s: merge mutated the preset", name)
		}
	}
}

func TestCargoCacheWithAnotherOptionKeepsBothInTheScript(t *testing.T) {
	base, _ := Get("cargo-test")
	p := base.MergeWith(map[string]any{"cargo_cache": "/srv/seed", "release": true})
	if !strings.HasSuffix(p.Command, "cargo test --release'") || !strings.Contains(p.Command, "cargo-seed") {
		t.Errorf("command = %s", p.Command)
	}
}
