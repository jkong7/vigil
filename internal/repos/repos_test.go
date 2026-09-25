package repos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", args, out)
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(t.TempDir(), "origin.git")
	run(t, root, "git", "init", "-q", "--bare", origin)
	app := filepath.Join(root, "app")
	os.Mkdir(app, 0o755)
	run(t, app, "git", "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(app, "a.txt"), []byte("a"), 0o644)
	run(t, app, "git", "add", ".")
	run(t, app, "git", "commit", "-qm", "first")
	run(t, app, "git", "remote", "add", "origin", origin)
	run(t, app, "git", "push", "-q", "-u", "origin", "main")
	os.WriteFile(filepath.Join(app, "b.txt"), []byte("b"), 0o644)
	run(t, app, "git", "add", ".")
	run(t, app, "git", "commit", "-qm", "second")
	os.WriteFile(filepath.Join(app, "a.txt"), []byte("changed"), 0o644)
	os.WriteFile(filepath.Join(app, "new.txt"), []byte("n"), 0o644)
	os.Mkdir(filepath.Join(root, "not-a-repo"), 0o755)

	list, err := Scan(context.Background(), root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 repo, got %+v", list)
	}
	r := list[0]
	if r.Name != "app" || r.Branch != "main" || !r.Upstream || r.Ahead != 1 || r.Behind != 0 {
		t.Fatalf("branch info: %+v", r)
	}
	if r.Changed != 1 || r.Untracked != 1 || !r.Dirty() {
		t.Fatalf("dirty info: %+v", r)
	}
	if r.LastSubject != "second" || r.CommitsToday != 2 || r.CommitsWeek != 2 || r.Remote != origin {
		t.Fatalf("log info: %+v", r)
	}
	if r.Touched.Before(r.LastCommit) {
		t.Fatalf("touched should include working-tree edits: %+v", r)
	}
}
