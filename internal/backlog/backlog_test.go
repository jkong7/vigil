package backlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const doc = `# app

## Roadmap

- [ ] Terraform module
- [x] Status page
* [ ] Later: voice dictation

**Next**
- [ ] Wire the statusline

` + "```md\n- [ ] not a real item\n```\n" + `
### Ideas
- [ ] 2026-09-21 14:03 email prof about extension
`

func TestParse(t *testing.T) {
	items := Parse(strings.NewReader(doc), "app", "README.md")
	if len(items) != 5 {
		t.Fatalf("want 5 items, got %d: %+v", len(items), items)
	}
	want := []struct {
		text, section string
		done          bool
		weight        int
	}{
		{"Terraform module", "Roadmap", false, 2},
		{"Status page", "Roadmap", true, 2},
		{"Later: voice dictation", "Roadmap", false, 1},
		{"Wire the statusline", "Next", false, 3},
		{"email prof about extension", "Ideas", false, 1},
	}
	for i, w := range want {
		it := items[i]
		if it.Text != w.text || it.Section != w.section || it.Done != w.done || it.Weight != w.weight {
			t.Errorf("item %d = %+v, want %+v", i, it, w)
		}
	}
	if items[4].Captured.IsZero() || items[0].Line != 5 {
		t.Errorf("captured/line not parsed: %+v %+v", items[4], items[0])
	}
}

func TestScanSkipsNoiseAndAddsExtras(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	os.MkdirAll(filepath.Join(repo, "docs"), 0o755)
	os.MkdirAll(filepath.Join(repo, "node_modules", "x"), 0o755)
	os.MkdirAll(filepath.Join(repo, "docs", "deep"), 0o755)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("- [ ] a\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "docs", "plan.md"), []byte("- [ ] b\n- [x] c\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "docs", "deep", "x.md"), []byte("- [ ] too deep\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "node_modules", "x", "README.md"), []byte("- [ ] vendored\n"), 0o644)
	inbox := filepath.Join(root, "inbox.md")
	os.WriteFile(inbox, []byte("- [ ] 2026-09-25 09:00 call the bank\n"), 0o644)

	items := Scan([]string{repo}, map[string]string{"inbox": inbox})
	open := Open(items)
	var texts []string
	for _, it := range open {
		texts = append(texts, it.Repo+":"+it.Text)
	}
	if strings.Join(texts, ",") != "app:a,app:b,inbox:call the bank" {
		t.Fatalf("open items = %v", texts)
	}
}
