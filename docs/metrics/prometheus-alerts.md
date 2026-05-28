# Prometheus Alerts

This page provides practical alert ideas for xp-tracker and a ready-to-use `PrometheusRule` example.

Use these as a starting point, then tune thresholds and `for` durations to match your cluster size, poll interval, and on-call expectations.

## Alerting goals

A useful alert set should cover:

- Exporter pipeline health (is xp-tracker collecting reliably?)
- Data freshness (are metrics still being updated?)
- Resource health signals (are claims/XRs becoming not-ready?)

## Suggested severities

- **warning**: potential degradation; investigate during working hours
- **critical**: clear outage risk or sustained failure; page immediately

## Example PrometheusRule

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: xp-tracker-alerts
  namespace: crossplane-system
  labels:
    app.kubernetes.io/name: crossplane-metrics-exporter
    app.kubernetes.io/component: exporter
spec:
  groups:
    - name: xp-tracker-self
      rules:
        - alert: XpTrackerPollErrorsHigh
          expr: increase(xp_tracker_poll_errors_total[10m]) > 0
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: xp-tracker poll errors detected
            description: |
              xp-tracker has reported at least one poll error in the last 10 minutes.
              Check exporter logs and RBAC/discovery access.

        - alert: XpTrackerPollErrorsSustained
          expr: increase(xp_tracker_poll_errors_total[30m]) > 10
          for: 15m
          labels:
            severity: critical
          annotations:
            summary: xp-tracker poll errors are sustained
            description: |
              xp-tracker continues to fail polling and may be serving stale or incomplete data.

        - alert: XpTrackerPollDurationP99High
          expr: histogram_quantile(0.99, sum by (le) (rate(xp_tracker_poll_duration_seconds_bucket[15m]))) > 20
          for: 15m
          labels:
            severity: warning
          annotations:
            summary: xp-tracker poll duration p99 is high
            description: |
              Poll cycles are slow compared with the default 30s interval.
              Consider reducing scope, increasing poll interval, or checking API server load.

        - alert: XpTrackerMetricsMissing
          expr: absent(xp_tracker_store_claims)
          for: 10m
          labels:
            severity: critical
          annotations:
            summary: xp-tracker metrics are missing
            description: |
              Prometheus cannot find xp-tracker self metrics. Scrape or exporter may be down.

    - name: xp-tracker-resource-health
      rules:
        - alert: XpTrackerClaimReadinessLow
          expr: (sum(crossplane_claims_ready) / sum(crossplane_claims_total)) < 0.8
          for: 20m
          labels:
            severity: warning
          annotations:
            summary: claim readiness ratio is low
            description: |
              Fewer than 80% of claims are ready for at least 20 minutes.

        - alert: XpTrackerClaimReadinessCritical
          expr: (sum(crossplane_claims_ready) / sum(crossplane_claims_total)) < 0.5
          for: 15m
          labels:
            severity: critical
          annotations:
            summary: claim readiness ratio is critically low
            description: |
              Fewer than 50% of claims are ready.

        - alert: XpTrackerXRReadinessLow
          expr: (sum(crossplane_xr_ready) / sum(crossplane_xr_total)) < 0.8
          for: 20m
          labels:
            severity: warning
          annotations:
            summary: XR readiness ratio is low
            description: |
              Fewer than 80% of XRs are ready for at least 20 minutes.

        - alert: XpTrackerNotReadyClaimsHigh
          expr: (sum(crossplane_claims_total) - sum(crossplane_claims_ready)) > 25
          for: 15m
          labels:
            severity: warning
          annotations:
            summary: high number of not-ready claims
            description: |
              More than 25 claims are currently not ready.
```

## Tuning guidance

- Tune readiness thresholds per environment size:
  - small clusters may alert on absolute counts
  - larger clusters often work better with ratios
- Start with longer `for` windows (10-20m) to reduce noise from short transitions.
- If `POLL_INTERVAL_SECONDS` is above 30s, increase `for` windows proportionally.
- Route `warning` alerts to team channels and `critical` alerts to paging.

## Caveats for status-in-label metrics

`crossplane_claims_total` and `crossplane_xr_total` include `synced` and `ready` labels. When a resource changes state, it can produce a different label tuple.

For alerting, prefer:

- ratio/aggregate expressions (`sum(...)`)
- explicit status metrics (`crossplane_*_status_ready`, `crossplane_*_status_synced`) when useful

Avoid very high-cardinality alert dimensions unless required.
