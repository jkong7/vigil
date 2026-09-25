package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/state"
)

func open(t *testing.T) *Store {
	url := os.Getenv("VIGIL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("VIGIL_TEST_DATABASE_URL not set")
	}
	s, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	s.pool.Exec(context.Background(), "TRUNCATE check_results, incidents")
	t.Cleanup(s.Close)
	return s
}

func TestResultsAndUptime(t *testing.T) {
	s, ctx := open(t), context.Background()
	now := time.Now()
	for i, up := range []bool{true, true, false, true} {
		r := check.Result{Monitor: "api", Up: up, Latency: 12 * time.Millisecond, At: now.Add(-time.Duration(i) * time.Minute)}
		if !up {
			r.Error, r.StatusCode = "status 503", 503
		}
		if err := s.SaveResult(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	ratio, n, err := s.Uptime(ctx, "api", time.Hour)
	if err != nil || n != 4 || ratio != 0.75 {
		t.Fatalf("uptime = %v over %d (%v), want 0.75 over 4", ratio, n, err)
	}
	if ratio, n, _ := s.Uptime(ctx, "missing", time.Hour); ratio != 0 || n != 0 {
		t.Fatalf("missing monitor uptime = %v/%d", ratio, n)
	}
}

func TestIncidentLifecycle(t *testing.T) {
	s, ctx := open(t), context.Background()
	start := time.Now().Add(-time.Minute).Truncate(time.Microsecond)
	i := state.Incident{Monitor: "api", Cause: "timeout", Started: start}
	if err := s.OpenIncident(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenIncident(ctx, i); err != nil {
		t.Fatal("reopening an open incident should be a no-op:", err)
	}
	end := time.Now().Truncate(time.Microsecond)
	i.Resolved = &end
	if err := s.ResolveIncident(ctx, i); err != nil {
		t.Fatal(err)
	}
	got, err := s.Incidents(ctx, "api", 10)
	if err != nil || len(got) != 1 || got[0].Resolved == nil || !got[0].Resolved.Equal(end) {
		t.Fatalf("incidents = %+v (%v)", got, err)
	}
}
