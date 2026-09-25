package scheduler

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/config"
)

type Prober interface {
	Probe(ctx context.Context, m config.Monitor) check.Result
}

type Scheduler struct {
	Prober Prober
	Jitter func(time.Duration) time.Duration
}

func New(p Prober) *Scheduler {
	return &Scheduler{Prober: p, Jitter: func(d time.Duration) time.Duration {
		return time.Duration(rand.Int64N(int64(d)))
	}}
}

func (s *Scheduler) Run(ctx context.Context, monitors []config.Monitor, out chan<- check.Result) {
	var wg sync.WaitGroup
	for _, m := range monitors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.loop(ctx, m, out)
		}()
	}
	wg.Wait()
}

func (s *Scheduler) loop(ctx context.Context, m config.Monitor, out chan<- check.Result) {
	delay := s.Jitter(m.Interval)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		r := s.Prober.Probe(ctx, m)
		if ctx.Err() != nil {
			return
		}
		select {
		case out <- r:
		case <-ctx.Done():
			return
		}
		timer.Reset(m.Interval)
	}
}
