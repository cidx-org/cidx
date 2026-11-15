package events

import (
	"encoding/json"
	"net/url"
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

	// User and Password are given apart from the address. When set they win over
	// any credentials the address carries. Neither is ever printed.
	User     string
	Password string
	// CredsFile is the path of a NATS credentials file (JWT and seed).
	CredsFile string
}

// overrideWarning is said when credentials given apart from the address replace
// those the address carries. It names neither the account nor the secret.
const overrideWarning = "NATS credentials from the environment override those in the address"

// authOptions turns the credentials given apart from the address into
// connection options, and returns the address to dial. The client library lets
// the credentials of the address win over its options, so when separate
// credentials exist the address is stripped of its own: that is what makes the
// separate ones take precedence. The second result says whether anything was
// overridden.
func authOptions(s Settings) (string, []nats.Option, bool) {
	var opts []nats.Option
	separate := false
	if s.User != "" || s.Password != "" {
		opts = append(opts, nats.UserInfo(s.User, s.Password))
		separate = true
	}
	if s.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(s.CredsFile))
		separate = true
	}
	address := strings.TrimSpace(s.URL)
	if !separate {
		return address, opts, false
	}
	u, err := url.Parse(address)
	if err != nil || u.User == nil {
		return address, opts, false
	}
	u.User = nil
	return u.String(), opts, true
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

	address, auth, overridden := authOptions(s)
	if overridden {
		// Not warnOnce: that one is kept for the failures of the bus itself.
		p.warn(overrideWarning)
	}
	opts := append([]nats.Option{
		nats.Name("cidx"),
		nats.Timeout(connectTimeout),
		nats.NoReconnect(),
		nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) {}),
	}, auth...)
	conn, err := nats.Connect(address, opts...)
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
