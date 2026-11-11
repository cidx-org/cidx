package events

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/internal/natstest"
	"github.com/sirupsen/logrus"
)

func quietLogger() (*logrus.Logger, *bytes.Buffer) {
	var out bytes.Buffer
	l := logrus.New()
	l.SetOutput(&out)
	l.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})
	return l, &out
}

func startServer(t *testing.T) *natstest.Server {
	t.Helper()
	srv, err := natstest.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv
}

func TestEventsArePublishedAsVersionedJSON(t *testing.T) {
	srv := startServer(t)
	logger, _ := quietLogger()

	em, runID := Open(Settings{URL: srv.URL(), SubjectPrefix: "ci.demo", RunID: "run-42"}, logger)
	if runID != "run-42" {
		t.Fatalf("the caller's run id is not kept: %q", runID)
	}
	em.Emit(Event{Type: ContainerFinished, Phase: "test", Container: "go-test", Image: "golang:1", Status: StatusFailed, ExitCode: Code(3), DurationMS: Millis(1500 * time.Millisecond)})
	em.Close()

	msgs := srv.Messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].Subject != "ci.demo.container.finished" {
		t.Errorf("subject = %q", msgs[0].Subject)
	}
	var got Event
	if err := json.Unmarshal(msgs[0].Data, &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if got.Format != Format || got.RunID != "run-42" || got.Container != "go-test" || got.Image != "golang:1" {
		t.Errorf("unexpected event: %+v", got)
	}
	if got.ExitCode == nil || *got.ExitCode != 3 || got.DurationMS == nil || *got.DurationMS != 1500 {
		t.Errorf("exit code or duration lost: %+v", got)
	}
	if got.Time.IsZero() {
		t.Error("event has no time")
	}
}

func TestRunIDIsGeneratedWhenTheCallerGivesNone(t *testing.T) {
	_, a := Open(Settings{}, logrus.New())
	_, b := Open(Settings{}, logrus.New())
	if a == "" || a == b {
		t.Errorf("run ids must be non-empty and distinct: %q, %q", a, b)
	}
}

func TestNoAddressMeansNoConnection(t *testing.T) {
	// A listener that fails the test if anyone dials it, with no address given.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	dialled := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			dialled <- struct{}{}
			_ = c.Close()
		}
	}()

	logger, out := quietLogger()
	em, _ := Open(Settings{SubjectPrefix: "x"}, logger)
	em.Emit(Event{Type: RunStarted})
	em.Close()

	if _, ok := em.(Nop); !ok {
		t.Errorf("without an address the emitter must be Nop, got %T", em)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be logged without an address:\n%s", out)
	}
	select {
	case <-dialled:
		t.Error("a connection was attempted")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestUnreachableBrokerWarnsOnceAndGivesNop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nobody listens there any more

	logger, out := quietLogger()
	em, _ := Open(Settings{URL: "nats://" + addr}, logger)
	em.Emit(Event{Type: RunStarted})
	em.Emit(Event{Type: RunFinished})
	em.Close()

	if _, ok := em.(Nop); !ok {
		t.Fatalf("an unreachable broker must give Nop, got %T", em)
	}
	if n := strings.Count(out.String(), "NATS events disabled"); n != 1 {
		t.Errorf("want exactly one warning, got %d:\n%s", n, out)
	}
}

func TestRefusedPublicationWarnsOnce(t *testing.T) {
	srv := startServer(t)
	logger, out := quietLogger()

	em, _ := Open(Settings{URL: srv.URL()}, logger)
	n := em.(*natsEmitter)
	n.conn.Close() // every publication is now refused

	for range 5 {
		em.Emit(Event{Type: PhaseStarted, Phase: "code"})
	}
	em.Close()

	if got := strings.Count(out.String(), "NATS events disabled"); got != 1 {
		t.Errorf("want exactly one warning, got %d:\n%s", got, out)
	}
}

func TestCredentialsNeverLeakIntoEventsOrWarnings(t *testing.T) {
	srv := startServer(t)
	logger, out := quietLogger()

	const token = "s3cr3t-token-value"
	url := strings.Replace(srv.URL(), "nats://", "nats://alice:hunter2pw@", 1)
	em, _ := Open(Settings{URL: url, Token: token}, logger)
	em.Emit(Event{Type: RunStarted, Target: "ci"})
	em.Close()

	if len(srv.Messages()) != 1 {
		t.Fatalf("expected the event to be published, got %d", len(srv.Messages()))
	}
	all := out.String() + string(srv.Messages()[0].Data)
	for _, secret := range []string{token, "hunter2pw", "alice"} {
		if strings.Contains(all, secret) {
			t.Errorf("%q leaked into the output or the event:\n%s", secret, all)
		}
	}
}

func TestRedactAndSecretsOf(t *testing.T) {
	got := redact("dial nats://u:p@h failed, token tok", secretsOf("nats://u:p@h:4222", "tok"))
	if strings.Contains(got, "tok") || strings.Contains(got, ":p@") {
		t.Errorf("secrets survive redaction: %q", got)
	}
	if len(secretsOf("nats://host:4222", "")) != 0 {
		t.Error("a URL without credentials has no secrets")
	}
	if len(secretsOf("a@b://", "")) == 0 {
		t.Error("odd URLs must not panic or lose the user part")
	}
}

func TestSubject(t *testing.T) {
	for prefix, want := range map[string]string{
		"":          "cidx.run.run.started",
		"  ":        "cidx.run.run.started",
		"team.ci.":  "team.ci.run.started",
		"my.prefix": "my.prefix.run.started",
	} {
		if got := Subject(prefix, RunStarted); got != want {
			t.Errorf("Subject(%q) = %q, want %q", prefix, got, want)
		}
	}
}
