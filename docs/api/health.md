# Health Endpoints

xp-tracker exposes health and readiness endpoints for Kubernetes probes.

## `GET /healthz`

Liveness probe. Always returns `200 OK` with body `ok`.

Use this to detect if the process is alive. The HTTP server binds on startup and responds immediately -- no dependencies on Kubernetes API or store state.

## `GET /readyz`

Readiness probe. Returns `503 Service Unavailable` until the first poll cycle completes, then `200 OK` with body `ok`.

This prevents Kubernetes from sending traffic (including Prometheus scrapes) to the exporter before it has populated the in-memory store. Without this, Prometheus would scrape empty metrics on startup.

## Deployment probes

The base Deployment manifests configure both probes:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: metrics
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /readyz
    port: metrics
  initialDelaySeconds: 5
  periodSeconds: 10
```

!!! tip
    If your first poll cycle takes longer than the default readiness probe timeout (e.g., large number of GVRs or slow API server), increase `initialDelaySeconds` or `POLL_INTERVAL_SECONDS` accordingly.
