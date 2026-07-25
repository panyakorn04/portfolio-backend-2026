# Production observability baseline

This document defines the initial production objectives and the evidence required before the backend is considered deployment-ready. It is an operating baseline, not a claim that the objectives have already been achieved.

## Service-level indicators and objectives

| Signal | Indicator | Initial objective | 30-day budget |
|---|---|---:|---:|
| API availability | Non-5xx completed HTTP requests / all completed HTTP requests, excluding `/api/health` | 99.90% | 43.2 minutes equivalent unavailability |
| Readiness | Successful `/api/ready` probes / all readiness probes | 99.95% | 21.6 minutes equivalent unavailability |
| API latency | `duration_ms` from `http.request.completed` for non-streaming API routes | p95 <= 1,000 ms over 5 minutes | Alert after 3 consecutive breached windows |
| Monitor freshness | Successful production-log-monitor heartbeat | At least one heartbeat every 10 minutes | Alert after 15 minutes without a heartbeat |
| AI stream durability | `RUN_FINISHED` only after atomic exchange persistence for persisted sessions | 100% | No error budget; correctness invariant |

SSE routes are excluded from the request-latency objective because `duration_ms` includes the full stream lifetime. Track their first-token latency separately before defining an SSE latency SLO.

## Current telemetry

The HTTP middleware emits structured, allowlisted dimensions:

- `event=http.request.completed`
- `request_id`
- HTTP `method`
- normalized route pattern, never a raw visitor path
- response `status`
- `duration_ms`

Grafana Alloy ships only the `api`/`backend` Compose service logs to Loki. Alloy reaches Docker through `tecnativa/docker-socket-proxy`; Alloy itself does not mount the host Docker socket. The proxy allows only container, network-discovery, and event GET APIs and rejects Docker POST requests. Both containers run read-only and without Linux capabilities. The proxy is attached only to an internal network; Alloy is dual-homed onto that private discovery network and a separate egress network so it can reach Grafana Cloud.

## Alerting behavior

The production log monitor:

1. queries all structured logs since the prior durable checkpoint, with bounded pagination;
2. alerts on HTTP 5xx, error/fatal/panic levels, and explicit failed events;
3. sends only allowlisted aggregate dimensions to the optional AI summarizer;
4. preserves incident state in a short-lived GitHub artifact;
5. emits a success heartbeat after the state artifact is persisted.

Configure `MONITOR_HEARTBEAT_URL` as a GitHub Actions secret backed by an external dead-man-switch service. Configure that service to alert after 15 minutes without a ping. GitHub scheduled workflows are best-effort and cannot monitor their own absence.

## Deployment gates

Before production GO:

- `docker compose config` must validate the Alloy stack with required environment values.
- The socket proxy and Alloy containers must start healthy on a disposable Docker host.
- A proxy probe must confirm Docker GET access needed by Alloy and rejection of POST requests.
- A scheduled monitor run must persist state and successfully ping the external heartbeat.
- Grafana/Loki must show normalized `http.request.completed` records without raw URLs, credentials, request bodies, or user identifiers.

## Incident response

- A new fingerprint sends a firing alert immediately.
- Changed aggregate dimensions send a changed alert.
- Unresolved incidents remind every 30 minutes.
- A clean subsequent window sends a resolved alert.
- Loki query failure sends a deterministic monitor-failure alert and does not overwrite incident state.

Production deployment remains NO-GO when the monitor heartbeat is not configured or the Alloy proxy has not been exercised against a running Docker daemon.
