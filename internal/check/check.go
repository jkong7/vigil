package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jkong7/vigil/internal/config"
)

type Result struct {
	Monitor    string        `json:"monitor"`
	Up         bool          `json:"up"`
	Latency    time.Duration `json:"latency_ns"`
	StatusCode int           `json:"status_code,omitempty"`
	Error      string        `json:"error,omitempty"`
	At         time.Time     `json:"at"`
}

type Prober interface {
	Probe(ctx context.Context, m config.Monitor) Result
}

type Probers map[string]Prober

func Default() Probers {
	return Probers{"http": NewHTTP(), "tcp": TCP{}}
}

func (p Probers) Probe(ctx context.Context, m config.Monitor) Result {
	pr, ok := p[m.Type]
	if !ok {
		return Result{Monitor: m.Name, Error: fmt.Sprintf("no prober for type %q", m.Type), At: time.Now()}
	}
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	return pr.Probe(ctx, m)
}

const maxBody = 1 << 20

type HTTP struct {
	Client *http.Client
}

func NewHTTP() HTTP {
	return HTTP{Client: &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}}
}

func (h HTTP) Probe(ctx context.Context, m config.Monitor) Result {
	r := Result{Monitor: m.Name, At: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Target, nil)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	req.Header.Set("User-Agent", "vigil/0.1 (+https://github.com/jkong7/vigil)")
	start := time.Now()
	resp, err := h.Client.Do(req)
	if err != nil {
		r.Latency = time.Since(start)
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	r.Latency = time.Since(start)
	r.StatusCode = resp.StatusCode
	switch {
	case err != nil:
		r.Error = err.Error()
	case resp.StatusCode != m.ExpectStatus:
		r.Error = fmt.Sprintf("status %d, want %d", resp.StatusCode, m.ExpectStatus)
	case m.Contains != "" && !strings.Contains(string(body), m.Contains):
		r.Error = fmt.Sprintf("body does not contain %q", m.Contains)
	default:
		r.Up = true
	}
	return r
}

type TCP struct{}

func (TCP) Probe(ctx context.Context, m config.Monitor) Result {
	r := Result{Monitor: m.Name, At: time.Now()}
	var d net.Dialer
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", m.Target)
	r.Latency = time.Since(start)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	conn.Close()
	r.Up = true
	return r
}
