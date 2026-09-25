# vigil

A self-hosted uptime monitor written in Go. It probes HTTP and TCP endpoints on a schedule and serves a status page. It also records history in Postgres, streams every check and incident to Kafka, and exports Prometheus metrics that ship with a Grafana dashboard.

```
            ┌────────────┐   results   ┌──────────┐  transitions  ┌───────────┐
monitors ──▶│ scheduler  │────────────▶│ tracker  │──────────────▶│ incidents │
            │ (jittered) │             │ (window) │               └─────┬─────┘
            └────────────┘             └────┬─────┘                     │
                                            │                           │
                  ┌──────────────┬──────────┼──────────────┬────────────┘
                  ▼              ▼          ▼              ▼
             Postgres        Kafka     Prometheus     status page / API
          (history, SLOs) (event stream) (/metrics)     (/, /api/*)
```

## Workbench mode

On your own machine, vigil also watches how you work. It reads your Claude Code sessions from `~/.claude` and your git repos under `~/dev`, plus any markdown checklists, and turns them into a dashboard:

- **What to build today**: a ranked list drawn from failing monitors, sessions you left mid-task (with a copyable `claude --resume` command), diverged, dirty or unpushed repos, and open `- [ ]` items in repos you have been active in
- **Live sessions**: every running Claude Code session, busy or idle, with its title and last prompt
- **Activity**: prompts per day for the last 14 days, stacked by the repo each session actually touched
- **Repos**: branch, ahead/behind, uncommitted changes, commits today and this week
- **Session history**: searchable, with prompts, tool usage, output tokens and a resume command
- **Backlog**: open checklist items from each repo's markdown and from extra files such as a capture inbox

```sh
make install     # builds, installs a LaunchAgent, serves http://127.0.0.1:7777
make open
make logs
make uninstall
```

The config lives at `~/.config/vigil/vigil.yaml` (seeded from `vigil.local.example.yaml`). Everything stays local: it binds to 127.0.0.1 and needs no database.

| Endpoint | Returns |
| --- | --- |
| `GET /api/today` | Ranked suggestions, 72h focus by repo, live and busy session counts |
| `GET /api/sessions?repo=&live=1&limit=` | Claude Code sessions, newest first |
| `GET /api/activity?days=` | Prompts and sessions per day, by repo |
| `GET /api/repos` | Git status for every repo under `repos_root` |
| `GET /api/backlog?all=1` | Checklist items (open only unless `all=1`) |

## Quick start (server mode)

```sh
make up          # vigil, Postgres, Redpanda, Prometheus, Grafana
open http://localhost:8080/status.html
open http://localhost:3000   # Grafana dashboard
```

Or run just the binary, with no database and no broker:

```sh
cp vigil.example.yaml vigil.yaml
make run
```

## Config

```yaml
listen: ":8080"
database_url: "${DATABASE_URL}"
kafka:
  brokers: ["${KAFKA_BROKER}"]
monitors:
  - name: api
    target: https://api.example.com/health
    interval: 30s
    timeout: 5s
    expect_status: 200
    contains: '"ok":true'
    fail_after: 2
  - name: db
    type: tcp
    target: db.internal:5432
```

`${VARS}` expand from the environment. Postgres and Kafka are both optional. Without them, vigil keeps state in memory and logs incidents.

An incident opens after `fail_after` consecutive failures and resolves on the next passing check.

## API

| Endpoint | Returns |
| --- | --- |
| `GET /` | Workbench dashboard (monitors only when the workbench is off) |
| `GET /status.html` | Public-style status page |
| `GET /api/monitors` | Current status, rolling uptime, average latency and recent results for every monitor |
| `GET /api/monitors/{name}` | Same, plus 24h/7d/30d uptime and incident history from Postgres |
| `GET /api/incidents?monitor=&limit=` | Incidents, newest first |
| `GET /metrics` | Prometheus metrics |
| `GET /healthz` | Liveness |

## Events

Kafka topics, keyed by monitor name so each monitor's events stay ordered:

- `vigil.checks`: one `check.completed` event per probe
- `vigil.incidents`: `incident.opened` and `incident.resolved`

## Metrics

`vigil_monitor_up`, `vigil_check_duration_seconds` (histogram), `vigil_checks_total{outcome}`, `vigil_incidents_total`.

## Development

```sh
make test        # unit tests; store tests skip without a database
make test-db     # full suite against Postgres (VIGIL_TEST_DATABASE_URL)
```

## Roadmap

- [ ] Terraform module to deploy on AWS (ECS Fargate, RDS, MSK Serverless)
- [ ] Alerting consumers for Slack, email and webhooks, reading `vigil.incidents`
- [ ] Multi-region probes that agree before opening an incident
- [ ] OpenTelemetry traces for each probe
- [ ] SSL certificate expiry and DNS checks
- [ ] Menu bar item with live session count and the top suggestion
- [ ] Daily digest of yesterday's sessions and commits
- [ ] Monitors managed through the API instead of the config file
