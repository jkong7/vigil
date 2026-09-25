package api

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jkong7/vigil/internal/state"
)

//go:embed web
var web embed.FS

type Tracker interface {
	All() []state.Snapshot
	Get(name string) (state.Snapshot, bool)
}

type History interface {
	Incidents(ctx context.Context, monitor string, limit int) ([]state.Incident, error)
	Uptime(ctx context.Context, monitor string, since time.Duration) (float64, int, error)
}

type Server struct {
	Tracker  Tracker
	History  History
	Registry *prometheus.Registry
}

var windows = []struct {
	Label string
	Span  time.Duration
}{{"24h", 24 * time.Hour}, {"7d", 7 * 24 * time.Hour}, {"30d", 30 * 24 * time.Hour}}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web, "web")
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/monitors", s.monitors)
	mux.HandleFunc("GET /api/monitors/{name}", s.monitor)
	mux.HandleFunc("GET /api/incidents", s.incidents)
	if s.Registry != nil {
		mux.Handle("GET /metrics", promhttp.HandlerFor(s.Registry, promhttp.HandlerOpts{}))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) monitors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Tracker.All())
}

type detail struct {
	state.Snapshot
	History   map[string]float64 `json:"history,omitempty"`
	Incidents []state.Incident   `json:"incidents,omitempty"`
}

func (s *Server) monitor(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.Tracker.Get(r.PathValue("name"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such monitor"})
		return
	}
	d := detail{Snapshot: snap}
	if s.History != nil {
		d.History = map[string]float64{}
		for _, win := range windows {
			if ratio, n, err := s.History.Uptime(r.Context(), snap.Monitor.Name, win.Span); err == nil && n > 0 {
				d.History[win.Label] = ratio
			}
		}
		d.Incidents, _ = s.History.Incidents(r.Context(), snap.Monitor.Name, 20)
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	if s.History == nil {
		writeJSON(w, http.StatusOK, []state.Incident{})
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 500 {
		limit = 50
	}
	list, err := s.History.Incidents(r.Context(), r.URL.Query().Get("monitor"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []state.Incident{}
	}
	writeJSON(w, http.StatusOK, list)
}
