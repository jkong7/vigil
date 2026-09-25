package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/config"
	"github.com/jkong7/vigil/internal/metrics"
	"github.com/jkong7/vigil/internal/state"
)

type fakeHistory struct{}

func (fakeHistory) Incidents(ctx context.Context, monitor string, limit int) ([]state.Incident, error) {
	return []state.Incident{{Monitor: "api", Cause: "timeout", Started: time.Unix(0, 0)}}, nil
}

func (fakeHistory) Uptime(ctx context.Context, monitor string, since time.Duration) (float64, int, error) {
	return 0.99, 100, nil
}

func server(history History) *httptest.Server {
	tr := state.NewTracker([]config.Monitor{{Name: "api", FailAfter: 1}}, 10)
	tr.Observe(check.Result{Monitor: "api", Up: true, At: time.Now()})
	s := &Server{Tracker: tr, History: history, Registry: metrics.New().Registry}
	return httptest.NewServer(s.Handler())
}

func get(t *testing.T, url string, into any) int {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if into != nil {
		json.NewDecoder(resp.Body).Decode(into)
	}
	return resp.StatusCode
}

func TestMonitors(t *testing.T) {
	srv := server(nil)
	defer srv.Close()
	var list []state.Snapshot
	if code := get(t, srv.URL+"/api/monitors", &list); code != 200 || len(list) != 1 || list[0].Status != state.Up {
		t.Fatalf("GET /api/monitors = %d %+v", code, list)
	}
	if code := get(t, srv.URL+"/api/monitors/nope", nil); code != 404 {
		t.Fatalf("unknown monitor returned %d", code)
	}
}

func TestMonitorDetailWithHistory(t *testing.T) {
	srv := server(fakeHistory{})
	defer srv.Close()
	var d detail
	if code := get(t, srv.URL+"/api/monitors/api", &d); code != 200 || d.History["7d"] != 0.99 || len(d.Incidents) != 1 {
		t.Fatalf("detail = %d %+v", code, d)
	}
}

func TestIncidentsWithoutHistory(t *testing.T) {
	srv := server(nil)
	defer srv.Close()
	var list []state.Incident
	if code := get(t, srv.URL+"/api/incidents", &list); code != 200 || list == nil || len(list) != 0 {
		t.Fatalf("incidents = %d %v", code, list)
	}
}

func TestStaticAndMetrics(t *testing.T) {
	srv := server(nil)
	defer srv.Close()
	for _, path := range []string{"/", "/app.js", "/healthz", "/metrics"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("GET %s = %v %v", path, resp.StatusCode, err)
		}
		resp.Body.Close()
	}
}
