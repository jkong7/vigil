package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/config"
)

type countingProber struct{ n atomic.Int64 }

func (c *countingProber) Probe(ctx context.Context, m config.Monitor) check.Result {
	c.n.Add(1)
	return check.Result{Monitor: m.Name, Up: true, At: time.Now()}
}

func TestRunProbesEachMonitorOnInterval(t *testing.T) {
	p := &countingProber{}
	s := New(p)
	s.Jitter = func(time.Duration) time.Duration { return 0 }
	monitors := []config.Monitor{{Name: "a", Interval: 20 * time.Millisecond}, {Name: "b", Interval: 20 * time.Millisecond}}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Millisecond)
	defer cancel()
	out := make(chan check.Result, 100)
	s.Run(ctx, monitors, out)
	close(out)
	seen := map[string]int{}
	for r := range out {
		seen[r.Monitor]++
	}
	for _, name := range []string{"a", "b"} {
		if seen[name] < 3 || seen[name] > 7 {
			t.Errorf("monitor %s probed %d times, want about 6", name, seen[name])
		}
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	s := New(&countingProber{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx, []config.Monitor{{Name: "a", Interval: time.Hour}}, make(chan check.Result))
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop after cancel")
	}
}
