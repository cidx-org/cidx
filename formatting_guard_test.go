package main

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/config"
)

// writeOptions are the catalogue options that turn a checker into a writer:
// prettier's `write`, golangci-lint's and ruff's `fix`. A tool running under one
// of them rewrites the files it was asked to inspect and exits 0 whatever it
// found.
var writeOptions = []string{"write", "fix"}

// writeFlags are the same switch spelled into a command, which is how a custom
// [containers.NAME] declaration says it.
var writeFlags = []string{"--write", "--fix"}

// TestCheckingPhasesNeverWrite is the standing guard behind issue #272.
//
// `[containers.prettier] write = true` put prettier in write mode inside the
// `code` phase: the Code Quality job rewrote the files it was meant to check and
// exited 0, so CI never once failed on formatting drift, and `cidx run code`
// mutated the working tree as a side effect of a check — including
// SECURITY-BASELINE.md, a generated file it fought with its generator.
//
// The failure is invisible from the outside: the job stays green either way.
// That is the whole problem with a decorative check, so the invariant is pinned
// here rather than left to review — a container reachable from a pipeline
// checks, it does not write. Writing lives in the `format` phase, which no
// pipeline lists, and this test is also what keeps it out of one.
func TestCheckingPhasesNeverWrite(t *testing.T) {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		t.Fatalf("failed to load cidx.toml: %v", err)
	}

	// The phases CI can reach: those any pipeline lists.
	reachable := make(map[string]bool)
	for _, pipeline := range cfg.Pipelines {
		for _, phase := range pipeline.Phases {
			reachable[phase] = true
		}
	}
	if !reachable["code"] {
		t.Fatal("no pipeline lists the `code` phase — this guard would examine the wrong config")
	}

	examined := 0
	for phase := range reachable {
		for _, container := range cfg.Phases[phase].Containers {
			examined++
			overrides := cfg.Overrides[container]

			for _, option := range writeOptions {
				if isEnabled(overrides[option]) {
					t.Errorf("[containers.%s] %s is enabled and a pipeline lists the %q phase: "+
						"the tool would rewrite what it was asked to check and exit 0 whatever it found (#272). "+
						"Writing belongs in a phase no pipeline lists — the `format` phase, run on demand.",
						container, option, phase)
				}
			}

			command, _ := overrides["command"].(string)
			for _, flag := range writeFlags {
				if strings.Contains(command, flag) {
					t.Errorf("[containers.%s] runs %q and a pipeline lists the %q phase it belongs to: "+
						"a check that writes exits 0 whatever it finds (#272). "+
						"Writing belongs in a phase no pipeline lists — the `format` phase, run on demand.",
						container, flag, phase)
				}
			}
		}
	}

	if examined == 0 {
		t.Fatal("no container examined — the guard would pass vacuously")
	}
}

// isEnabled reports whether an override value reads as an enabled boolean. TOML
// gives a real bool; a quoted "true" is accepted here so the guard sees it too,
// even though the option merge refuses it (#214).
func isEnabled(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}
