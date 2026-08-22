package presets

import (
	"slices"
	"testing"
)

// TestCatalogueCapabilitiesMatchTheDeclarations pins the derivation to the real
// catalogue, the way the discussion measured it: every release preset declares
// a credential, and the two socket-mounting presets are the ones that mount it.
// A release preset added without its token env, or a new socket mount, moves a
// decision context — this test is what makes that a red diff instead of a
// silently changed fingerprint.
func TestCatalogueCapabilitiesMatchTheDeclarations(t *testing.T) {
	catalogue, err := Catalogue()
	if err != nil {
		t.Fatalf("loading catalogue: %v", err)
	}

	for name, p := range catalogue {
		caps := Capabilities(p)
		if p.Phase == "release" && !slices.Contains(caps, CapabilityPublishingCredential) {
			t.Errorf("release preset %q derives no %s: its declarations name no credential", name, CapabilityPublishingCredential)
		}
	}

	for _, name := range []string{"docker-buildx", "goreleaser"} {
		p, ok := catalogue[name]
		if !ok {
			t.Fatalf("catalogue no longer ships %q", name)
		}
		if !slices.Contains(Capabilities(p), CapabilityDockerSocket) {
			t.Errorf("%q no longer derives %s: if the mount really left, decisions expecting it must be re-reviewed, not the test relaxed", name, CapabilityDockerSocket)
		}
	}
}
