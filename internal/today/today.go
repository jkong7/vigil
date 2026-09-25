package today

import (
	"fmt"
	"sort"
	"time"

	"github.com/jkong7/vigil/internal/backlog"
	"github.com/jkong7/vigil/internal/claude"
	"github.com/jkong7/vigil/internal/repos"
	"github.com/jkong7/vigil/internal/state"
)

type Kind string

const (
	Outage   Kind = "outage"
	Resume   Kind = "resume"
	Sync     Kind = "sync"
	Commit   Kind = "commit"
	Push     Kind = "push"
	Backlog  Kind = "backlog"
	Momentum Kind = "momentum"
)

type Suggestion struct {
	Kind   Kind    `json:"kind"`
	Repo   string  `json:"repo,omitempty"`
	Title  string  `json:"title"`
	Why    string  `json:"why"`
	Action string  `json:"action,omitempty"`
	Score  float64 `json:"score"`
}

type Input struct {
	Now      time.Time
	Sessions []claude.Session
	Repos    []repos.Repo
	Backlog  []backlog.Item
	Monitors []state.Snapshot
}

type Plan struct {
	Generated   time.Time      `json:"generated"`
	Suggestions []Suggestion   `json:"suggestions"`
	Focus       map[string]int `json:"focus"`
	Live        int            `json:"live_sessions"`
	Busy        int            `json:"busy_sessions"`
}

const perRepoBacklog = 3

func ago(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func recency(now, t time.Time, window time.Duration) float64 {
	if t.IsZero() {
		return 0
	}
	age := now.Sub(t)
	if age <= 0 {
		return 1
	}
	if age >= window {
		return 0
	}
	return 1 - float64(age)/float64(window)
}

func Build(in Input) Plan {
	now := in.Now
	p := Plan{Generated: now, Focus: map[string]int{}}
	var out []Suggestion
	activity := map[string]time.Time{}
	liveRepos := map[string]bool{}

	for _, m := range in.Monitors {
		if m.Status != state.Down {
			continue
		}
		why := "failing"
		if m.Last != nil && m.Last.Error != "" {
			why = m.Last.Error
		}
		if m.Open != nil {
			why += fmt.Sprintf(" (down since %s)", ago(now.Sub(m.Open.Started)))
		}
		out = append(out, Suggestion{Kind: Outage, Title: "Fix " + m.Monitor.Name, Why: why, Score: 100})
	}

	for _, s := range in.Sessions {
		repo := s.PrimaryRepo()
		if s.LastActive.After(activity[repo]) {
			activity[repo] = s.LastActive
		}
		for _, t := range s.PromptTimes {
			if now.Sub(t) < 72*time.Hour {
				p.Focus[repo]++
			}
		}
		if s.Live != nil {
			p.Live++
			liveRepos[repo] = true
			if s.Live.Status == "busy" {
				p.Busy++
			}
			continue
		}
		age := now.Sub(s.LastActive)
		if !s.EndedOnUser || s.Prompts == 0 || age > 72*time.Hour {
			continue
		}
		title := s.Title
		if title == "" {
			title = s.FirstPrompt
		}
		why := "ended on your message without a reply"
		if s.Interrupts > 0 {
			why = "you interrupted it and never came back"
		}
		out = append(out, Suggestion{
			Kind: Resume, Repo: repo, Title: "Pick up “" + title + "”",
			Why:    fmt.Sprintf("%s, %s. Last ask: %s", why, ago(age), s.LastPrompt),
			Action: fmt.Sprintf("cd %s && claude --resume %s", s.Project, s.ID),
			Score:  70 + 15*recency(now, s.LastActive, 72*time.Hour),
		})
	}

	for _, r := range in.Repos {
		if r.Touched.After(activity[r.Name]) {
			activity[r.Name] = r.Touched
		}
		fresh := recency(now, r.Touched, 7*24*time.Hour)
		cd := "cd " + r.Path
		switch {
		case r.Ahead > 0 && r.Behind > 0:
			out = append(out, Suggestion{Kind: Sync, Repo: r.Name, Title: "Reconcile " + r.Name + " with origin",
				Why:    fmt.Sprintf("%d local and %d remote commits have diverged on %s", r.Ahead, r.Behind, r.Branch),
				Action: cd + " && git pull --rebase", Score: 72 + 10*fresh})
		case r.Ahead > 0:
			out = append(out, Suggestion{Kind: Push, Repo: r.Name, Title: fmt.Sprintf("Push %d commit(s) in %s", r.Ahead, r.Name),
				Why: "last: " + r.LastSubject, Action: cd + " && git push", Score: 50 + 10*fresh})
		}
		if r.Dirty() && !liveRepos[r.Name] {
			out = append(out, Suggestion{Kind: Commit, Repo: r.Name,
				Title:  fmt.Sprintf("Commit or stash %d change(s) in %s", r.Changed+r.Untracked, r.Name),
				Why:    fmt.Sprintf("%d modified, %d untracked, touched %s", r.Changed, r.Untracked, ago(now.Sub(r.Touched))),
				Action: cd + " && git status", Score: 55 + 20*recency(now, r.Touched, 48*time.Hour)})
		}
	}

	perRepo := map[string]int{}
	for _, it := range backlog.Open(in.Backlog) {
		momentum := recency(now, activity[it.Repo], 5*24*time.Hour)
		score := 25 + 8*float64(it.Weight) + 20*momentum
		if !it.Captured.IsZero() {
			score += 5 * recency(now, it.Captured, 7*24*time.Hour)
		}
		why := fmt.Sprintf("%s:%d", it.File, it.Line)
		if it.Section != "" {
			why = it.Section + " · " + why
		}
		out = append(out, Suggestion{Kind: Backlog, Repo: it.Repo, Title: it.Text, Why: why, Score: score})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	for _, s := range out {
		if s.Kind == Backlog {
			if perRepo[s.Repo] >= perRepoBacklog {
				continue
			}
			perRepo[s.Repo]++
		}
		p.Suggestions = append(p.Suggestions, s)
	}
	return p
}
