// Package events publishes structured run events so a caller can follow a
// `cidx run` without parsing its text output.
//
// The contract is documented in docs/reference/events.md. The rule that shapes
// this package: publishing is observation. It never changes what a run prints,
// how it exits, or whether it starts — a bus that is down costs one warning.
package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/executor"
)

// Format is the value of the `format` field of every event. The suffix is the
// major version of the contract: fields are only ever added within a version.
const Format = "cidx.run-event/v1"

// DefaultSubject is where events go when no subject is configured.
const DefaultSubject = "cidx.run.events"

// Type names what an event reports.
type Type string

const (
	RunStarted        Type = "run.started"
	RunFinished       Type = "run.finished"
	PhaseStarted      Type = "phase.started"
	PhaseFinished     Type = "phase.finished"
	ContainerStarted  Type = "container.started"
	ContainerFinished Type = "container.finished"
)

// Status of a finished run, phase or container.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped" // local safety previewed the container instead of running it
)

// Failure is the nature of a failed step: a closed set, so a consumer can branch
// on it. It is never an error message.
const (
	FailureExitCode    = "exit_code"   // the container exited non-zero
	FailureImagePull   = "image_pull"  // the image could not be obtained
	FailureTimeout     = "timeout"     // the time limit was exceeded
	FailureInterrupted = "interrupted" // the run was interrupted
	FailureStart       = "start"       // the container could not start
	FailureOther       = "other"       // any other cause
)

// Classify names the nature of a failure. It returns "" for a nil error.
func Classify(err error) string {
	var exit *executor.ExitError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &exit):
		return FailureExitCode
	case errors.Is(err, executor.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return FailureTimeout
	case errors.Is(err, executor.ErrInterrupted), errors.Is(err, context.Canceled):
		return FailureInterrupted
	case errors.Is(err, executor.ErrImagePull):
		return FailureImagePull
	case errors.Is(err, executor.ErrStart):
		return FailureStart
	}
	return FailureOther
}

// Event is the JSON document published for each step of a run. It carries names,
// images, exit codes and durations — never container output, error text,
// environment values or connection details.
type Event struct {
	Format     string    `json:"format"`
	Type       Type      `json:"type"`
	RunID      string    `json:"run_id"`
	Time       time.Time `json:"time"`
	Target     string    `json:"target,omitempty"`
	Phase      string    `json:"phase,omitempty"`
	Container  string    `json:"container,omitempty"`
	Image      string    `json:"image,omitempty"`
	Status     string    `json:"status,omitempty"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	DurationMS *int64    `json:"duration_ms,omitempty"`

	// Failure is the nature of a failure, on a failed *.finished only.
	Failure string `json:"failure,omitempty"`

	// FailedContainer is the first container that failed, on a failed
	// phase.finished or run.finished when there is one.
	FailedContainer string `json:"failed_container,omitempty"`
}

// Emitter receives the events of one run. Emit never fails and never blocks the
// run on a slow bus; Close flushes what is pending.
type Emitter interface {
	Emit(Event)
	Close()
}

// Nop is the emitter of a run that was given no bus: it does nothing and opens
// no connection.
type Nop struct{}

func (Nop) Emit(Event) {}
func (Nop) Close()     {}

// Finish fills the closing fields of a finished event from how the step ended.
func Finish(e Event, start time.Time, err error) Event {
	ms := time.Since(start).Milliseconds()
	e.DurationMS = &ms
	e.Status = StatusPassed
	if err != nil {
		e.Status = StatusFailed
		e.Failure = Classify(err)
	}
	if code, ok := ExitCode(err); ok {
		e.ExitCode = &code
	}
	return e
}

// ExitCode is the exit code of a step: 0 when it passed, the container's own
// when it exited non-zero. A step that failed without a container exit (an
// image that could not be pulled) has none, and the field is left out.
func ExitCode(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var exit *executor.ExitError
	if errors.As(err, &exit) {
		return int(exit.Code), true
	}
	return 0, false
}

// NewRunID returns a random identifier for a run whose caller gave none.
func NewRunID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "run-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}
