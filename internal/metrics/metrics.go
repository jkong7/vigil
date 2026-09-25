package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/state"
)

type Metrics struct {
	Registry  *prometheus.Registry
	up        *prometheus.GaugeVec
	latency   *prometheus.HistogramVec
	checks    *prometheus.CounterVec
	incidents *prometheus.CounterVec
}

func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "vigil_monitor_up", Help: "1 if the last check passed, 0 otherwise.",
		}, []string{"monitor"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "vigil_check_duration_seconds", Help: "Check latency.",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"monitor"}),
		checks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "vigil_checks_total", Help: "Checks run, by outcome.",
		}, []string{"monitor", "outcome"}),
		incidents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "vigil_incidents_total", Help: "Incidents opened.",
		}, []string{"monitor"}),
	}
	m.Registry.MustRegister(m.up, m.latency, m.checks, m.incidents,
		prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	return m
}

func (m *Metrics) ObserveResult(r check.Result) {
	outcome, up := "down", 0.0
	if r.Up {
		outcome, up = "up", 1
	}
	m.up.WithLabelValues(r.Monitor).Set(up)
	m.latency.WithLabelValues(r.Monitor).Observe(r.Latency.Seconds())
	m.checks.WithLabelValues(r.Monitor, outcome).Inc()
}

func (m *Metrics) ObserveIncident(i state.Incident) {
	m.incidents.WithLabelValues(i.Monitor).Inc()
}
