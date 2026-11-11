package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

const (
	connectTimeout = 2 * time.Second
	flushTimeout   = 2 * time.Second
)

// Settings is everything needed to publish. An empty URL means "do not
// publish": Open then returns Nop without dialling anything.
type Settings struct {
	URL           string // NATS server address; may carry user:password@
	Token         string // optional token; never written anywhere
	SubjectPrefix string
	RunID         string
}

// natsEmitter publishes events on a NATS connection. The first failure — a
// refused publication — is reported once and silences the emitter: a run is
// never made slower or noisier by a broker that went away.
type natsEmitter struct {
	mu      sync.Mutex
	conn    *nats.Conn
	prefix  string
	runID   string
	logger  *logrus.Logger
	secrets []string
	dead    bool
}

// Open connects to the broker named by s and returns the emitter for one
// execution. It never fails: with no address it returns Nop and dials nothing;
// when the broker cannot be reached it warns once and returns Nop.
func Open(s Settings, logger *logrus.Logger) (Emitter, string) {
	runID := strings.TrimSpace(s.RunID)
	if runID == "" {
		runID = NewRunID()
	}
	url := strings.TrimSpace(s.URL)
	if url == "" {
		return Nop{}, runID
	}

	secrets := secretsOf(url, s.Token)
	opts := []nats.Option{
		nats.Name("cidx"),
		nats.Timeout(connectTimeout),
		nats.NoReconnect(),
	}
	if s.Token != "" {
		opts = append(opts, nats.Token(s.Token))
	}
	conn, err := nats.Connect(url, opts...)
	if err != nil {
		logger.Warnf("⚠️  NATS events disabled: could not connect (%s)", redact(err.Error(), secrets))
		return Nop{}, runID
	}
	logger.Infof("📡 Events: publishing run %s", runID)
	return &natsEmitter{
		conn:    conn,
		prefix:  s.SubjectPrefix,
		runID:   runID,
		logger:  logger,
		secrets: secrets,
	}, runID
}

// Emit publishes one event. Format, run id and time are filled in here so every
// event of an execution is consistent.
func (n *natsEmitter) Emit(e Event) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.dead {
		return
	}
	e.Format = Format
	e.RunID = n.runID
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	payload, err := json.Marshal(e)
	if err != nil {
		n.fail(err)
		return
	}
	if err := n.conn.Publish(Subject(n.prefix, e.Type), payload); err != nil {
		n.fail(err)
	}
}

// fail reports the first failure and stops publishing. Callers hold n.mu.
func (n *natsEmitter) fail(err error) {
	n.dead = true
	n.logger.Warnf("⚠️  NATS events disabled: %s", redact(err.Error(), n.secrets))
}

// Close flushes what the connection still holds and closes it.
func (n *natsEmitter) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.dead {
		if err := n.conn.FlushTimeout(flushTimeout); err != nil {
			n.fail(fmt.Errorf("flush: %w", err))
		}
	}
	n.conn.Close()
}

// secretsOf lists the strings that must never reach the output: the token and
// the password and user of the URL.
func secretsOf(url, token string) []string {
	var out []string
	if token != "" {
		out = append(out, token)
	}
	if at := strings.LastIndex(url, "@"); at > 0 {
		start := 0
		if i := strings.Index(url, "://"); i >= 0 && i+3 <= at {
			start = i + 3
		}
		creds := url[start:at]
		for _, part := range strings.SplitN(creds, ":", 2) {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// redact removes every secret from a message.
func redact(msg string, secrets []string) string {
	for _, s := range secrets {
		msg = strings.ReplaceAll(msg, s, "***")
	}
	return msg
}
