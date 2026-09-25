package guards

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
)

// pinnedImage is rule 1 of the supply-chain policy, the form the catalogue
// and Renovate's regex manager both expect: a readable tag and the digest it
// resolved to.
var pinnedImage = regexp.MustCompile(`^[^@:\s]+(:[^@\s]+)?@sha256:[a-f0-9]{64}$`)

// TestActionImagesArePinnedByDigest holds the action containers this
// repository runs, and the ones the complete example teaches, to the rule the
// catalogue is held to by TestCatalogueImagesArePinnedByDigest. The
// release-create action ran commitizen/commitizen:latest — whatever was last
// pushed to that tag, with the repository and .git mounted read-write — while
// every preset it sits next to was pinned.
func TestActionImagesArePinnedByDigest(t *testing.T) {
	for _, file := range []string{"cidx.toml", filepath.Join("examples", "cidx-complete.toml")} {
		cfg, err := config.Load(filepath.Join(projectRoot, file))
		if err != nil {
			t.Fatalf("loading %s: %v", file, err)
		}
		if len(cfg.Actions) == 0 {
			t.Fatalf("%s declares no action: the guard checks nothing", file)
		}
		for name, action := range cfg.Actions {
			if !pinnedImage.MatchString(action.Image) {
				t.Errorf("%s: [actions.%s] image %q is not pinned by digest (image:tag@sha256:...)", file, name, action.Image)
			}
		}
	}
}
