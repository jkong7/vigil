package state

import (
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/config"
)

func res(up bool, at int) check.Result {
	r := check.Result{Monitor: "api", Up: up, Latency: 10 * time.Millisecond, At: time.Unix(int64(at), 0)}
	if !up {
		r.Error = "boom"
	}
	return r
}

func TestIncidentLifecycle(t *testing.T) {
	tr := NewTracker([]config.Monitor{{Name: "api", FailAfter: 2}}, 10)
	if o, _ := tr.Observe(res(true, 1)); o != nil {
		t.Fatal("success opened an incident")
	}
	if o, _ := tr.Observe(res(false, 2)); o != nil {
		t.Fatal("single failure opened an incident before fail_after")
	}
	o, _ := tr.Observe(res(false, 3))
	if o == nil || o.Cause != "boom" || o.Started.Unix() != 3 {
		t.Fatalf("expected incident at t=3, got %+v", o)
	}
	if o, _ := tr.Observe(res(false, 4)); o != nil {
		t.Fatal("incident opened twice")
	}
	if s, _ := tr.Get("api"); s.Status != Down || s.Open == nil {
		t.Fatalf("expected down with open incident: %+v", s)
	}
	_, r := tr.Observe(res(true, 5))
	if r == nil || r.Resolved == nil || r.Resolved.Unix() != 5 {
		t.Fatalf("expected resolution at t=5, got %+v", r)
	}
	s, _ := tr.Get("api")
	if s.Status != Up || s.Open != nil || s.Checks != 5 || s.Uptime != 0.4 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
}

func TestWindowWraps(t *testing.T) {
	tr := NewTracker([]config.Monitor{{Name: "api", FailAfter: 1}}, 3)
	for i, up := range []bool{false, false, true, true, true} {
		tr.Observe(res(up, i))
	}
	s, _ := tr.Get("api")
	if s.Checks != 3 || s.Uptime != 1 || len(s.Recent) != 3 {
		t.Fatalf("window did not wrap: %+v", s)
	}
}

func TestUnknownMonitorIgnored(t *testing.T) {
	tr := NewTracker(nil, 3)
	if o, r := tr.Observe(res(false, 1)); o != nil || r != nil {
		t.Fatal("unknown monitor produced an incident")
	}
	if _, ok := tr.Get("api"); ok {
		t.Fatal("unknown monitor has state")
	}
}
