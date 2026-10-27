# Grafana Queries

Example PromQL queries for building Grafana dashboards with xp-tracker metrics.

For alerting rules and ready-to-use `PrometheusRule` examples, see [Prometheus Alerts](prometheus-alerts.md). A checked-in starter dashboard is available as [`xp-tracker-dashboard.json`](xp-tracker-dashboard.json).

## Import the starter dashboard

1. Ensure Grafana can query a Prometheus (or compatible) datasource that scrapes xp-tracker `/metrics`.
2. In Grafana: **Dashboards** → **New** → **Import**.
3. Upload `docs/metrics/xp-tracker-dashboard.json` from this repository (or paste its contents).
4. Select your Prometheus datasource when prompted, then import.

The dashboard covers claim/XR/MR inventory totals and readiness, store gauges (`xp_tracker_store_*`), poll duration/errors, and discovery self-metrics (`xp_tracker_discovery_*`, `xp_tracker_mrd_discovery_skipped_total`). Set the dashboard refresh close to your `POLL_INTERVAL_SECONDS` (default 30s).

Panels use a `${datasource}` template variable so the same JSON works across Grafana organisations without editing panel UIDs.

## Claim queries

### Total claims by namespace

```promql
sum by (namespace)(crossplane_claims_total)
```

### Ready claims by team

```promql
sum by (team)(crossplane_claims_ready)
```

### Claims by creator

```promql
sum by (creator)(crossplane_claims_total)
```

### Claim readiness ratio by namespace

```promql
sum by (namespace)(crossplane_claims_ready) / sum by (namespace)(crossplane_claims_total)
```

!!! tip
    Use this with a Grafana stat panel and percentage unit to show a readiness percentage per namespace.

### Not-ready claims by namespace

```promql
sum by (namespace)(crossplane_claims_total) - sum by (namespace)(crossplane_claims_ready)
```

### Claims by team

```promql
sum by (team)(crossplane_claims_total)
```

### Top creators by claim count

```promql
topk(10, sum by (creator)(crossplane_claims_total))
```

### Claims by synced status

```promql
sum by (synced)(crossplane_claims_total)
```

### Claims by ready status label

```promql
sum by (ready)(crossplane_claims_total)
```

### Per-claim status table

```promql
max by (namespace, claim_name, synced, ready)(crossplane_claims_total)
```

## XR queries

### All XRs grouped by kind

```promql
sum by (kind)(crossplane_xr_total)
```

### XR readiness ratio by kind

```promql
sum by (kind)(crossplane_xr_ready) / sum by (kind)(crossplane_xr_total)
```

### Not-ready XRs

```promql
sum(crossplane_xr_total) - sum(crossplane_xr_ready)
```

### XRs by synced status

```promql
sum by (synced)(crossplane_xr_total)
```

### XRs by ready status label

```promql
sum by (ready)(crossplane_xr_total)
```

### Per-XR status table

```promql
max by (name, parent_kind, parent_name, parent_namespace, claim_name, claim_namespace, synced, ready)(crossplane_xr_total)
```

### Root XRs only (no parent)

```promql
sum by (group, kind, namespace, name)(crossplane_xr_total{parent_name=""})
```

### Child XRs of a parent

```promql
sum by (kind, name)(
  crossplane_xr_total{parent_kind="XApp", parent_name="my-app", parent_namespace="default"}
)
```

### Nested XR tree by parent

```promql
sum by (parent_kind, parent_name, parent_namespace, kind)(crossplane_xr_total)
```

!!! tip
    Prefer `parent_*` for XR→XR nesting. Avoid hard-coded child-kind allowlists or name-suffix matching — those belong in the exporter, not the dashboard.

## MR queries

Modern ownership is **XR → MR**. Prefer `xr_name` / `xr_kind` / `xr_namespace` / `xr_group` (enabled by default via `MR_XR_PARENT_LABELS`). Do not require `claim_*` — those are empty on native Crossplane v2 trees.

### XR-linked MRs by kind

```promql
sum by (kind)(crossplane_mr_total)
```

### MRs by parent XR

```promql
sum by (xr_group, xr_kind, xr_namespace, xr_name)(crossplane_mr_total)
```

### MRs for one XR (typed parent)

```promql
max by (name, namespace, synced, ready)(
  crossplane_mr_total{xr_group="example.org", xr_kind="App", xr_namespace="default", xr_name="my-app"}
)
```

### MR readiness ratio by provider

```promql
sum by (provider)(crossplane_mr_ready) / sum by (provider)(crossplane_mr_total)
```

### Not-ready MRs

```promql
sum(crossplane_mr_total) - sum(crossplane_mr_ready)
```

### Legacy: MRs by claim (v1 claim path only)

```promql
sum by (claim_namespace, claim_name)(crossplane_mr_total{claim_name!=""})
```

## Self-monitoring queries

### Average poll duration

```promql
rate(xp_tracker_poll_duration_seconds_sum[5m]) / rate(xp_tracker_poll_duration_seconds_count[5m])
```

### Store size

```promql
xp_tracker_store_claims
xp_tracker_store_xrs
xp_tracker_store_mrs
```

### Discovery errors and MRD skips

```promql
rate(xp_tracker_discovery_runs_total{result="error"}[15m])
sum(xp_tracker_mrd_discovery_skipped_total)
```

## Combined queries

### Total inventory (claims + XRs + MRs)

```promql
sum(crossplane_claims_total) + sum(crossplane_xr_total) + sum(crossplane_mr_total)
```

### Overall readiness ratio (claims + XRs)

```promql
(sum(crossplane_claims_ready) + sum(crossplane_xr_ready)) / (sum(crossplane_claims_total) + sum(crossplane_xr_total))
```

## Dashboard tips

- Prefer the [starter dashboard JSON](xp-tracker-dashboard.json) for a quick inventory + exporter health view; extend panels from the queries above as needed.
- **Single stat panels** work well for readiness ratios and total counts.
- **Table panels** are useful for showing per-namespace or per-team breakdowns.
- **Time series panels** show trends over time -- useful for spotting claim growth or readiness drops.
- Set a refresh interval that matches your `POLL_INTERVAL_SECONDS` (default: 30s) for accurate data.
