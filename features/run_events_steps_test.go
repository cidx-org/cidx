package features

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/natstest"
	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/events"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/pipeline"
	"github.com/cucumber/godog"
)

// The addresses and credentials the "nothing is listening" scenarios use. The
// scenarios assert none of it ever reaches a warning.
const (
	runEventsUser  = "deploy"
	runEventsToken = "s3cr3t-token"
)

// documentedEventFields is the contract of docs/reference/events.md.
var documentedEventFields = []string{
	"format", "type", "run_id", "time", "target", "phase", "container",
	"image", "status", "exit_code", "duration_ms",
}

// runEventsState is one scenario's world: a test NATS server (or the address
// where one used to be), the pipeline to run and what came out of it.
type runEventsState struct {
	srv      *natstest.Server
	url      string
	addr     string
	phases   []string
	exits    map[string]int
	secrets  map[string]string
	runErr   error
	warnings []string
	ran      bool
}

// exitingExecutor passes every container except those scripted to exit non-zero.
type exitingExecutor struct{ exits map[string]int }

func (e exitingExecutor) Run(_ context.Context, cfg *config.ContainerConfig) error {
	if code, ok := e.exits[cfg.Name]; ok {
		return &executor.ExitError{Code: int64(code)}
	}
	return nil
}
func (exitingExecutor) Available() bool { return true }
func (exitingExecutor) Name() string    { return "exiting" }
func (exitingExecutor) Close() error    { return nil }

func (tc *TestContext) runEventsWorld() *runEventsState {
	if tc.runEvents == nil {
		tc.runEvents = &runEventsState{exits: map[string]int{}, secrets: map[string]string{}}
	}
	return tc.runEvents
}

func RegisterRunEventsSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a NATS server is listening$`, tc.natsServerListening)
	ctx.Given(`^a NATS address with credentials where nothing is listening$`, tc.natsAddressNothingListening)
	ctx.Given(`^a run-events pipeline with the phases "([^"]*)"$`, tc.runEventsPipeline)
	ctx.Given(`^the run-events container "([^"]*)" exits with code (\d+)$`, tc.runEventsContainerExits)
	ctx.Given(`^the run-events container "([^"]*)" holds the secret "([^"]*)"$`, tc.runEventsContainerHoldsSecret)

	ctx.When(`^I run the pipeline with run events published$`, func() error {
		return tc.runWithEvents(true, events.Settings{})
	})
	ctx.When(`^I run the pipeline with run events published as run "([^"]*)" on subject "([^"]*)"$`,
		func(runID, subject string) error {
			return tc.runWithEvents(true, events.Settings{RunID: runID, Subject: subject})
		})
	ctx.When(`^I run the pipeline without a NATS address$`, func() error {
		return tc.runWithEvents(false, events.Settings{})
	})

	ctx.Then(`^the run events should be, in order:$`, tc.runEventsInOrder)
	ctx.Then(`^every run event should carry the format "([^"]*)"$`, tc.everyEventCarriesFormat)
	ctx.Then(`^every run event should share one run id$`, tc.everyEventSharesRunID)
	ctx.Then(`^every run event should carry the run id "([^"]*)"$`, tc.everyEventCarriesRunID)
	ctx.Then(`^every run event should be on the subject "([^"]*)"$`, tc.everyEventOnSubject)
	ctx.Then(`^the finished container events should carry the image, the exit code and the duration$`, tc.finishedContainersAreComplete)
	ctx.Then(`^the run\.finished event should be "([^"]*)" with exit code (\d+)$`, tc.runFinishedIs)
	ctx.Then(`^no event should mention the phase "([^"]*)"$`, tc.noEventMentionsPhase)
	ctx.Then(`^the run should pass$`, func() error { return tc.runOutcome(false) })
	ctx.Then(`^the run should fail$`, func() error { return tc.runOutcome(true) })
	ctx.Then(`^the run exit code should be (\d+)$`, tc.runExitCodeIs)
	ctx.Then(`^the NATS server should have received no connection$`, tc.natsReceivedNoConnection)
	ctx.Then(`^no run-events warning should be printed$`, func() error { return tc.runEventsWarningCount(0) })
	ctx.Then(`^exactly one run-events warning should be printed$`, func() error { return tc.runEventsWarningCount(1) })
	ctx.Then(`^the run-events warning should not contain the address or its credentials$`, tc.warningLeaksNothing)
	ctx.Then(`^no run event should contain "([^"]*)"$`, tc.noEventContains)
	ctx.Then(`^no run event should have a field other than the documented ones$`, tc.onlyDocumentedFields)
}

func (tc *TestContext) natsServerListening() error {
	w := tc.runEventsWorld()
	srv, err := natstest.Start()
	if err != nil {
		return err
	}
	w.srv = srv
	w.url = srv.URL()
	return nil
}

func (tc *TestContext) natsAddressNothingListening() error {
	w := tc.runEventsWorld()
	srv, err := natstest.Start()
	if err != nil {
		return err
	}
	w.addr = strings.TrimPrefix(srv.URL(), "nats://")
	srv.Close()
	w.url = "nats://" + runEventsUser + ":" + runEventsToken + "@" + w.addr
	return nil
}

func (tc *TestContext) runEventsPipeline(phases string) error {
	tc.runEventsWorld().phases = strings.Split(phases, ",")
	return nil
}

func (tc *TestContext) runEventsContainerExits(name, code string) error {
	n, err := strconv.Atoi(code)
	if err != nil {
		return err
	}
	tc.runEventsWorld().exits[name] = n
	return nil
}

func (tc *TestContext) runEventsContainerHoldsSecret(name, secret string) error {
	tc.runEventsWorld().secrets[name] = secret
	return nil
}

func (tc *TestContext) runWithEvents(publish bool, settings events.Settings) error {
	w := tc.runEventsWorld()
	settings.URL = w.url

	cfg := &config.Config{
		Phases:    map[string]config.Phase{},
		Pipelines: map[string]config.Pipeline{"ci": {Phases: w.phases}},
		Overrides: map[string]map[string]any{},
	}
	for _, phase := range w.phases {
		name := "scenario-" + phase
		cfg.Phases[phase] = config.Phase{Containers: []string{name}}
		override := map[string]any{"image": "scenario/image:fixture", "command": "true", "phase": phase}
		if secret, ok := w.secrets[name]; ok {
			override["env"] = map[string]any{"API_TOKEN": secret}
		}
		cfg.Overrides[name] = override
	}

	var emitter events.Emitter = events.Nop{}
	if publish {
		emitter = events.Connect(settings, func(msg string) { w.warnings = append(w.warnings, msg) })
	}
	runner := pipeline.NewRunnerWithOptions(cfg, fixedExecutorSelector{backend: exitingExecutor{exits: w.exits}},
		pipeline.RunnerOptions{Backend: executor.BackendDocker, Events: emitter})
	w.runErr = runner.Run(context.Background(), "ci")
	emitter.Close()
	w.ran = true
	return nil
}

// received decodes what the test server got. Close flushed the emitter, and the
// server answers a flush only after recording what came before it, so the list
// is complete once the run is over.
func (tc *TestContext) received() ([]events.Event, []natstest.Message, error) {
	w := tc.runEventsWorld()
	if !w.ran {
		return nil, nil, fmt.Errorf("the pipeline was not run")
	}
	if w.srv == nil {
		return nil, nil, fmt.Errorf("no NATS server is listening in this scenario")
	}
	msgs := w.srv.Messages()
	got := make([]events.Event, 0, len(msgs))
	for _, m := range msgs {
		var e events.Event
		if err := json.Unmarshal(m.Payload, &e); err != nil {
			return nil, nil, fmt.Errorf("an event is not JSON: %w (%s)", err, m.Payload)
		}
		got = append(got, e)
	}
	return got, msgs, nil
}

func (tc *TestContext) runEventsInOrder(table *godog.Table) error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	rows := table.Rows[1:] // header
	var want, have []string
	for _, row := range rows {
		want = append(want, fmt.Sprintf("%s|%s|%s", row.Cells[0].Value, row.Cells[1].Value, row.Cells[2].Value))
	}
	for _, e := range got {
		have = append(have, fmt.Sprintf("%s|%s|%s", e.Type, e.Phase, e.Container))
	}
	if !slices.Equal(want, have) {
		return fmt.Errorf("events were\n  %s\nwant\n  %s", strings.Join(have, "\n  "), strings.Join(want, "\n  "))
	}
	return nil
}

func (tc *TestContext) everyEventCarriesFormat(format string) error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	if len(got) == 0 {
		return fmt.Errorf("no event was published")
	}
	for _, e := range got {
		if e.Format != format {
			return fmt.Errorf("%s carries format %q, want %q", e.Type, e.Format, format)
		}
	}
	return nil
}

func (tc *TestContext) everyEventSharesRunID() error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	if len(got) == 0 || got[0].RunID == "" {
		return fmt.Errorf("events carry no run id")
	}
	return tc.everyEventCarriesRunID(got[0].RunID)
}

func (tc *TestContext) everyEventCarriesRunID(id string) error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	if len(got) == 0 {
		return fmt.Errorf("no event was published")
	}
	for _, e := range got {
		if e.RunID != id {
			return fmt.Errorf("%s carries run id %q, want %q", e.Type, e.RunID, id)
		}
	}
	return nil
}

func (tc *TestContext) everyEventOnSubject(subject string) error {
	_, msgs, err := tc.received()
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return fmt.Errorf("no event was published")
	}
	for _, m := range msgs {
		if m.Subject != subject {
			return fmt.Errorf("event published on %q, want %q", m.Subject, subject)
		}
	}
	return nil
}

func (tc *TestContext) finishedContainersAreComplete() error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	seen := 0
	for _, e := range got {
		if e.Type != events.ContainerFinished {
			continue
		}
		seen++
		if e.Image == "" || e.ExitCode == nil || e.DurationMS == nil || e.Status == "" {
			return fmt.Errorf("incomplete container.finished: %+v", e)
		}
	}
	if seen == 0 {
		return fmt.Errorf("no container.finished event")
	}
	return nil
}

func (tc *TestContext) runFinishedIs(status, code string) error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	want, _ := strconv.Atoi(code)
	for _, e := range got {
		if e.Type != events.RunFinished {
			continue
		}
		if e.Status != status || e.ExitCode == nil || *e.ExitCode != want {
			return fmt.Errorf("run.finished = %+v, want %s with exit code %d", e, status, want)
		}
		return nil
	}
	return fmt.Errorf("no run.finished event")
}

func (tc *TestContext) noEventMentionsPhase(phase string) error {
	got, _, err := tc.received()
	if err != nil {
		return err
	}
	for _, e := range got {
		if e.Phase == phase || e.Container == "scenario-"+phase {
			return fmt.Errorf("%s mentions %q although the run stopped before it", e.Type, phase)
		}
	}
	return nil
}

func (tc *TestContext) runOutcome(wantFailure bool) error {
	w := tc.runEventsWorld()
	if !w.ran {
		return fmt.Errorf("the pipeline was not run")
	}
	if wantFailure && w.runErr == nil {
		return fmt.Errorf("the run passed, expected it to fail")
	}
	if !wantFailure && w.runErr != nil {
		return fmt.Errorf("the run failed: %v", w.runErr)
	}
	return nil
}

func (tc *TestContext) runExitCodeIs(code string) error {
	want, _ := strconv.Atoi(code)
	got, ok := events.ExitCode(tc.runEventsWorld().runErr)
	if !ok || got != want {
		return fmt.Errorf("exit code = %d (known: %v), want %d", got, ok, want)
	}
	return nil
}

func (tc *TestContext) natsReceivedNoConnection() error {
	w := tc.runEventsWorld()
	if w.srv == nil {
		return fmt.Errorf("no NATS server is listening in this scenario")
	}
	if n := w.srv.Connections(); n != 0 {
		return fmt.Errorf("%d connection(s) were opened, expected none", n)
	}
	return nil
}

func (tc *TestContext) runEventsWarningCount(want int) error {
	if got := tc.runEventsWorld().warnings; len(got) != want {
		return fmt.Errorf("warnings = %v, want %d", got, want)
	}
	return nil
}

func (tc *TestContext) warningLeaksNothing() error {
	w := tc.runEventsWorld()
	for _, msg := range w.warnings {
		for _, secret := range []string{runEventsUser, runEventsToken, w.addr} {
			if strings.Contains(msg, secret) {
				return fmt.Errorf("warning %q contains %q", msg, secret)
			}
		}
	}
	return nil
}

func (tc *TestContext) noEventContains(text string) error {
	_, msgs, err := tc.received()
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return fmt.Errorf("no event was published")
	}
	for _, m := range msgs {
		if strings.Contains(string(m.Payload), text) {
			return fmt.Errorf("an event contains %q: %s", text, m.Payload)
		}
	}
	return nil
}

func (tc *TestContext) onlyDocumentedFields() error {
	_, msgs, err := tc.received()
	if err != nil {
		return err
	}
	for _, m := range msgs {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(m.Payload, &fields); err != nil {
			return err
		}
		for name := range fields {
			if !slices.Contains(documentedEventFields, name) {
				return fmt.Errorf("undocumented field %q in %s", name, m.Payload)
			}
		}
	}
	return nil
}

// closeRunEvents stops the scenario's test server.
func (tc *TestContext) closeRunEvents() {
	if tc.runEvents != nil && tc.runEvents.srv != nil {
		tc.runEvents.srv.Close()
	}
	tc.runEvents = nil
}
