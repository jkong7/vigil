package workbench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/config"
	"github.com/jkong7/vigil/internal/today"
)

func TestRefreshAndPlan(t *testing.T) {
	root := t.TempDir()
	claudeDir := filepath.Join(root, ".claude")
	reposRoot := filepath.Join(root, "dev")
	app := filepath.Join(reposRoot, "app")
	os.MkdirAll(app, 0o755)
	os.MkdirAll(filepath.Join(claudeDir, "projects", "p"), 0o755)
	cmd := exec.Command("git", "init", "-q", app)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	os.WriteFile(filepath.Join(app, "README.md"), []byte("## Next\n- [ ] ship it\n"), 0o644)
	now := time.Now().UTC()
	ts := now.Add(-time.Minute).Format(time.RFC3339)
	transcript := `{"type":"user","sessionId":"s1","cwd":"` + reposRoot + `","timestamp":"` + ts + `","origin":{"kind":"human"},"message":{"content":"build the thing"}}
{"type":"assistant","sessionId":"s1","timestamp":"` + ts + `","message":{"model":"m","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"` + app + `/main.go"}}]}}
{"type":"user","sessionId":"s1","timestamp":"` + ts + `","origin":{"kind":"human"},"message":{"content":"keep going"}}
`
	os.WriteFile(filepath.Join(claudeDir, "projects", "p", "s1.jsonl"), []byte(transcript), 0o644)

	w := New(config.Workbench{ClaudeDir: claudeDir, ReposRoot: reposRoot}, nil)
	if err := w.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(w.Sessions()) != 1 || len(w.Repos()) != 1 || len(w.Backlog()) != 1 {
		t.Fatalf("sessions=%d repos=%d backlog=%d", len(w.Sessions()), len(w.Repos()), len(w.Backlog()))
	}
	p := w.Plan()
	seen := map[today.Kind]bool{}
	for _, s := range p.Suggestions {
		seen[s.Kind] = true
	}
	if !seen[today.Resume] || !seen[today.Commit] || !seen[today.Backlog] {
		t.Fatalf("plan missing kinds: %+v", p.Suggestions)
	}
	if p.Focus["app"] != 2 {
		t.Fatalf("focus = %v", p.Focus)
	}
	if days := w.Activity(7); len(days) != 7 || days[6].Prompts != 2 {
		t.Fatalf("activity = %+v", days)
	}
}
