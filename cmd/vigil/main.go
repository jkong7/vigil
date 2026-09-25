package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jkong7/vigil/internal/api"
	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/config"
	"github.com/jkong7/vigil/internal/events"
	"github.com/jkong7/vigil/internal/metrics"
	"github.com/jkong7/vigil/internal/scheduler"
	"github.com/jkong7/vigil/internal/state"
	"github.com/jkong7/vigil/internal/store"
)

const window = 1440

func main() {
	path := flag.String("config", "vigil.yaml", "path to config file")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(*path, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(path string, log *slog.Logger) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracker := state.NewTracker(cfg.Monitors, window)
	m := metrics.New()
	srv := &api.Server{Tracker: tracker, Registry: m.Registry}

	var db *store.Store
	if cfg.DatabaseURL != "" {
		if db, err = store.Open(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
		defer db.Close()
		srv.History = db
	}

	var pub events.Publisher = events.Log{Logger: log}
	if len(cfg.Kafka.Brokers) > 0 && cfg.Kafka.Brokers[0] != "" {
		pub = events.NewKafka(cfg.Kafka.Brokers, cfg.Kafka.ChecksTopic, cfg.Kafka.IncidentsTopic)
	}
	defer pub.Close()

	results := make(chan check.Result, 64)
	go scheduler.New(check.Default()).Run(ctx, cfg.Monitors, results)
	go consume(ctx, log, results, tracker, m, db, pub)
	if db != nil {
		go prune(ctx, log, db)
	}

	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdown)
	}()
	log.Info("vigil listening", "addr", cfg.Listen, "monitors", len(cfg.Monitors), "postgres", db != nil)
	if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func consume(ctx context.Context, log *slog.Logger, results <-chan check.Result, tracker *state.Tracker,
	m *metrics.Metrics, db *store.Store, pub events.Publisher) {
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-results:
			m.ObserveResult(r)
			opened, resolved := tracker.Observe(r)
			if db != nil {
				if err := db.SaveResult(ctx, r); err != nil {
					log.Warn("save result", "monitor", r.Monitor, "err", err)
				}
			}
			if err := pub.Publish(ctx, events.FromResult(r)); err != nil {
				log.Warn("publish result", "monitor", r.Monitor, "err", err)
			}
			for _, inc := range []*state.Incident{opened, resolved} {
				if inc != nil {
					handleIncident(ctx, log, *inc, m, db, pub)
				}
			}
		}
	}
}

func handleIncident(ctx context.Context, log *slog.Logger, inc state.Incident, m *metrics.Metrics, db *store.Store,
	pub events.Publisher) {
	if inc.Resolved == nil {
		m.ObserveIncident(inc)
		log.Warn("incident opened", "monitor", inc.Monitor, "cause", inc.Cause)
	} else {
		log.Info("incident resolved", "monitor", inc.Monitor, "after", inc.Resolved.Sub(inc.Started).String())
	}
	if db != nil {
		var err error
		if inc.Resolved == nil {
			err = db.OpenIncident(ctx, inc)
		} else {
			err = db.ResolveIncident(ctx, inc)
		}
		if err != nil {
			log.Warn("save incident", "monitor", inc.Monitor, "err", err)
		}
	}
	if err := pub.Publish(ctx, events.FromIncident(inc)); err != nil {
		log.Warn("publish incident", "monitor", inc.Monitor, "err", err)
	}
}

func prune(ctx context.Context, log *slog.Logger, db *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := db.Prune(ctx, 90*24*time.Hour); err != nil {
				log.Warn("prune", "err", err)
			} else if n > 0 {
				log.Info("pruned old results", "rows", n)
			}
		}
	}
}
