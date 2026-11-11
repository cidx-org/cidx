package events

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// Settings say where and how a run publishes. An empty URL means no bus.
type Settings struct {
	URL     string // nats://host:port; may carry credentials, which are never printed
	Subject string // defaults to DefaultSubject
	RunID   string // defaults to a generated identifier
}

const (
	connectTimeout = 2 * time.Second
	flushTimeout   = 2 * time.Second
)

// publisher sends events on one NATS subject.
type publisher struct {
	conn    *nats.Conn
	subject string
	runID   string
	warn    func(string)
	once    sync.Once
}

// Connect returns the emitter of a run. With no URL it is Nop and no connection
// is attempted. When the bus cannot be reached it is Nop too, after one warning
// through warn: the run goes on, with the same output and the same exit code.
func Connect(s Settings, warn func(string)) Emitter {
	if strings.TrimSpace(s.URL) == "" {
		return Nop{}
	}
	if warn == nil {
		warn = func(string) {}
	}
	p := &publisher{
		subject: s.Subject,
		runID:   s.RunID,
		warn:    warn,
	}
	if p.subject == "" {
		p.subject = DefaultSubject
	}
	if p.runID == "" {
		p.runID = NewRunID()
	}

	conn, err := nats.Connect(s.URL,
		nats.Name("cidx"),
		nats.Timeout(connectTimeout),
		nats.NoReconnect(),
		nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) {}),
	)
	if err != nil {
		// The warning names the failure class, not the error text and never the
		// address: the URL is where credentials live.
		p.warnOnce("NATS unavailable, run events are not published")
		return Nop{}
	}
	p.conn = conn
	return p
}

func (p *publisher) warnOnce(msg string) {
	p.once.Do(func() { p.warn(msg) })
}

// Emit stamps the event and publishes it. A refused publication warns once and
// the run goes on.
func (p *publisher) Emit(e Event) {
	e.Format = Format
	e.RunID = p.runID
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	payload, err := json.Marshal(e)
	if err != nil {
		p.warnOnce("a run event could not be encoded, events may be missing")
		return
	}
	if err := p.conn.Publish(p.subject, payload); err != nil {
		p.warnOnce("NATS refused a run event, events may be missing")
	}
}

// Close flushes pending events and drops the connection.
func (p *publisher) Close() {
	if err := p.conn.FlushTimeout(flushTimeout); err != nil {
		p.warnOnce("NATS did not confirm the last run events, events may be missing")
	}
	p.conn.Close()
}
