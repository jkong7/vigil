package check

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/config"
)

func monitor(typ, target string) config.Monitor {
	return config.Monitor{Name: "t", Type: typ, Target: target, Timeout: time.Second, ExpectStatus: 200}
}

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte("all systems go"))
		case "/slow":
			time.Sleep(200 * time.Millisecond)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	p := Default()

	cases := []struct {
		name     string
		path     string
		contains string
		timeout  time.Duration
		up       bool
	}{
		{"ok", "/ok", "", time.Second, true},
		{"contains match", "/ok", "systems go", time.Second, true},
		{"contains miss", "/ok", "outage", time.Second, false},
		{"bad status", "/down", "", time.Second, false},
		{"timeout", "/slow", "", 50 * time.Millisecond, false},
	}
	for _, c := range cases {
		m := monitor("http", srv.URL+c.path)
		m.Contains, m.Timeout = c.contains, c.timeout
		r := p.Probe(context.Background(), m)
		if r.Up != c.up {
			t.Errorf("%s: up=%v want %v (err=%q)", c.name, r.Up, c.up, r.Error)
		}
		if !r.Up && r.Error == "" {
			t.Errorf("%s: down result has no error", c.name)
		}
	}
}

func TestTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	p := Default()
	if r := p.Probe(context.Background(), monitor("tcp", addr)); !r.Up {
		t.Fatalf("open port reported down: %s", r.Error)
	}
	ln.Close()
	if r := p.Probe(context.Background(), monitor("tcp", addr)); r.Up {
		t.Fatal("closed port reported up")
	}
}

func TestUnknownType(t *testing.T) {
	if r := Default().Probe(context.Background(), monitor("icmp", "x")); r.Up || r.Error == "" {
		t.Fatalf("unknown type should fail: %+v", r)
	}
}
