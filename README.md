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

## Quick start

```sh
make up          # vigil, Postgres, Redpanda, Prometheus, Grafana
open http://localhost:8080   # status page
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
- [ ] Monitors managed through the API instead of the config file
