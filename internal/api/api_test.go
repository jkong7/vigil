package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/backlog"
	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/claude"
	"github.com/jkong7/vigil/internal/config"
	"github.com/jkong7/vigil/internal/metrics"
	"github.com/jkong7/vigil/internal/repos"
	"github.com/jkong7/vigil/internal/state"
	"github.com/jkong7/vigil/internal/today"
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

type fakeWorkbench struct{}

func (fakeWorkbench) Sessions() []claude.Session {
	return []claude.Session{
		{ID: "a", Repos: map[string]int{"app": 1}, Live: &claude.Live{Status: "busy"}},
		{ID: "b", Repos: map[string]int{"api": 1}},
	}
}
func (fakeWorkbench) Repos() []repos.Repo { return nil }
func (fakeWorkbench) Backlog() []backlog.Item {
	return []backlog.Item{{Text: "open"}, {Text: "closed", Done: true}}
}
func (fakeWorkbench) Activity(days int) []claude.Day { return make([]claude.Day, days) }
func (fakeWorkbench) Plan() today.Plan {
	return today.Plan{Suggestions: []today.Suggestion{{Kind: today.Backlog, Title: "open"}}}
}

func TestWorkbenchEndpoints(t *testing.T) {
	tr := state.NewTracker(nil, 1)
	srv := httptest.NewServer((&Server{Tracker: tr, Workbench: fakeWorkbench{}}).Handler())
	defer srv.Close()
	var sessions []claude.Session
	if get(t, srv.URL+"/api/sessions?live=1", &sessions); len(sessions) != 1 || sessions[0].ID != "a" {
		t.Fatalf("live sessions = %+v", sessions)
	}
	if get(t, srv.URL+"/api/sessions?repo=api", &sessions); len(sessions) != 1 || sessions[0].ID != "b" {
		t.Fatalf("repo sessions = %+v", sessions)
	}
	var items []backlog.Item
	if get(t, srv.URL+"/api/backlog", &items); len(items) != 1 {
		t.Fatalf("open backlog = %+v", items)
	}
	if get(t, srv.URL+"/api/backlog?all=1", &items); len(items) != 2 {
		t.Fatalf("all backlog = %+v", items)
	}
	var list []repos.Repo
	if code := get(t, srv.URL+"/api/repos", &list); code != 200 || list == nil {
		t.Fatalf("repos = %d %v", code, list)
	}
	var days []claude.Day
	if get(t, srv.URL+"/api/activity?days=5", &days); len(days) != 5 {
		t.Fatalf("activity = %d days", len(days))
	}
	var plan today.Plan
	if get(t, srv.URL+"/api/today", &plan); len(plan.Suggestions) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestWorkbenchEndpointsAbsentWhenDisabled(t *testing.T) {
	srv := server(nil)
	defer srv.Close()
	if code := get(t, srv.URL+"/api/today", nil); code != 404 {
		t.Fatalf("/api/today without a workbench = %d, want 404", code)
	}
}
