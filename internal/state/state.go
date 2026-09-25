package state

import (
	"sort"
	"sync"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/config"
)

type Status string

const (
	Unknown Status = "unknown"
	Up      Status = "up"
	Down    Status = "down"
)

type Incident struct {
	Monitor  string     `json:"monitor"`
	Started  time.Time  `json:"started"`
	Resolved *time.Time `json:"resolved,omitempty"`
	Cause    string     `json:"cause"`
}

type Snapshot struct {
	Monitor   config.Monitor `json:"monitor"`
	Status    Status         `json:"status"`
	Last      *check.Result  `json:"last,omitempty"`
	Uptime    float64        `json:"uptime"`
	Checks    int            `json:"checks"`
	AvgMillis float64        `json:"avg_latency_ms"`
	Recent    []bool         `json:"recent"`
	Open      *Incident      `json:"open_incident,omitempty"`
}

type monitorState struct {
	cfg      config.Monitor
	status   Status
	failures int
	last     *check.Result
	window   []check.Result
	next     int
	full     bool
	open     *Incident
}

type Tracker struct {
	mu     sync.RWMutex
	size   int
	states map[string]*monitorState
}

func NewTracker(monitors []config.Monitor, window int) *Tracker {
	t := &Tracker{size: window, states: map[string]*monitorState{}}
	for _, m := range monitors {
		t.states[m.Name] = &monitorState{cfg: m, status: Unknown, window: make([]check.Result, window)}
	}
	return t
}

func (t *Tracker) Observe(r check.Result) (opened, resolved *Incident) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.states[r.Monitor]
	if !ok {
		return nil, nil
	}
	s.last = &r
	s.window[s.next] = r
	s.next = (s.next + 1) % t.size
	if s.next == 0 {
		s.full = true
	}
	if r.Up {
		s.failures = 0
		if s.open != nil {
			at := r.At
			s.open.Resolved = &at
			resolved, s.open = s.open, nil
		}
		s.status = Up
		return nil, resolved
	}
	s.failures++
	if s.failures >= s.cfg.FailAfter && s.open == nil {
		s.open = &Incident{Monitor: r.Monitor, Started: r.At, Cause: r.Error}
		s.status = Down
		cp := *s.open
		return &cp, nil
	}
	return nil, nil
}

func (t *Tracker) results(s *monitorState) []check.Result {
	if s.full {
		return append(append([]check.Result{}, s.window[s.next:]...), s.window[:s.next]...)
	}
	return append([]check.Result{}, s.window[:s.next]...)
}

func (t *Tracker) snapshot(s *monitorState) Snapshot {
	rs := t.results(s)
	snap := Snapshot{Monitor: s.cfg, Status: s.status, Last: s.last, Checks: len(rs), Recent: make([]bool, len(rs))}
	var up int
	var total time.Duration
	for i, r := range rs {
		snap.Recent[i] = r.Up
		total += r.Latency
		if r.Up {
			up++
		}
	}
	if len(rs) > 0 {
		snap.Uptime = float64(up) / float64(len(rs))
		snap.AvgMillis = float64(total.Microseconds()) / 1000 / float64(len(rs))
	}
	if s.open != nil {
		cp := *s.open
		snap.Open = &cp
	}
	return snap
}

func (t *Tracker) Get(name string) (Snapshot, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.states[name]
	if !ok {
		return Snapshot{}, false
	}
	return t.snapshot(s), true
}

func (t *Tracker) All() []Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Snapshot, 0, len(t.states))
	for _, s := range t.states {
		out = append(out, t.snapshot(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Monitor.Name < out[j].Monitor.Name })
	return out
}
