package store

import (
	"context"
	"embed"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/state"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	return s, s.migrate(ctx)
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) migrate(ctx context.Context) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		sql, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, string(sql)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveResult(ctx context.Context, r check.Result) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO check_results (monitor, up, latency_ms, status_code, error, checked_at)
		 VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, ''), $6)`,
		r.Monitor, r.Up, float64(r.Latency.Microseconds())/1000, r.StatusCode, r.Error, r.At)
	return err
}

func (s *Store) OpenIncident(ctx context.Context, i state.Incident) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO incidents (monitor, cause, started_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		i.Monitor, i.Cause, i.Started)
	return err
}

func (s *Store) ResolveIncident(ctx context.Context, i state.Incident) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE incidents SET resolved_at = $2 WHERE monitor = $1 AND resolved_at IS NULL`,
		i.Monitor, i.Resolved)
	return err
}

func (s *Store) Incidents(ctx context.Context, monitor string, limit int) ([]state.Incident, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT monitor, cause, started_at, resolved_at FROM incidents
		 WHERE $1 = '' OR monitor = $1 ORDER BY started_at DESC LIMIT $2`, monitor, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (state.Incident, error) {
		var i state.Incident
		err := row.Scan(&i.Monitor, &i.Cause, &i.Started, &i.Resolved)
		return i, err
	})
}

func (s *Store) Uptime(ctx context.Context, monitor string, since time.Duration) (float64, int, error) {
	var ratio *float64
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT avg(up::int)::float8, count(*) FROM check_results
		 WHERE monitor = $1 AND checked_at > now() - $2::interval`,
		monitor, since.String()).Scan(&ratio, &n)
	if ratio == nil {
		return 0, n, err
	}
	return *ratio, n, err
}

func (s *Store) Prune(ctx context.Context, keep time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM check_results WHERE checked_at < now() - $1::interval`, keep.String())
	return tag.RowsAffected(), err
}
