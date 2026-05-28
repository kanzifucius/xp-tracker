# Metrics Reference

xp-tracker exposes eight Prometheus **gauge** metrics for Crossplane resources, plus five **self-monitoring** metrics for operational visibility.

## Claim metrics

### `crossplane_claims_total`

Total number of Crossplane claims, broken down by label tuple.

| Label | Description |
|---|---|
| `group` | API group from the GVR (e.g. `platform.example.org`) |
| `kind` | Resource kind (e.g. `PostgresqlInstance`) |
| `namespace` | Kubernetes namespace |
| `composition` | Crossplane Composition name (enriched from backing XR) |
| `creator` | Value of the `CREATOR_ANNOTATION_KEY` annotation |
| `team` | Value of the `TEAM_ANNOTATION_KEY` annotation |
| `claim_name` | Claim metadata name |
| `synced` | Crossplane `Synced` condition status (`true`/`false`) |
| `ready` | Crossplane `Ready` condition status (`true`/`false`) |

### `crossplane_claims_ready`

Number of Crossplane claims with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_claims_total`.

### `crossplane_claims_status_synced`

Per-claim Synced condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_claims_total`.

### `crossplane_claims_status_ready`

Per-claim Ready condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_claims_total`.

## XR metrics

### `crossplane_xr_total`

Total number of Crossplane composite resources (XRs), broken down by label tuple.

| Label | Description |
|---|---|
| `group` | API group from the GVR |
| `kind` | Resource kind (e.g. `XPostgreSQLInstance`) |
| `namespace` | Kubernetes namespace (usually empty for cluster-scoped XRs) |
| `composition` | Crossplane Composition name (from `COMPOSITION_LABEL_KEY` label) |
| `name` | XR metadata name |
| `synced` | Crossplane `Synced` condition status (`true`/`false`) |
| `ready` | Crossplane `Ready` condition status (`true`/`false`) |

### `crossplane_xr_ready`

Number of XRs with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_xr_total`.

### `crossplane_xr_status_synced`

Per-XR Synced condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_xr_total`.

### `crossplane_xr_status_ready`

Per-XR Ready condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_xr_total`.

## Example output

Output from `curl localhost:8080/metrics` with sample resources applied:

```prometheus
# HELP crossplane_claims_ready Number of Ready Crossplane claims by group, kind, namespace, composition, creator, claim_name, and status.
# TYPE crossplane_claims_ready gauge
crossplane_claims_ready{claim_name="gadget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",ready="false",synced="true",team="platform"} 0
crossplane_claims_ready{claim_name="widget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1

# HELP crossplane_claims_total Number of Crossplane claims by group, kind, namespace, composition, creator, claim_name, and status.
# TYPE crossplane_claims_total gauge
crossplane_claims_total{claim_name="gadget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",ready="false",synced="true",team="platform"} 1
crossplane_claims_total{claim_name="widget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1

# HELP crossplane_claims_status_synced Synced status for Crossplane claims (1=true, 0=false).
# TYPE crossplane_claims_status_synced gauge
crossplane_claims_status_synced{claim_name="gadget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",ready="false",synced="true",team="platform"} 1
crossplane_claims_status_synced{claim_name="widget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1

# HELP crossplane_claims_status_ready Ready status for Crossplane claims (1=true, 0=false).
# TYPE crossplane_claims_status_ready gauge
crossplane_claims_status_ready{claim_name="gadget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",ready="false",synced="true",team="platform"} 0
crossplane_claims_status_ready{claim_name="widget-a",composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1

# HELP crossplane_xr_ready Number of Ready Crossplane XRs by group, kind, namespace, composition, name, and status.
# TYPE crossplane_xr_ready gauge
crossplane_xr_ready{composition="",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 0
crossplane_xr_ready{composition="",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1

# HELP crossplane_xr_total Number of Crossplane composite resources (XRs) by group, kind, namespace, composition, name, and status.
# TYPE crossplane_xr_total gauge
crossplane_xr_total{composition="",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 1
crossplane_xr_total{composition="",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1

# HELP crossplane_xr_status_synced Synced status for Crossplane XRs (1=true, 0=false).
# TYPE crossplane_xr_status_synced gauge
crossplane_xr_status_synced{composition="",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 1
crossplane_xr_status_synced{composition="",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1

# HELP crossplane_xr_status_ready Ready status for Crossplane XRs (1=true, 0=false).
# TYPE crossplane_xr_status_ready gauge
crossplane_xr_status_ready{composition="",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 0
crossplane_xr_status_ready{composition="",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1
```

## Aggregation behaviour

Metrics are aggregated by their full label tuple. Because `claim_name` is included, claims are emitted as per-claim series.

This means cardinality is closely tied to the number of claims, with additional dimensions from status labels.

## Label notes

- **Empty labels**: if an annotation key is not configured or the annotation is not present on a resource, the label value is an empty string (`""`).
- **Composition enrichment**: claims inherit their `composition` label from the backing XR via the `spec.resourceRef.name` linkage. If the claim has no resource reference yet, the composition will be empty.
- **Namespace for XRs**: composite resources are typically cluster-scoped, so the `namespace` label is usually empty.

## Self-monitoring metrics

xp-tracker exposes operational metrics about itself under the `xp_tracker_` prefix. These are useful for alerting on poller failures, slow poll cycles, or S3 persistence issues.

### `xp_tracker_poll_duration_seconds`

Histogram tracking the duration of each poll cycle (all GVRs combined).

**Default buckets:** 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60 seconds.

### `xp_tracker_poll_errors_total`

Counter of per-GVR poll errors. Incremented each time a List call for a specific GVR fails.

### `xp_tracker_store_claims`

Gauge showing the current number of claims in the in-memory store, updated after each poll.

### `xp_tracker_store_xrs`

Gauge showing the current number of XRs in the in-memory store, updated after each poll.

### `xp_tracker_s3_persist_duration_seconds`

Histogram tracking the duration of S3 snapshot persistence. Only emitted when `STORE_BACKEND=s3`.

**Default buckets:** 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30 seconds.

### Example PromQL for self-monitoring

```promql
# Average poll duration over the last 5 minutes
rate(xp_tracker_poll_duration_seconds_sum[5m]) / rate(xp_tracker_poll_duration_seconds_count[5m])

# Poll error rate
rate(xp_tracker_poll_errors_total[5m])

# Current store size
xp_tracker_store_claims + xp_tracker_store_xrs

# 99th percentile S3 persist latency
histogram_quantile(0.99, rate(xp_tracker_s3_persist_duration_seconds_bucket[5m]))
```
