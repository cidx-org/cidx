package pipeline

import (
	"testing"

	"github.com/cidx-org/cidx/pkg/executor"
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
