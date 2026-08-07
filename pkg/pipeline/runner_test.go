package pipeline

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/sirupsen/logrus"
)

func TestRunnerOptions_Defaults(t *testing.T) {
	opts := RunnerOptions{
		Backend:     executor.BackendAuto,
		Parallel:    false,
		Concurrency: 2,
	}

	if opts.Backend != executor.BackendAuto {
		t.Errorf("Expected backend auto, got %s", opts.Backend)
	}

	if opts.Parallel != false {
		t.Error("Expected parallel false by default")
	}

	if opts.Concurrency != 2 {
		t.Errorf("Expected concurrency 2, got %d", opts.Concurrency)
	}
}

func TestRunnerOptions_Parallel(t *testing.T) {
	opts := RunnerOptions{
		Backend:     executor.BackendDocker,
		Parallel:    true,
		Concurrency: 4,
	}

	if !opts.Parallel {
		t.Error("Expected parallel to be true")
	}

	if opts.Concurrency != 4 {
		t.Errorf("Expected concurrency 4, got %d", opts.Concurrency)
	}
}

func TestBackendType_String(t *testing.T) {
	tests := []struct {
		backend executor.BackendType
		want    string
	}{
		{executor.BackendAuto, "auto"},
		{executor.BackendDocker, "docker"},
		{executor.BackendPodman, "podman"},
	}

	for _, tt := range tests {
		if string(tt.backend) != tt.want {
			t.Errorf("Expected %s, got %s", tt.want, tt.backend)
		}
	}
}

func TestExpandWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		volumes   []string
		want      []string
	}{
		{
			"basic replacement",
			"/home/user/project",
			[]string{"${WORKSPACE}:/app"},
			[]string{"/home/user/project:/app"},
		},
		{
			"multiple volumes",
			"/src",
			[]string{"${WORKSPACE}:/app", "${WORKSPACE}/config:/config"},
			[]string{"/src:/app", "/src/config:/config"},
		},
		{
			"no placeholder",
			"/src",
			[]string{"/static:/data"},
			[]string{"/static:/data"},
		},
		{
			"empty volumes",
			"/src",
			[]string{},
			[]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				config: &config.Config{Workspace: tt.workspace},
			}
			got := r.expandWorkspace(tt.volumes)
			if len(got) != len(tt.want) {
				t.Fatalf("expandWorkspace() length = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("expandWorkspace()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCheckWorkdirCoveredByVolumes locks down the #151 runtime guardrail:
// silent "no files found" failures from a workdir that isn't bind-mounted
// must surface as a clear, actionable error before the container starts.
func TestCheckWorkdirCoveredByVolumes(t *testing.T) {
	tests := []struct {
		name    string
		workdir string
		volumes []string
		wantErr bool
	}{
		{
			name:    "workdir matches volume target exactly",
			workdir: "/work",
			volumes: []string{"/home/user/project:/work"},
			wantErr: false,
		},
		{
			name:    "workdir is a subdir of volume target (monorepo case)",
			workdir: "/work/client-react",
			volumes: []string{"/home/user/project:/work"},
			wantErr: false,
		},
		{
			name:    "consumer repro: workdir override without matching volume",
			workdir: "/src/client-react",
			volumes: []string{"/home/user/project:/work"},
			wantErr: true,
		},
		{
			name:    "user remounted to match custom workdir",
			workdir: "/src/client-react",
			volumes: []string{"/home/user/project/client-react:/src/client-react"},
			wantErr: false,
		},
		{
			name:    "workdir covered by one of multiple mounts",
			workdir: "/work/.git",
			volumes: []string{"/home/user/project:/work", "/home/user/project/.git:/work/.git"},
			wantErr: false,
		},
		{
			name:    "empty workdir is fine",
			workdir: "",
			volumes: []string{"/home/user/project:/work"},
			wantErr: false,
		},
		{
			name:    "no volumes at all is fine (host network style)",
			workdir: "/anywhere",
			volumes: nil,
			wantErr: false,
		},
		{
			name:    "volume with mount options (ro) still parses",
			workdir: "/kaniko/.docker",
			volumes: []string{"/home/user/.docker:/kaniko/.docker:ro"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkWorkdirCoveredByVolumes(tt.workdir, tt.volumes)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkWorkdirCoveredByVolumes(%q, %v) error = %v, wantErr %v",
					tt.workdir, tt.volumes, err, tt.wantErr)
			}
		})
	}
}

// TestRun_TargetResolution covers #216: `cidx run <name>` must accept a
// container declared only through [containers.NAME] with an `image` field,
// not just built-in presets. The custom container is only rejected at the
// resolution step — anything past it (executor selection, execution) is out
// of scope here, so we assert on the "unknown target" rejection alone.
func TestRun_TargetResolution(t *testing.T) {
	cfg := &config.Config{
		Overrides: map[string]map[string]any{
			"my-tool": {"image": "alpine:3.20", "command": "echo hi"},
			// Override-only section (no image): not a runnable target.
			"tweaks-only": {"severity": "HIGH"},
		},
		Workspace: t.TempDir(),
	}

	selector, err := executor.NewSelector(true, false, true) // dry-run executors
	if err != nil {
		t.Fatalf("failed to create selector: %v", err)
	}
	t.Cleanup(func() { _ = selector.Close() })

	runner := NewRunnerWithOptions(cfg, selector, RunnerOptions{
		Backend:     executor.BackendAuto,
		Concurrency: 1,
	})
	ctx := context.Background()

	// Declared custom container: resolved, never rejected as unknown.
	if err := runner.Run(ctx, "my-tool"); err != nil && strings.Contains(err.Error(), "unknown target") {
		t.Errorf("custom container declared via [containers.my-tool] was rejected: %v", err)
	}

	// Names the config does not declare still fail, and say so clearly.
	for _, target := range []string{"nope", "tweaks-only"} {
		err := runner.Run(ctx, target)
		if err == nil {
			t.Fatalf("Run(%q) should fail: nothing declares it", target)
		}
		if !strings.Contains(err.Error(), "unknown target") || !strings.Contains(err.Error(), target) {
			t.Errorf("Run(%q) error should name the unknown target, got: %v", target, err)
		}
	}
}

// TestPrintLocalSafetyDryRun_EnvironmentOrderIsStable is the local-safety
// counterpart of the executor dry-run: same map, same phantom-diff problem
// (issue #230).
func TestPrintLocalSafetyDryRun_EnvironmentOrderIsStable(t *testing.T) {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	r := &Runner{logger: logger}

	cfg := &config.ContainerConfig{
		Name:  "envy",
		Image: "alpine:latest",
		Env: map[string]string{
			"ZULU": "1", "ALPHA": "2", "MIKE": "3", "BRAVO": "4",
			"YANKEE": "5", "CHARLIE": "6", "DELTA": "7", "ECHO": "8",
		},
	}

	render := func() string {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		r.printLocalSafetyDryRun(cfg)
		return buf.String()
	}

	first := render()
	for i := range 20 {
		if again := render(); again != first {
			t.Fatalf("run %d differs on identical input:\n%s\n---\n%s", i, first, again)
		}
	}
	if !strings.Contains(first, "ALPHA=2") || strings.Index(first, "ALPHA=2") > strings.Index(first, "ZULU=1") {
		t.Errorf("environment should be printed in key order:\n%s", first)
	}
}
