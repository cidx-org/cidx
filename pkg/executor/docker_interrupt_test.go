package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/docker/docker/api/types/container"
)

// A run that ends before its container does — Ctrl-C, SIGTERM, a timeout — must
// stop the container: it used to keep running, and writing into the workspace,
// after the CLI had returned (#535). Real Docker, skipped without a daemon.
func TestRunStopsTheContainerWhenTheContextEnds(t *testing.T) {
	e, err := NewDockerExecutor(false, false, true)
	if err != nil || !e.Available() {
		t.Skip("no Docker daemon reachable")
	}
	defer func() { _ = e.Close() }()

	cfg := &config.ContainerConfig{
		Name: "interrupt-probe", Image: "busybox:1.37", PullPolicy: "if-not-present",
		Command: "sleep 60", Workdir: "/tmp",
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, cfg) }()

	var id string
	for deadline := time.Now().Add(60 * time.Second); id == "" && time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if c, _ := e.findManagedContainer(context.Background(), e.containerName(cfg)); c != nil && c.State == "running" {
			id = c.ID
		}
	}
	if id == "" {
		t.Fatal("the container never started")
	}
	t.Cleanup(func() {
		_ = e.client.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true})
	})

	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "interrupted") {
			t.Fatalf("an interrupted run must fail as interrupted, got %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after the context ended")
	}
	info, err := e.client.ContainerInspect(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Running {
		t.Fatal("the container is still running after the run was interrupted")
	}
}
