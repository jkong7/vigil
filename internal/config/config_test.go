package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseAppliesDefaults(t *testing.T) {
	c, err := Parse([]byte("monitors:\n  - name: site\n    target: https://example.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := c.Monitors[0]
	if m.Type != "http" || m.Interval != 30*time.Second || m.Timeout != 10*time.Second || m.ExpectStatus != 200 || m.FailAfter != 2 {
		t.Fatalf("defaults not applied: %+v", m)
	}
	if c.Listen != ":8080" || c.Kafka.ChecksTopic != "vigil.checks" {
		t.Fatalf("top-level defaults not applied: %+v", c)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]string{
		"no name":   "monitors:\n  - target: x\n",
		"duplicate": "monitors:\n  - {name: a, target: x}\n  - {name: a, target: y}\n",
		"bad type":  "monitors:\n  - {name: a, type: icmp, target: x}\n",
		"timeout":   "monitors:\n  - {name: a, target: x, interval: 5s, timeout: 5s}\n",
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestWorkbenchDefaultsExpandHome(t *testing.T) {
	c, err := Parse([]byte("workbench:\n  enabled: true\n  backlog_files:\n    inbox: ~/inbox.md\n"))
	if err != nil {
		t.Fatal(err)
	}
	w := c.Workbench
	if !w.Enabled || w.Refresh != 30*time.Second || strings.HasPrefix(w.ClaudeDir, "~") || !strings.HasSuffix(w.ReposRoot, "/dev") {
		t.Fatalf("workbench defaults: %+v", w)
	}
	if strings.HasPrefix(w.BacklogFiles["inbox"], "~") {
		t.Fatalf("backlog file not expanded: %v", w.BacklogFiles)
	}
}
