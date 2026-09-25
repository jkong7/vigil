package repos

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Repo struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	Remote       string    `json:"remote,omitempty"`
	Ahead        int       `json:"ahead"`
	Behind       int       `json:"behind"`
	Upstream     bool      `json:"upstream"`
	Changed      int       `json:"changed"`
	Untracked    int       `json:"untracked"`
	LastCommit   time.Time `json:"last_commit"`
	LastSubject  string    `json:"last_subject"`
	CommitsToday int       `json:"commits_today"`
	CommitsWeek  int       `json:"commits_week"`
	Touched      time.Time `json:"touched"`
	Error        string    `json:"error,omitempty"`
}

func (r Repo) Dirty() bool { return r.Changed+r.Untracked > 0 }

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	out, err := cmd.Output()
	return string(out), err
}

func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func Inspect(ctx context.Context, dir string, now time.Time) Repo {
	r := Repo{Name: filepath.Base(dir), Path: dir}
	status, err := git(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		r.Error = "git status failed"
		return r
	}
	parseStatus(&r, status)
	if url, err := git(ctx, dir, "remote", "get-url", "origin"); err == nil {
		r.Remote = strings.TrimSpace(url)
	}
	if out, err := git(ctx, dir, "log", "-1", "--format=%ct%x09%s"); err == nil {
		if ts, subject, ok := strings.Cut(strings.TrimSpace(out), "\t"); ok {
			if sec, err := strconv.ParseInt(ts, 10, 64); err == nil {
				r.LastCommit = time.Unix(sec, 0)
			}
			r.LastSubject = subject
		}
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if out, err := git(ctx, dir, "log", "--since="+midnight.AddDate(0, 0, -6).Format(time.RFC3339), "--format=%ct"); err == nil {
		for _, l := range strings.Fields(out) {
			sec, _ := strconv.ParseInt(l, 10, 64)
			r.CommitsWeek++
			if !time.Unix(sec, 0).Before(midnight) {
				r.CommitsToday++
			}
		}
	}
	r.Touched = r.LastCommit
	if r.Dirty() {
		if t := newestChange(ctx, dir); t.After(r.Touched) {
			r.Touched = t
		}
	}
	return r
}

func parseStatus(r *Repo, status string) {
	sc := bufio.NewScanner(strings.NewReader(status))
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "# branch.head "):
			r.Branch = strings.TrimPrefix(l, "# branch.head ")
		case strings.HasPrefix(l, "# branch.upstream "):
			r.Upstream = true
		case strings.HasPrefix(l, "# branch.ab "):
			f := strings.Fields(strings.TrimPrefix(l, "# branch.ab "))
			if len(f) == 2 {
				r.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[0], "+"))
				r.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[1], "-"))
			}
		case strings.HasPrefix(l, "? "):
			r.Untracked++
		case strings.HasPrefix(l, "1 "), strings.HasPrefix(l, "2 "), strings.HasPrefix(l, "u "):
			r.Changed++
		}
	}
}

func newestChange(ctx context.Context, dir string) time.Time {
	out, err := git(ctx, dir, "status", "--porcelain", "-uall")
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, l := range strings.Split(out, "\n") {
		if len(l) < 4 {
			continue
		}
		name := l[3:]
		if _, after, ok := strings.Cut(name, " -> "); ok {
			name = after
		}
		if st, err := os.Stat(filepath.Join(dir, strings.Trim(name, `"`))); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	return newest
}

func Scan(ctx context.Context, root string, now time.Time) ([]Repo, error) {
	dirs, err := Discover(root)
	if err != nil {
		return nil, err
	}
	out := make([]Repo, len(dirs))
	done := make(chan struct{}, len(dirs))
	for i, d := range dirs {
		go func() {
			out[i] = Inspect(ctx, d, now)
			done <- struct{}{}
		}()
	}
	for range dirs {
		<-done
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Touched.After(out[j].Touched) })
	return out, nil
}
