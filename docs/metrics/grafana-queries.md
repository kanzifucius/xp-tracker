# Grafana Queries

Example PromQL queries for building Grafana dashboards with xp-tracker metrics.

For alerting rules and ready-to-use `PrometheusRule` examples, see [Prometheus Alerts](prometheus-alerts.md).

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
max by (name, claim_name, claim_namespace, synced, ready)(crossplane_xr_total)
```

## Combined queries

### Total managed resources (claims + XRs)

```promql
sum(crossplane_claims_total) + sum(crossplane_xr_total)
```

### Overall readiness ratio

```promql
(sum(crossplane_claims_ready) + sum(crossplane_xr_ready)) / (sum(crossplane_claims_total) + sum(crossplane_xr_total))
```

## Dashboard tips

- **Single stat panels** work well for readiness ratios and total counts.
- **Table panels** are useful for showing per-namespace or per-team breakdowns.
- **Time series panels** show trends over time -- useful for spotting claim growth or readiness drops.
- Set a refresh interval that matches your `POLL_INTERVAL_SECONDS` (default: 30s) for accurate data.
