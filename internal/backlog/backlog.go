package backlog

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Item struct {
	Repo     string    `json:"repo"`
	File     string    `json:"file"`
	Line     int       `json:"line"`
	Section  string    `json:"section,omitempty"`
	Text     string    `json:"text"`
	Done     bool      `json:"done"`
	Weight   int       `json:"weight"`
	Captured time.Time `json:"captured,omitempty"`
}

var (
	checkbox = regexp.MustCompile(`^\s*[-*+] \[([ xX])\]\s+(.+?)\s*$`)
	heading  = regexp.MustCompile(`^(#{1,6}|\*\*)\s*(.+?)\s*(\*\*)?$`)
	stamp    = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2})\s+`)
	soon     = regexp.MustCompile(`(?i)\b(next|now|today|todo|up next|this week)\b`)
	later    = regexp.MustCompile(`(?i)\b(later|someday|maybe|ideas?|icebox)\b|^later:`)
)

func weight(section, text string) int {
	switch {
	case later.MatchString(text) || later.MatchString(section):
		return 1
	case soon.MatchString(section):
		return 3
	}
	return 2
}

func Parse(r io.Reader, repo, file string) []Item {
	var out []Item
	section := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	fence := false
	for n := 1; sc.Scan(); n++ {
		l := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if m := checkbox.FindStringSubmatch(l); m != nil {
			it := Item{Repo: repo, File: file, Line: n, Section: section, Text: m[2], Done: m[1] != " "}
			if s := stamp.FindStringSubmatch(it.Text); s != nil {
				if t, err := time.ParseInLocation("2006-01-02 15:04", s[1], time.Local); err == nil {
					it.Captured = t
					it.Text = strings.TrimPrefix(it.Text, s[0])
				}
			}
			it.Weight = weight(section, it.Text)
			out = append(out, it)
			continue
		}
		if m := heading.FindStringSubmatch(strings.TrimSpace(l)); m != nil && (m[1] != "**" || m[3] == "**") {
			section = strings.Trim(m[2], "*: ")
		}
	}
	return out
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	"dossiers": true, "testdata": true, ".venv": true, "venv": true}

func markdownFiles(repo string, depth int) []string {
	var out []string
	var walk func(dir string, d int)
	walk = func(dir string, d int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if d < depth && !skipDirs[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
					walk(p, d+1)
				}
				continue
			}
			if strings.EqualFold(filepath.Ext(e.Name()), ".md") {
				out = append(out, p)
			}
		}
	}
	walk(repo, 0)
	return out
}

func parseFile(path, repo, rel string) []Item {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return Parse(f, repo, rel)
}

func Scan(repoDirs []string, extra map[string]string) []Item {
	var out []Item
	for _, dir := range repoDirs {
		name := filepath.Base(dir)
		for _, p := range markdownFiles(dir, 1) {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, parseFile(p, name, rel)...)
		}
	}
	for label, p := range extra {
		out = append(out, parseFile(p, label, filepath.Base(p))...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func Open(items []Item) []Item {
	var out []Item
	for _, it := range items {
		if !it.Done {
			out = append(out, it)
		}
	}
	return out
}
