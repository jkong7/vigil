package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Usage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
}

type Session struct {
	ID          string         `json:"id"`
	Project     string         `json:"project"`
	Branch      string         `json:"branch,omitempty"`
	Title       string         `json:"title"`
	FirstPrompt string         `json:"first_prompt"`
	LastPrompt  string         `json:"last_prompt"`
	Started     time.Time      `json:"started"`
	LastActive  time.Time      `json:"last_active"`
	Prompts     int            `json:"prompts"`
	Turns       int            `json:"turns"`
	Interrupts  int            `json:"interrupts"`
	Models      []string       `json:"models"`
	Tools       map[string]int `json:"tools"`
	Repos       map[string]int `json:"repos"`
	Usage       Usage          `json:"usage"`
	EndedOnUser bool           `json:"ended_on_user"`
	PromptTimes []time.Time    `json:"-"`
	Live        *Live          `json:"live,omitempty"`
}

type Live struct {
	PID        int       `json:"pid"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Kind       string    `json:"kind"`
	Entrypoint string    `json:"entrypoint"`
	Updated    time.Time `json:"updated"`
}

type line struct {
	Type        string                 `json:"type"`
	Timestamp   time.Time              `json:"timestamp"`
	SessionID   string                 `json:"sessionId"`
	Cwd         string                 `json:"cwd"`
	GitBranch   string                 `json:"gitBranch"`
	IsSidechain bool                   `json:"isSidechain"`
	IsMeta      bool                   `json:"isMeta"`
	AITitle     string                 `json:"aiTitle"`
	LastPrompt  string                 `json:"lastPrompt"`
	Origin      *struct{ Kind string } `json:"origin"`
	Message     *struct {
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func RootPattern(root string) *regexp.Regexp {
	if root == "" {
		return nil
	}
	root = filepath.Clean(root)
	alts := []string{regexp.QuoteMeta(root)}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(root, home+"/") {
		alts = append(alts, "~"+regexp.QuoteMeta(strings.TrimPrefix(root, home)))
	}
	return regexp.MustCompile(`(?:` + strings.Join(alts, "|") + `)/([A-Za-z0-9._-]+)`)
}

func contentText(raw json.RawMessage) (string, []block) {
	if len(raw) == 0 {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		json.Unmarshal(raw, &s)
		return s, nil
	}
	var blocks []block
	json.Unmarshal(raw, &blocks)
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n"), blocks
}

const interrupted = "[Request interrupted by user"

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func ParseTranscript(r io.Reader, root *regexp.Regexp) (*Session, error) {
	s := &Session{Tools: map[string]int{}, Repos: map[string]int{}}
	models := map[string]bool{}
	rd := bufio.NewReaderSize(r, 1<<20)
	for {
		raw, err := rd.ReadBytes('\n')
		if len(bytes.TrimSpace(raw)) > 0 {
			var l line
			if json.Unmarshal(raw, &l) == nil {
				s.apply(&l, models, root)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	for m := range models {
		s.Models = append(s.Models, m)
	}
	sort.Strings(s.Models)
	return s, nil
}

func (s *Session) apply(l *line, models map[string]bool, root *regexp.Regexp) {
	if l.SessionID != "" && s.ID == "" {
		s.ID = l.SessionID
	}
	switch l.Type {
	case "ai-title":
		if l.AITitle != "" {
			s.Title = l.AITitle
		}
		return
	case "last-prompt":
		if l.LastPrompt != "" {
			s.LastPrompt = clip(l.LastPrompt, 200)
		}
		return
	case "user", "assistant":
	default:
		return
	}
	if l.IsSidechain || l.Message == nil {
		return
	}
	if !l.Timestamp.IsZero() {
		if s.Started.IsZero() || l.Timestamp.Before(s.Started) {
			s.Started = l.Timestamp
		}
		if l.Timestamp.After(s.LastActive) {
			s.LastActive = l.Timestamp
		}
	}
	if l.Cwd != "" && s.Project == "" {
		s.Project = l.Cwd
	}
	if l.GitBranch != "" && l.GitBranch != "HEAD" {
		s.Branch = l.GitBranch
	}
	text, blocks := contentText(l.Message.Content)
	if l.Type == "assistant" {
		s.Turns++
		s.EndedOnUser = false
		if l.Message.Model != "" && !strings.HasPrefix(l.Message.Model, "<") {
			models[l.Message.Model] = true
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && b.Name != "" {
				s.Tools[b.Name]++
				if root != nil {
					seen := map[string]bool{}
					for _, m := range root.FindAllSubmatch(b.Input, -1) {
						if name := strings.TrimRight(string(m[1]), "."); name != "" && !seen[name] && !strings.HasPrefix(name, ".") {
							seen[name] = true
							s.Repos[name]++
						}
					}
				}
			}
		}
		if u := l.Message.Usage; u != nil {
			s.Usage.add(Usage{u.Input, u.Output, u.CacheRead, u.CacheWrite})
		}
		return
	}
	if strings.HasPrefix(text, interrupted) {
		s.Interrupts++
		s.EndedOnUser = true
		return
	}
	if l.IsMeta || l.Origin == nil || l.Origin.Kind != "human" {
		return
	}
	s.Prompts++
	s.EndedOnUser = true
	s.PromptTimes = append(s.PromptTimes, l.Timestamp)
	if s.FirstPrompt == "" {
		s.FirstPrompt = clip(text, 200)
	}
	s.LastPrompt = clip(text, 200)
}

type registry struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	UpdatedAt  int64  `json:"updatedAt"`
	StartedAt  int64  `json:"startedAt"`
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

type cached struct {
	size    int64
	modTime time.Time
	session *Session
}

type Index struct {
	Dir   string
	Root  *regexp.Regexp
	Alive func(pid int) bool

	mu    sync.RWMutex
	files map[string]cached
	live  map[string]registry
}

func NewIndex(dir, repoRoot string) *Index {
	return &Index{Dir: dir, Root: RootPattern(repoRoot), Alive: alive, files: map[string]cached{}}
}

func (s Session) PrimaryRepo() string {
	best, n := "", 0
	for name, c := range s.Repos {
		if c > n || (c == n && name < best) {
			best, n = name, c
		}
	}
	if best == "" {
		return filepath.Base(s.Project)
	}
	return best
}

func (x *Index) Refresh() error {
	paths, err := filepath.Glob(filepath.Join(x.Dir, "projects", "*", "*.jsonl"))
	if err != nil {
		return err
	}
	x.mu.RLock()
	old := x.files
	x.mu.RUnlock()
	next := make(map[string]cached, len(paths))
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if c, ok := old[p]; ok && c.size == st.Size() && c.modTime.Equal(st.ModTime()) {
			next[p] = c
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		s, err := ParseTranscript(f, x.Root)
		f.Close()
		if err != nil || s.Turns == 0 {
			continue
		}
		if s.ID == "" {
			s.ID = strings.TrimSuffix(filepath.Base(p), ".jsonl")
		}
		next[p] = cached{st.Size(), st.ModTime(), s}
	}
	live := map[string]registry{}
	regs, _ := filepath.Glob(filepath.Join(x.Dir, "sessions", "*.json"))
	for _, p := range regs {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var r registry
		if json.Unmarshal(raw, &r) == nil && r.SessionID != "" && x.Alive(r.PID) {
			live[r.SessionID] = r
		}
	}
	x.mu.Lock()
	x.files, x.live = next, live
	x.mu.Unlock()
	return nil
}

func (x *Index) Sessions() []Session {
	x.mu.RLock()
	defer x.mu.RUnlock()
	byID := map[string]Session{}
	for _, c := range x.files {
		s := *c.session
		if prev, ok := byID[s.ID]; ok && prev.LastActive.After(s.LastActive) {
			continue
		}
		byID[s.ID] = s
	}
	for id, r := range x.live {
		s, ok := byID[id]
		if !ok {
			s = Session{ID: id, Project: r.Cwd, Started: time.UnixMilli(r.StartedAt)}
		}
		s.Live = &Live{PID: r.PID, Name: r.Name, Status: r.Status, Kind: r.Kind, Entrypoint: r.Entrypoint,
			Updated: time.UnixMilli(r.UpdatedAt)}
		byID[id] = s
	}
	out := make([]Session, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActive.After(out[j].LastActive) })
	return out
}

type Day struct {
	Date     string         `json:"date"`
	Prompts  int            `json:"prompts"`
	Sessions int            `json:"sessions"`
	Projects map[string]int `json:"projects"`
}

func Activity(sessions []Session, days int, now time.Time) []Day {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	out := make([]Day, days)
	for i := range out {
		out[i] = Day{Date: start.AddDate(0, 0, i).Format("2006-01-02"), Projects: map[string]int{}}
	}
	for _, s := range sessions {
		touched := map[int]bool{}
		for _, t := range s.PromptTimes {
			t = t.In(now.Location())
			i := int(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()).Sub(start).Hours() / 24)
			if i < 0 || i >= days {
				continue
			}
			out[i].Prompts++
			out[i].Projects[s.PrimaryRepo()]++
			touched[i] = true
		}
		for i := range touched {
			out[i].Sessions++
		}
	}
	return out
}
