package workbench

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jkong7/vigil/internal/backlog"
	"github.com/jkong7/vigil/internal/claude"
	"github.com/jkong7/vigil/internal/config"
	"github.com/jkong7/vigil/internal/repos"
	"github.com/jkong7/vigil/internal/state"
	"github.com/jkong7/vigil/internal/today"
)

type Monitors interface {
	All() []state.Snapshot
}

type Workbench struct {
	cfg      config.Workbench
	index    *claude.Index
	monitors Monitors
	now      func() time.Time

	mu        sync.RWMutex
	sessions  []claude.Session
	repos     []repos.Repo
	backlog   []backlog.Item
	refreshed time.Time
}

func New(cfg config.Workbench, monitors Monitors) *Workbench {
	return &Workbench{cfg: cfg, index: claude.NewIndex(cfg.ClaudeDir, cfg.ReposRoot), monitors: monitors, now: time.Now}
}

func (w *Workbench) Refresh(ctx context.Context) error {
	if err := w.index.Refresh(); err != nil {
		return err
	}
	now := w.now()
	list, err := repos.Scan(ctx, w.cfg.ReposRoot, now)
	if err != nil {
		return err
	}
	dirs := make([]string, len(list))
	for i, r := range list {
		dirs[i] = r.Path
	}
	items := backlog.Scan(dirs, w.cfg.BacklogFiles)
	sessions := w.index.Sessions()
	w.mu.Lock()
	w.sessions, w.repos, w.backlog, w.refreshed = sessions, list, items, now
	w.mu.Unlock()
	return nil
}

func (w *Workbench) Run(ctx context.Context, log *slog.Logger) {
	t := time.NewTicker(w.cfg.Refresh)
	defer t.Stop()
	for {
		if err := w.Refresh(ctx); err != nil {
			log.Warn("workbench refresh", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (w *Workbench) Sessions() []claude.Session {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sessions
}

func (w *Workbench) Repos() []repos.Repo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.repos
}

func (w *Workbench) Backlog() []backlog.Item {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.backlog
}

func (w *Workbench) Activity(days int) []claude.Day {
	return claude.Activity(w.Sessions(), days, w.now())
}

func (w *Workbench) Plan() today.Plan {
	w.mu.RLock()
	in := today.Input{Now: w.now(), Sessions: w.sessions, Repos: w.repos, Backlog: w.backlog}
	w.mu.RUnlock()
	if w.monitors != nil {
		in.Monitors = w.monitors.All()
	}
	return today.Build(in)
}
