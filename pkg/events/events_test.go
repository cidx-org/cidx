package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/internal/natstest"
	"github.com/cidx-org/cidx/v3/pkg/executor"
)

// warnings collects what the publisher says, safe for concurrent emitters.
type warnings struct {
	mu   sync.Mutex
	msgs []string
}

func (w *warnings) add(m string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.msgs = append(w.msgs, m)
}

func (w *warnings) all() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.msgs...)
}

func TestNoAddressMeansNoPublisher(t *testing.T) {
	for _, url := range []string{"", "   "} {
		w := &warnings{}
		got := Connect(Settings{URL: url}, w.add)
		if _, ok := got.(Nop); !ok {
			t.Errorf("Connect(%q) = %T, want Nop", url, got)
		}
		got.Emit(Event{Type: RunStarted})
		got.Close()
		if len(w.all()) != 0 {
			t.Errorf("a run without an address warned: %v", w.all())
		}
	}
}

func TestEventsArePublishedOnTheSubject(t *testing.T) {
	srv, err := natstest.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	w := &warnings{}
	em := Connect(Settings{URL: srv.URL(), Subject: "ci.events", RunID: "run-42"}, w.add)
	em.Emit(Event{Type: RunStarted, Target: "ci"})
	em.Emit(Finish(Event{Type: ContainerFinished, Container: "trivy", Image: "img:1"}, time.Now(), &executor.ExitError{Code: 3}))
	em.Close()

	msgs := srv.WaitFor(2, 2*time.Second)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	for _, m := range msgs {
		if m.Subject != "ci.events" {
			t.Errorf("subject = %q", m.Subject)
		}
	}

	var first, second Event
	if err := json.Unmarshal(msgs[0].Payload, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(msgs[1].Payload, &second); err != nil {
		t.Fatal(err)
	}
	if first.Format != Format || first.RunID != "run-42" || first.Type != RunStarted || first.Target != "ci" || first.Time.IsZero() {
		t.Errorf("run.started = %+v", first)
	}
	if second.RunID != "run-42" || second.Container != "trivy" || second.Image != "img:1" || second.Status != StatusFailed {
		t.Errorf("container.finished = %+v", second)
	}
	if second.ExitCode == nil || *second.ExitCode != 3 || second.DurationMS == nil {
		t.Errorf("exit code and duration missing: %+v", second)
	}
	if len(w.all()) != 0 {
		t.Errorf("a healthy bus warned: %v", w.all())
	}
}

func TestDefaultsAndGeneratedRunID(t *testing.T) {
	srv, err := natstest.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	em := Connect(Settings{URL: srv.URL()}, nil)
	em.Emit(Event{Type: RunStarted})
	em.Emit(Event{Type: RunFinished})
	em.Close()

	msgs := srv.WaitFor(2, 2*time.Second)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Subject != DefaultSubject {
		t.Errorf("subject = %q, want %q", msgs[0].Subject, DefaultSubject)
	}
	var a, b Event
	_ = json.Unmarshal(msgs[0].Payload, &a)
	_ = json.Unmarshal(msgs[1].Payload, &b)
	if a.RunID == "" || a.RunID != b.RunID {
		t.Errorf("run ids = %q, %q: want one shared non-empty id", a.RunID, b.RunID)
	}
}

func TestUnreachableBusWarnsOnceWithoutLeakingCredentials(t *testing.T) {
	srv, err := natstest.Start()
	if err != nil {
		t.Fatal(err)
	}
	addr := strings.TrimPrefix(srv.URL(), "nats://")
	srv.Close()

	url := "nats://svc-account:hunter2-s3cret@" + addr
	w := &warnings{}
	em := Connect(Settings{URL: url}, w.add)
	for range 5 {
		em.Emit(Event{Type: PhaseStarted})
	}
	em.Close()

	got := w.all()
	if len(got) != 1 {
		t.Fatalf("warnings = %v, want exactly one", got)
	}
	for _, secret := range []string{"hunter2", "svc-account", addr, "nats://"} {
		if strings.Contains(got[0], secret) {
			t.Errorf("warning leaks %q: %s", secret, got[0])
		}
	}
}

func TestRefusedPublicationWarnsOnce(t *testing.T) {
	srv, err := natstest.StartRefusing()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	w := &warnings{}
	em := Connect(Settings{URL: srv.URL()}, w.add)
	for range 20 {
		em.Emit(Event{Type: PhaseStarted})
		time.Sleep(time.Millisecond)
	}
	em.Close()

	if got := w.all(); len(got) != 1 {
		t.Fatalf("warnings = %v, want exactly one", got)
	}
}

func TestFinish(t *testing.T) {
	start := time.Now().Add(-1500 * time.Millisecond)

	passed := Finish(Event{Type: PhaseFinished}, start, nil)
	if passed.Status != StatusPassed || passed.ExitCode == nil || *passed.ExitCode != 0 {
		t.Errorf("passed = %+v", passed)
	}
	if passed.DurationMS == nil || *passed.DurationMS < 1500 {
		t.Errorf("duration = %v", passed.DurationMS)
	}

	wrapped := fmt.Errorf("container x failed: %w", &executor.ExitError{Code: 7})
	failed := Finish(Event{Type: PhaseFinished}, start, wrapped)
	if failed.Status != StatusFailed || failed.ExitCode == nil || *failed.ExitCode != 7 {
		t.Errorf("failed = %+v", failed)
	}

	noExit := Finish(Event{Type: ContainerFinished}, start, errors.New("failed to pull image"))
	if noExit.Status != StatusFailed || noExit.ExitCode != nil {
		t.Errorf("a failure without an exit code must not invent one: %+v", noExit)
	}
	raw, _ := json.Marshal(noExit)
	if strings.Contains(string(raw), "pull") || strings.Contains(string(raw), "exit_code") {
		t.Errorf("error text or exit_code in payload: %s", raw)
	}
}
