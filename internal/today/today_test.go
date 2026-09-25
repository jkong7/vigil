package today

import (
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/backlog"
	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/claude"
	"github.com/jkong7/vigil/internal/config"
	"github.com/jkong7/vigil/internal/repos"
	"github.com/jkong7/vigil/internal/state"
)

var now = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func kinds(p Plan) []Kind {
	var out []Kind
	for _, s := range p.Suggestions {
		out = append(out, s.Kind)
	}
	return out
}

func TestBuildRanksAndFilters(t *testing.T) {
	in := Input{
		Now: now,
		Monitors: []state.Snapshot{
			{Monitor: config.Monitor{Name: "api"}, Status: state.Down, Last: &check.Result{Error: "status 503"}},
			{Monitor: config.Monitor{Name: "web"}, Status: state.Up},
		},
		Sessions: []claude.Session{
			{ID: "s1", Project: "/dev/app", Title: "Login page", Prompts: 3, EndedOnUser: true, Interrupts: 1,
				LastActive: now.Add(-2 * time.Hour), PromptTimes: []time.Time{now.Add(-2 * time.Hour)}, Repos: map[string]int{"app": 3}},
			{ID: "s2", Project: "/dev/app", Title: "Done work", Prompts: 1, LastActive: now.Add(-time.Hour), Repos: map[string]int{"app": 1}},
			{ID: "s3", Project: "/dev/old", Title: "Ancient", Prompts: 1, EndedOnUser: true, LastActive: now.Add(-100 * time.Hour)},
			{ID: "s4", Project: "/dev/api", Prompts: 2, EndedOnUser: true, LastActive: now, Live: &claude.Live{Status: "busy"},
				Repos: map[string]int{"api": 1}},
		},
		Repos: []repos.Repo{
			{Name: "app", Path: "/dev/app", Changed: 2, Touched: now.Add(-time.Hour)},
			{Name: "api", Path: "/dev/api", Changed: 5, Touched: now},
			{Name: "lib", Path: "/dev/lib", Ahead: 2, Behind: 1, Branch: "main", Touched: now.Add(-24 * time.Hour)},
			{Name: "cli", Path: "/dev/cli", Ahead: 1, Touched: now.Add(-24 * time.Hour)},
		},
		Backlog: []backlog.Item{
			{Repo: "app", Text: "a1", Weight: 3}, {Repo: "app", Text: "a2", Weight: 2}, {Repo: "app", Text: "a3", Weight: 2},
			{Repo: "app", Text: "a4", Weight: 1}, {Repo: "app", Text: "done", Done: true, Weight: 3},
			{Repo: "stale", Text: "s1", Weight: 3},
		},
	}
	p := Build(in)
	got := kinds(p)
	if got[0] != Outage || got[1] != Resume {
		t.Fatalf("outage then resume should lead: %v", got)
	}
	count := map[Kind]int{}
	titles := map[string]bool{}
	for _, s := range p.Suggestions {
		count[s.Kind]++
		titles[s.Title] = true
	}
	if count[Resume] != 1 || count[Sync] != 1 || count[Push] != 1 || count[Commit] != 1 || count[Backlog] != 4 {
		t.Fatalf("counts = %v (%v)", count, got)
	}
	if titles["done"] || titles["a4"] || !titles["s1"] {
		t.Fatalf("backlog filtering wrong: %v", titles)
	}
	if p.Live != 1 || p.Busy != 1 || p.Focus["app"] != 1 {
		t.Fatalf("summary wrong: %+v", p)
	}
	if p.Suggestions[1].Action != "cd /dev/app && claude --resume s1" {
		t.Fatalf("resume action = %q", p.Suggestions[1].Action)
	}
	var a1, s1 float64
	for _, s := range p.Suggestions {
		switch s.Title {
		case "a1":
			a1 = s.Score
		case "s1":
			s1 = s.Score
		}
	}
	if a1 <= s1 {
		t.Fatalf("active repo backlog should outrank stale repo: a1=%v s1=%v", a1, s1)
	}
}
