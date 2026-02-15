# Grafana Queries

Example PromQL queries for building Grafana dashboards with xp-tracker metrics.

## Claim queries

### Total claims by namespace

```promql
sum by (namespace)(crossplane_claims_total)
```

### Ready claims by composition

```promql
sum by (composition)(crossplane_claims_ready)
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

## XR queries

### All XRs grouped by kind

```promql
sum by (kind)(crossplane_xr_total)
```

### XR readiness ratio by composition

```promql
sum by (composition)(crossplane_xr_ready) / sum by (composition)(crossplane_xr_total)
```

### Not-ready XRs

```promql
sum(crossplane_xr_total) - sum(crossplane_xr_ready)
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
