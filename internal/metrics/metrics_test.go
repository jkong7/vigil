package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/state"
)

func TestObserve(t *testing.T) {
	m := New()
	m.ObserveResult(check.Result{Monitor: "api", Up: true, Latency: 30 * time.Millisecond})
	m.ObserveResult(check.Result{Monitor: "api", Up: false, Latency: time.Second})
	m.ObserveIncident(state.Incident{Monitor: "api"})
	if v := testutil.ToFloat64(m.up.WithLabelValues("api")); v != 0 {
		t.Errorf("up = %v, want 0 after a failure", v)
	}
	if v := testutil.ToFloat64(m.checks.WithLabelValues("api", "up")); v != 1 {
		t.Errorf("up checks = %v", v)
	}
	if v := testutil.ToFloat64(m.incidents.WithLabelValues("api")); v != 1 {
		t.Errorf("incidents = %v", v)
	}
	if n := testutil.CollectAndCount(m.latency); n != 1 {
		t.Errorf("latency series = %d", n)
	}
}
