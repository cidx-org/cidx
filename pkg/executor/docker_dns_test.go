package executor

import (
	"context"
	"slices"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/docker/docker/api/types/container"
)

// The DNS servers a container declares must reach Docker's HostConfig.DNS, and a
// container that declares none must leave it empty so the runtime default holds
// (#543). Real Docker, skipped without a daemon.
func TestRunPassesTheDeclaredDNSServersToDocker(t *testing.T) {
	e, err := NewDockerExecutor(false, false, true)
	if err != nil || !e.Available() {
		t.Skip("no Docker daemon reachable")
	}
	defer func() { _ = e.Close() }()

	for _, tc := range []struct {
		name string
		dns  []string
	}{
		{"dns-probe-set", []string{"1.1.1.1", "9.9.9.9"}},
		{"dns-probe-default", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.ContainerConfig{
				Name: tc.name, Image: "busybox:1.37", PullPolicy: "if-not-present",
				Command: "true", Workdir: "/tmp", DNS: tc.dns,
			}
			if err := e.Run(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			c, err := e.findManagedContainer(context.Background(), e.containerName(cfg))
			if err != nil || c == nil {
				t.Fatalf("container not found: %v", err)
			}
			t.Cleanup(func() {
				_ = e.client.ContainerRemove(context.Background(), c.ID, container.RemoveOptions{Force: true})
			})
			info, err := e.client.ContainerInspect(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(info.HostConfig.DNS, tc.dns) {
				t.Fatalf("HostConfig.DNS = %v, want %v", info.HostConfig.DNS, tc.dns)
			}
		})
	}
}
