package presets

import (
	"strings"
	"time"
)

// UnansweredFinding is one finding still waiting for an answer, named the way
// the code-scanning alert that publishes it is: by repository and identifier.
type UnansweredFinding struct {
	Repository string
	ID         string

	// KEV records that CISA lists it as actively exploited. It is the one signal
	// that never waits: the discussion's emergency path.
	KEV bool
}

// AlertKey is the key a finding's first-seen date is filed under: the alert's
// rule id, repository then identifier, with the identifier upper-cased because
// the two scanners do not agree on the case of a GHSA.
func AlertKey(repository, id string) string {
	return repository + "/" + strings.ToUpper(id)
}

// Grace is the window a finding gets before it fails the audit (#524).
//
// The gate used to ask whether anything was unanswered, which on images that all
// carry an OS base layer is true almost every day: three audits in one morning
// found three batches of CVE published after the previous scan. That measures
// how fast advisories arrive, not how long a finding has waited. A window
// delays the failure and never the visibility — the page still lists everything.
type Grace struct {
	// Days is the length of the window. It is counted the way an acceptance's
	// expiry is: the last day is still inside it.
	Days int

	// FirstSeen is the day the audit first reported each finding, by AlertKey.
	//
	// nil means no source answered, so no age can be established and every
	// finding counts as past its window: an age nobody can establish fails
	// closed, like every other unreadable input here. A non-nil map, even an
	// empty one, means the source answered, and a finding absent from it is one
	// the audit reports for the first time today.
	FirstSeen map[string]time.Time
}

// PastGrace returns the unanswered findings that fail the gate on the page's day.
func (s CatalogueSummary) PastGrace() []UnansweredFinding {
	var past []UnansweredFinding
	for _, f := range s.Findings {
		if s.pastGrace(f) {
			past = append(past, f)
		}
	}
	return past
}

func (s CatalogueSummary) pastGrace(f UnansweredFinding) bool {
	if s.Grace == nil {
		return true // no window: presence is what fails, as it always did
	}
	if f.KEV || s.Grace.FirstSeen == nil {
		return true
	}
	seen, known := s.Grace.FirstSeen[AlertKey(f.Repository, f.ID)]
	if !known {
		return false // the source answered and has never seen it: new today
	}
	age := int(utcDay(s.Day).Sub(utcDay(seen)).Hours() / 24)
	return age > s.Grace.Days
}

// Failing reports whether the audit gate fails.
//
// Waiting says what is on the page; Failing says which of it is overdue. Without
// a window they are the same question. With one, Failing is a subset of Waiting
// and the page names the subset, so the gate, the tab and the page cannot
// disagree about what an audit failure meant (#436).
func (s CatalogueSummary) Failing() bool {
	if s.Grace == nil {
		return s.Waiting()
	}
	return len(s.PastGrace()) > 0 ||
		len(s.Expired) > 0 ||
		s.basesWaiting()
}

func (s CatalogueSummary) basesWaiting() bool {
	return len(s.basesIn(BaseEnded)) > 0 ||
		len(s.basesIn(BaseEndingSoon)) > 0 ||
		len(s.basesIn(BaseUnknown)) > 0
}
