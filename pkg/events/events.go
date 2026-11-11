// Package events publishes what "cidx run" is doing as structured events, so a
// caller can follow an execution without parsing its text output.
//
// Publication is strictly optional and strictly best-effort: without a NATS
// address nothing is created and nothing is dialled, and a broker that is
// absent or refuses a message never changes the run or its exit code.
package events

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Format is the value of the "format" field of every event. It changes only
// when an existing field changes meaning or disappears.
const Format = "cidx.event/v1"

// DefaultSubjectPrefix is the subject prefix used when none is configured.
const DefaultSubjectPrefix = "cidx.run"

// Event types. The subject of an event is "<prefix>.<type>".
const (
	RunStarted        = "run.started"
	RunFinished       = "run.finished"
	PhaseStarted      = "phase.started"
	PhaseFinished     = "phase.finished"
	ContainerStarted  = "container.started"
	ContainerFinished = "container.finished"
)

// Status of a finished run, phase or container.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped" // local safety previewed it instead of running it
)

// Event is the JSON document published for each step of an execution. It
// carries names, codes and durations only: never container output, never
// environment values, never connection details.
type Event struct {
	Format     string    `json:"format"`
	Type       string    `json:"type"`
	RunID      string    `json:"run_id"`
	Time       time.Time `json:"time"`
	Target     string    `json:"target,omitempty"`
	Phase      string    `json:"phase,omitempty"`
	Container  string    `json:"container,omitempty"`
	Image      string    `json:"image,omitempty"`
	Status     string    `json:"status,omitempty"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	DurationMS *int64    `json:"duration_ms,omitempty"`
}

// Emitter receives the events of one execution. Implementations are safe for
// concurrent use (containers of a phase may run in parallel) and never fail
// the caller: Emit has no error to return.
type Emitter interface {
	Emit(Event)
	// Close delivers what is still pending and releases the connection.
	Close()
}

// Nop is the emitter used when no NATS address is given.
type Nop struct{}

// Emit discards the event.
func (Nop) Emit(Event) {}

// Close does nothing.
func (Nop) Close() {}

// NewRunID returns a random identifier for an execution whose caller gave none.
func NewRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Millis is the duration as the pointer the Event fields use.
func Millis(d time.Duration) *int64 {
	ms := d.Milliseconds()
	return &ms
}

// Code is the exit code as the pointer the Event fields use.
func Code(c int) *int { return &c }

// Subject returns the subject an event type is published on.
func Subject(prefix, eventType string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	if prefix == "" {
		prefix = DefaultSubjectPrefix
	}
	return prefix + "." + eventType
}

// Recorder is an Emitter that keeps what it is given, for tests of the code
// that emits.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

// Emit records the event.
func (r *Recorder) Emit(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// Close does nothing.
func (r *Recorder) Close() {}

// Events returns a copy of what was recorded, in order.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}
