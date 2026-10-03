package features

import (
	"fmt"
	"os"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cucumber/godog"
)

// Steps for features/executor/container_reuse.feature (#531). They compute the
// key with executor.ReuseKey, the call the executor labels a container with and
// compares on the next run, so a scenario cannot pass on a rule the executor
// does not apply.

// RegisterContainerReuseSteps registers the container reuse step definitions.
func RegisterContainerReuseSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a tool "([^"]*)" that runs as the host user$`, tc.toolRunsAsHostUser)
	ctx.Given(`^a tool "([^"]*)" that runs as the host user (\d+):(\d+)$`, tc.toolRunsAsHostUserID)
	ctx.Given(`^a privileged tool "([^"]*)"$`, tc.privilegedTool)
	ctx.When(`^the tool is switched to privileged$`, tc.switchToPrivileged)
	ctx.When(`^the tool is switched to run as the host user$`, tc.switchToHostUser)
	ctx.When(`^the same tool is run by the host user (\d+):(\d+)$`, tc.runByHostUser)
	ctx.When(`^the same tool is run under rootless Podman$`, tc.runUnderRootlessPodman)
	ctx.When(`^the same tool is run again$`, tc.runAgain)
	ctx.When(`^the tool's pull policy and timeout change$`, tc.changePullPolicyAndTimeout)
	ctx.Then(`^the container created before must not be reused$`, tc.containerMustNotBeReused)
	ctx.Then(`^the container created before is reused$`, tc.containerIsReused)
}

// reuseWorld is one scenario's view of a container: the configuration it was
// created from, who ran it, and the key it would be labelled with then and now.
type reuseWorld struct {
	cfg     config.ContainerConfig
	host    executor.HostIdentity
	created string
	now     string
}

func (tc *TestContext) reuse() *reuseWorld {
	w, ok := tc.Config["reuse"].(*reuseWorld)
	if !ok {
		w = &reuseWorld{host: executor.HostIdentity{UID: os.Getuid(), GID: os.Getgid()}}
		tc.Config["reuse"] = w
	}
	return w
}

func (w *reuseWorld) key() string {
	return executor.ReuseKey(&w.cfg, "tool --run", []string{"/work:/work"}, w.host)
}

// created records the container as it exists before the change under test.
func (w *reuseWorld) createContainer(name string, privileged bool) {
	w.cfg = config.ContainerConfig{Name: name, Image: "example/tool:1.0", Workdir: "/work", Privileged: privileged}
	w.created = w.key()
	w.now = w.created
}

func (tc *TestContext) toolRunsAsHostUser(name string) error {
	tc.reuse().createContainer(name, false)
	return nil
}

func (tc *TestContext) toolRunsAsHostUserID(name string, uid, gid int) error {
	w := tc.reuse()
	w.host = executor.HostIdentity{UID: uid, GID: gid}
	w.createContainer(name, false)
	return nil
}

func (tc *TestContext) privilegedTool(name string) error {
	tc.reuse().createContainer(name, true)
	return nil
}

func (tc *TestContext) switchToPrivileged() error {
	w := tc.reuse()
	w.cfg.Privileged = true
	w.now = w.key()
	return nil
}

func (tc *TestContext) switchToHostUser() error {
	w := tc.reuse()
	w.cfg.Privileged = false
	w.now = w.key()
	return nil
}

func (tc *TestContext) runByHostUser(uid, gid int) error {
	w := tc.reuse()
	w.host.UID, w.host.GID = uid, gid
	w.now = w.key()
	return nil
}

func (tc *TestContext) runUnderRootlessPodman() error {
	w := tc.reuse()
	w.host.Rootless = true
	w.now = w.key()
	return nil
}

func (tc *TestContext) runAgain() error {
	w := tc.reuse()
	w.now = w.key()
	return nil
}

// changePullPolicyAndTimeout changes the two fields that deliberately stay out
// of the key: they change how the tool is run, not the container it runs in.
func (tc *TestContext) changePullPolicyAndTimeout() error {
	w := tc.reuse()
	w.cfg.PullPolicy = "always"
	w.cfg.Timeout = "45m"
	w.now = w.key()
	return nil
}

func (tc *TestContext) containerMustNotBeReused() error {
	w := tc.reuse()
	if w.created == w.now {
		return fmt.Errorf("the container created before would be reused (key %s), but it was created as a different user", w.created)
	}
	return nil
}

func (tc *TestContext) containerIsReused() error {
	w := tc.reuse()
	if w.created != w.now {
		return fmt.Errorf("the container would be recreated (key %s -> %s), losing its caches for nothing", w.created, w.now)
	}
	return nil
}
