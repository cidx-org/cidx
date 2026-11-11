package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cidx-org/cidx/v3/internal/natstest"
	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/events"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/pipeline"
	"github.com/cucumber/godog"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// phaseExecutor stands for the container runtime: each phase's container can be
// told to exit with a code or to print a line.
type phaseExecutor struct {
	exits  map[string]int64
	prints map[string]string
}

func (e *phaseExecutor) Run(_ context.Context, cfg *config.ContainerConfig) error {
	if line := e.prints[cfg.Phase]; line != "" {
		_, _ = fmt.Fprintln(os.Stdout, line)
	}
	if code := e.exits[cfg.Phase]; code != 0 {
		return &executor.ExitError{Code: code}
	}
	return nil
}
func (*phaseExecutor) Available() bool { return true }
func (*phaseExecutor) Name() string    { return "phase-executor" }
func (*phaseExecutor) Close() error    { return nil }

// natsWorld is the state of one events scenario.
type natsWorld struct {
	srv       *natstest.Server
	url       string
	user      string
	password  string
	pipeline  string
	phases    []string
	exec      *phaseExecutor
	secrets   map[string]string // phase -> secret held by its container
	runErr    error
	output    string
	flagsOf   *cli.Command
	published []events.Event
}

func RegisterExecutionEventSteps(ctx *godog.ScenarioContext, _ *TestContext) {
	w := &natsWorld{
		exec:    &phaseExecutor{exits: map[string]int64{}, prints: map[string]string{}},
		secrets: map[string]string{},
	}

	ctx.After(func(c context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.srv != nil {
			w.srv.Close()
		}
		return c, nil
	})

	ctx.Given(`^a NATS server$`, w.startServer)
	ctx.Given(`^a NATS address where nothing listens$`, w.deadAddress)
	ctx.Given(`^the connection credentials "([^"]*)" and "([^"]*)"$`, func(user, pw string) error {
		w.user, w.password = user, pw
		return nil
	})
	ctx.Given(`^a pipeline "([^"]*)" with the phases "([^"]*)"$`, w.declarePipeline)
	ctx.Given(`^the container of the phase "([^"]*)" exits with code (\d+)$`, func(phase string, code int) error {
		w.exec.exits[phase] = int64(code)
		return nil
	})
	ctx.Given(`^the container of the phase "([^"]*)" prints "([^"]*)" and holds the secret "([^"]*)"$`, func(phase, line, secret string) error {
		w.exec.prints[phase] = line
		w.secrets[phase] = secret
		return nil
	})

	ctx.When(`^the pipeline "([^"]*)" is run publishing to the server as run "([^"]*)"$`, func(name, runID string) error {
		return w.run(name, events.Settings{URL: w.url, RunID: runID})
	})
	ctx.When(`^the pipeline "([^"]*)" is run publishing under the subject prefix "([^"]*)"$`, func(name, prefix string) error {
		return w.run(name, events.Settings{URL: w.url, SubjectPrefix: prefix})
	})
	ctx.When(`^the pipeline "([^"]*)" is run publishing to the server without a run id$`, func(name string) error {
		return w.run(name, events.Settings{URL: w.url})
	})
	ctx.When(`^the pipeline "([^"]*)" is run publishing to that address$`, func(name string) error {
		return w.run(name, events.Settings{URL: w.url})
	})
	ctx.When(`^the pipeline "([^"]*)" is run publishing to the server with those credentials$`, func(name string) error {
		url := strings.Replace(w.url, "nats://", "nats://"+w.user+":"+w.password+"@", 1)
		return w.run(name, events.Settings{URL: url, Token: w.password})
	})
	ctx.When(`^the pipeline "([^"]*)" is run without a NATS address$`, func(name string) error {
		return w.run(name, events.Settings{})
	})

	ctx.Then(`^the run should succeed$`, func() error {
		if w.runErr != nil {
			return fmt.Errorf("the run failed: %w\n%s", w.runErr, w.output)
		}
		return nil
	})
	ctx.Then(`^the run should fail$`, func() error {
		if w.runErr == nil {
			return errors.New("the run succeeded")
		}
		return nil
	})
	ctx.Then(`^the run should fail with the exit code of the container, (\d+)$`, func(want int) error {
		var exit *executor.ExitError
		if !errors.As(w.runErr, &exit) || exit.Code != int64(want) {
			return fmt.Errorf("run error = %v, want a container exit %d", w.runErr, want)
		}
		return nil
	})

	ctx.Then(`^the server should have received the events "([^"]*)"$`, w.receivedEvents)
	ctx.Then(`^every event should be format "([^"]*)" and carry the run id "([^"]*)"$`, w.everyEventHas)
	ctx.Then(`^every event should carry the same non-empty run id$`, w.sameRunID)
	ctx.Then(`^the event "([^"]*)" for "([^"]*)" should name an image, the exit code 0 and a duration$`, w.containerComplete)
	ctx.Then(`^the event "([^"]*)" for "([^"]*)" should have the status "([^"]*)" and the exit code (\d+)$`, w.containerStatus)
	ctx.Then(`^the event "run.finished" should have the status "([^"]*)" and the exit code (\d+)$`, w.runStatus)
	ctx.Then(`^the server should have received a message on "([^"]*)"$`, w.messageOn)
	ctx.Then(`^the server should have been neither contacted nor sent anything$`, func() error {
		if n := w.srv.Accepted(); n != 0 {
			return fmt.Errorf("%d connection(s) attempted", n)
		}
		if n := len(w.srv.Messages()); n != 0 {
			return fmt.Errorf("%d message(s) received", n)
		}
		return nil
	})
	ctx.Then(`^the output should not mention NATS$`, func() error {
		if strings.Contains(strings.ToLower(w.output), "nats") {
			return fmt.Errorf("the output mentions NATS:\n%s", w.output)
		}
		return nil
	})
	ctx.Then(`^the output should warn once that NATS events are disabled$`, func() error {
		if n := strings.Count(w.output, "NATS events disabled"); n != 1 {
			return fmt.Errorf("want one warning, got %d:\n%s", n, w.output)
		}
		return nil
	})
	ctx.Then(`^neither the output nor any event should contain "([^"]*)"$`, func(secret string) error {
		if strings.Contains(w.output, secret) {
			return fmt.Errorf("the output contains %q", secret)
		}
		return w.noEventContains(secret)
	})
	ctx.Then(`^no event should contain "([^"]*)"$`, w.noEventContains)

	ctx.When(`^I inspect the flags of "cidx run"$`, func() error {
		for _, cmd := range commands.NewApp().Commands {
			if cmd.Name == "run" {
				w.flagsOf = cmd
				return nil
			}
		}
		return errors.New("the CLI has no run command")
	})
	ctx.Then(`^"cidx run" should accept "--([^"]*)" from "([^"]*)"$`, func(name, env string) error {
		for _, f := range w.flagsOf.Flags {
			if sf, ok := f.(*cli.StringFlag); ok && sf.Name == name {
				if !slices.Contains(sf.EnvVars, env) {
					return fmt.Errorf("--%s does not read %s (reads %v)", name, env, sf.EnvVars)
				}
				return nil
			}
		}
		return fmt.Errorf("cidx run has no --%s flag", name)
	})
}

func (w *natsWorld) startServer() error {
	srv, err := natstest.Start()
	if err != nil {
		return err
	}
	w.srv, w.url = srv, srv.URL()
	return nil
}

func (w *natsWorld) deadAddress() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	w.url = "nats://" + ln.Addr().String()
	return ln.Close()
}

func (w *natsWorld) declarePipeline(name, phases string) error {
	w.pipeline = name
	w.phases = nil
	for _, p := range strings.Split(phases, ",") {
		w.phases = append(w.phases, strings.TrimSpace(p))
	}
	return nil
}

func (w *natsWorld) run(name string, s events.Settings) error {
	cfg := &config.Config{
		Phases:    map[string]config.Phase{},
		Pipelines: map[string]config.Pipeline{name: {Phases: w.phases}},
		Overrides: map[string]map[string]any{},
	}
	for _, phase := range w.phases {
		container := "scenario-" + phase
		cfg.Phases[phase] = config.Phase{Containers: []string{container}}
		override := map[string]any{"image": "scenario/image:fixture", "command": "true", "phase": phase}
		if secret := w.secrets[phase]; secret != "" {
			override["env"] = map[string]any{"API_TOKEN": secret}
		}
		cfg.Overrides[container] = override
	}

	done, err := captureStderr()
	if err != nil {
		return err
	}
	out, runErr := captureOutput(func() error {
		emitter, _ := events.Open(s, logrus.New())
		defer emitter.Close()
		runner := pipeline.NewRunnerWithOptions(cfg, fixedExecutorSelector{backend: w.exec},
			pipeline.RunnerOptions{Backend: executor.BackendDocker, Events: emitter})
		return runner.Run(context.Background(), name)
	})
	w.output, w.runErr = done()+out, runErr

	if w.srv != nil {
		w.published = w.published[:0]
		for _, m := range w.srv.Messages() {
			var e events.Event
			if err := json.Unmarshal(m.Data, &e); err != nil {
				return fmt.Errorf("event on %s is not JSON: %w", m.Subject, err)
			}
			w.published = append(w.published, e)
		}
	}
	return nil
}

func (w *natsWorld) receivedEvents(list string) error {
	var want []string
	for _, t := range strings.Split(list, ",") {
		want = append(want, strings.TrimSpace(t))
	}
	var got []string
	for _, e := range w.published {
		got = append(got, e.Type)
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("events = %v\nwant     %v", got, want)
	}
	return nil
}

func (w *natsWorld) everyEventHas(format, runID string) error {
	if len(w.published) == 0 {
		return errors.New("no event was published")
	}
	for _, e := range w.published {
		if e.Format != format || e.RunID != runID {
			return fmt.Errorf("event %s has format %q, run id %q", e.Type, e.Format, e.RunID)
		}
	}
	return nil
}

func (w *natsWorld) sameRunID() error {
	if len(w.published) == 0 {
		return errors.New("no event was published")
	}
	id := w.published[0].RunID
	for _, e := range w.published {
		if id == "" || e.RunID != id {
			return fmt.Errorf("run ids differ or are empty: %q vs %q", id, e.RunID)
		}
	}
	return nil
}

func (w *natsWorld) find(eventType, container string) (events.Event, error) {
	for _, e := range w.published {
		if e.Type == eventType && e.Container == container {
			return e, nil
		}
	}
	return events.Event{}, fmt.Errorf("no %q event for %q", eventType, container)
}

func (w *natsWorld) containerComplete(eventType, container string) error {
	e, err := w.find(eventType, container)
	if err != nil {
		return err
	}
	if e.Image == "" || e.ExitCode == nil || *e.ExitCode != 0 || e.DurationMS == nil {
		return fmt.Errorf("incomplete event: %+v", e)
	}
	return nil
}

func (w *natsWorld) containerStatus(eventType, container, status string, code int) error {
	e, err := w.find(eventType, container)
	if err != nil {
		return err
	}
	if e.Status != status || e.ExitCode == nil || *e.ExitCode != code {
		return fmt.Errorf("event = %+v, want status %s and exit code %d", e, status, code)
	}
	return nil
}

func (w *natsWorld) runStatus(status string, code int) error {
	e, err := w.find(events.RunFinished, "")
	if err != nil {
		return err
	}
	if e.Status != status || e.ExitCode == nil || *e.ExitCode != code {
		return fmt.Errorf("run.finished = %+v, want status %s and exit code %d", e, status, code)
	}
	return nil
}

func (w *natsWorld) messageOn(subject string) error {
	for _, m := range w.srv.Messages() {
		if m.Subject == subject {
			return nil
		}
	}
	return fmt.Errorf("nothing was published on %s", subject)
}

func (w *natsWorld) noEventContains(text string) error {
	if w.srv == nil {
		return errors.New("no server")
	}
	for _, m := range w.srv.Messages() {
		if strings.Contains(string(m.Data), text) {
			return fmt.Errorf("event on %s contains %q: %s", m.Subject, text, m.Data)
		}
	}
	return nil
}
