package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/pipeline"
)

// The two ways the pre-push check has nothing to say. Both let the push
// through: cpw is a workflow command, and a missing phase or a stopped Docker
// daemon is not a reason to stand between someone and their remote.
var (
	// errNoCodePhase: no cidx.toml, or one that declares no `code` phase.
	errNoCodePhase = errors.New("no code phase configured")

	// errNoContainerRuntime: neither Docker's daemon nor Podman's
	// Docker-compatible socket answers. Same reading as `cidx doctor` makes
	// (#190) — a Podman CLI without its socket is not a runtime cidx can use.
	errNoContainerRuntime = errors.New("no container runtime available")
)

// runCodePhase runs the `code` phase against the working tree, exactly as
// `cidx run code` does.
//
// It is reached through the verifyBeforePush variable so tests can replace it:
// this is the one function in the pre-push path that starts containers, and no
// test in this repository starts a container.
func runCodePhase(ctx context.Context) error {
	path, err := config.FindConfig()
	if err != nil {
		return errNoCodePhase
	}

	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", path, err)
	}

	if len(cfg.Phases["code"].Containers) == 0 {
		return errNoCodePhase
	}

	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		return errNoContainerRuntime
	}
	defer func() { _ = selector.Close() }()

	if !selector.DockerAvailable() && !selector.PodmanAvailable() {
		return errNoContainerRuntime
	}

	// BackendAuto explicitly: the zero BackendType is the empty string, which
	// the runner reports as `Backend:  (forced)` -- a backend nobody chose,
	// named after nothing.
	opts := pipeline.RunnerOptions{Backend: executor.BackendAuto}

	return pipeline.NewRunnerWithOptions(cfg, selector, opts).Run(ctx, "code")
}

// verifyBeforePush is the seam. Replace it in tests; never call it directly.
var verifyBeforePush = runCodePhase
