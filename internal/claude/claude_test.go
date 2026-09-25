package claude

import (
	"os"
	"testing"
	"time"
)

func TestParseTranscript(t *testing.T) {
	f, _ := os.Open("testdata/projects/-Users-me-dev-app/aaa.jsonl")
	defer f.Close()
	s, err := ParseTranscript(f, RootPattern("/Users/me/dev"))
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "aaa" || s.Project != "/Users/me/dev/app" || s.Branch != "main" || s.Title != "Login page" {
		t.Fatalf("identity wrong: %+v", s)
	}
	if s.Prompts != 2 || s.Turns != 2 || s.Interrupts != 1 {
		t.Fatalf("counts: prompts=%d turns=%d interrupts=%d", s.Prompts, s.Turns, s.Interrupts)
	}
	if s.FirstPrompt != "add a login page" || s.LastPrompt != "now add signup" {
		t.Fatalf("prompts: %q / %q", s.FirstPrompt, s.LastPrompt)
	}
	if s.Tools["Write"] != 1 || s.Tools["Bash"] != 1 || len(s.Models) != 1 {
		t.Fatalf("tools/models: %v %v", s.Tools, s.Models)
	}
	if s.Usage != (Usage{Input: 11, Output: 22, CacheRead: 100, CacheWrite: 5}) {
		t.Fatalf("usage: %+v", s.Usage)
	}
	if s.Repos["app"] != 1 || s.Repos["shared"] != 1 || s.PrimaryRepo() != "app" {
		t.Fatalf("repos: %v primary=%s", s.Repos, s.PrimaryRepo())
	}
	if !s.EndedOnUser {
		t.Fatal("session ending on an interrupt should be marked ended_on_user")
	}
	if !s.LastActive.Equal(time.Date(2026, 9, 25, 9, 0, 30, 0, time.UTC)) {
		t.Fatalf("last active: %v", s.LastActive)
	}
}

func TestIndexMergesLiveRegistry(t *testing.T) {
	x := NewIndex("testdata", "/Users/me/dev")
	x.Alive = func(pid int) bool { return pid != 333 }
	if err := x.Refresh(); err != nil {
		t.Fatal(err)
	}
	byID := map[string]Session{}
	for _, s := range x.Sessions() {
		byID[s.ID] = s
	}
	if len(byID) != 3 {
		t.Fatalf("want aaa, bbb, ccc; got %v", byID)
	}
	if byID["aaa"].Live != nil {
		t.Fatal("aaa is not running")
	}
	if l := byID["bbb"].Live; l == nil || l.Status != "busy" || l.Name != "api-1" {
		t.Fatalf("bbb live = %+v", l)
	}
	if c := byID["ccc"]; c.Live == nil || c.Project != "/Users/me/dev/web" {
		t.Fatalf("registry-only session missing: %+v", c)
	}
	if _, ok := byID["ddd"]; ok {
		t.Fatal("dead pid should be dropped")
	}
	if err := x.Refresh(); err != nil || len(x.Sessions()) != 3 {
		t.Fatal("cached refresh changed results")
	}
}

func TestActivity(t *testing.T) {
	x := NewIndex("testdata", "/Users/me/dev")
	x.Alive = func(int) bool { return false }
	x.Refresh()
	now := time.Date(2026, 9, 25, 20, 0, 0, 0, time.UTC)
	days := Activity(x.Sessions(), 3, now)
	if len(days) != 3 || days[2].Date != "2026-09-25" {
		t.Fatalf("days: %+v", days)
	}
	if days[1].Prompts != 1 || days[1].Sessions != 1 || days[2].Prompts != 2 || days[2].Sessions != 2 {
		t.Fatalf("buckets: %+v", days)
	}
	if days[2].Projects["api"] != 1 || days[2].Projects["app"] != 1 {
		t.Fatalf("projects: %+v", days[2].Projects)
	}
}
