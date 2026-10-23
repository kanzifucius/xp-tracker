# xp-tracker

<p align="center">
  <a href="https://github.com/kanzifucius/xp-tracker/actions/workflows/ci.yml"><img src="https://github.com/kanzifucius/xp-tracker/actions/workflows/ci.yml/badge.svg" alt="Build"></a>
  <a href="https://github.com/kanzifucius/xp-tracker/releases/latest"><img src="https://img.shields.io/github/v/release/kanzifucius/xp-tracker?style=flat" alt="Release"></a>
  <a href="https://kanzifucius.github.io/xp-tracker/"><img src="https://img.shields.io/badge/docs-GitHub%20Pages-blue?style=flat&logo=github" alt="Documentation"></a>
  <img src="https://img.shields.io/badge/Go-1.26.9+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License">
  <img src="https://img.shields.io/badge/Crossplane-2.0+-7C3AED?style=flat" alt="Crossplane">
</p>

A minimal, read-only Prometheus exporter for Crossplane legacy claims, composite resources (XRs), XR-linked provider managed resources (MRs), and low-cardinality package inventory (Providers, Configurations, Functions). It supports both legacy Crossplane v1 claim workflows and Crossplane v2 namespaced resources, polls the Kubernetes API via the dynamic client, aggregates resource counts by meaningful labels, and exposes them on `/metrics`.

## Why xp-tracker?

### The gap in standard Crossplane metrics

Crossplane ships with controller-level Prometheus metrics out of the box -- reconcile duration, workqueue depth, API request latency, and similar operational signals. These are valuable for monitoring the health of the Crossplane controllers themselves, but they don't answer the questions platform teams actually ask:

- *How many claims exist per namespace?*
- *Who created them?*
- *Which team owns them?*
- *Which resources are stuck, paused, or not ready, and why?*
- *Is adoption growing over time?*

Standard Crossplane metrics have no concept of **creator**, **team**, **per-resource health**, or **per-namespace inventory counts**. That is the gap xp-tracker fills.

### What xp-tracker adds

- **Business-level dimensions** -- Claim metrics are broken down by `creator`, `team`, and `namespace`; XR and MR metrics carry identity and linkage labels (`name`, and for MRs `xr_name` plus typed `xr_kind` / `xr_namespace` / `xr_group` by default) so you can walk ownership XR → MR on modern fleets. Legacy claim linkage (`claim_name` / `claim_namespace`) remains for v1 claim paths and is empty on native v2 trees. Every series also carries per-resource status labels (`ready`, `reason`, `paused`, `deleting`). These are the dimensions that matter when you're running a platform, not just an operator.
- **Inventory and adoption tracking** -- Get real answers to "how many claims of each type exist?", "which namespaces are using the platform?", and "which resources are not ready, and for what reason?" -- all via standard PromQL queries and Grafana dashboards.
- **Chargeback and showback** -- The `creator` + `team` + `namespace` labels make it straightforward to build cost-allocation or usage-reporting dashboards per team or business unit.
- **Dynamic, zero-codegen** -- Works with any Crossplane CRD without code generation or recompilation. xp-tracker discovers claim and XR GVRs from XRDs and provider MR GVRs from Active ManagedResourceDefinitions at startup and on a configurable re-discovery interval.

### Standard Crossplane metrics vs xp-tracker

| Dimension | Crossplane built-in | xp-tracker |
|---|---|---|
| Reconcile latency / errors | Yes | -- |
| Workqueue depth | Yes | -- |
| Claim count by namespace | -- | Yes |
| Claim count by creator | -- | Yes |
| Claim count by team | -- | Yes |
| Readiness ratio by namespace / team | -- | Yes |
| XR count by kind / claim linkage | -- | Yes |
| MR count by provider / claim | -- | Yes |
| Provider / Function / Configuration health | -- | Yes |
| Stuck or not-ready resources by reason | -- | Yes |

> **In short:** Crossplane tells you how the *controller* is doing. xp-tracker tells you what *resources* exist, who owns them, and whether they're healthy -- the information platform teams need to run an internal developer platform.

### Pairs well with kindplane

[kindplane](https://github.com/kanzifucius/kindplane) is a companion CLI tool that bootstraps [Kind](https://kind.sigs.k8s.io/) clusters pre-configured with Crossplane, cloud providers, and Helm charts -- all with a single command. If you're evaluating xp-tracker or developing Crossplane compositions locally, kindplane is the fastest way to get a working environment:

```bash
# One command to get a local Crossplane cluster
kindplane up

# Deploy xp-tracker and start exploring metrics
kubectl apply -k deploy/base
curl -s localhost:8080/metrics | grep crossplane_
```

Together, the two tools cover the full local platform-engineering workflow: **kindplane** provisions the cluster and Crossplane stack, **xp-tracker** gives you visibility into the resources running on it.

## Architecture

```
                       +-----------------+
                       |  Kubernetes API |
                       +--------+--------+
                                |
                         List (dynamic client, paginated)
                                |
                       +--------v--------+
                       |     Poller      |  polls every N seconds
                       |  (pkg/kube)     |  for each discovered GVR
                       +--------+--------+  skips Persist on errors
                                |
                       ReplaceClaims / ReplaceXRs / ReplaceMRs / ReplacePackages
                       EnrichClaimCompositions / EnrichXRClaims / EnrichMRClaims
                                |
                       +--------v--------+
                       |   In-Memory     |
                       |   Store         |  thread-safe (sync.RWMutex)
                       |  (pkg/store)    |  implements store.Store interface
                       +--------+--------+
                                |
                        SnapshotClaims / SnapshotXRs / SnapshotMRs
                                |
                       +--------v--------+
                       | Claim/XR/MR     |
                       | Collectors      |  aggregate by label tuple
                       | (pkg/metrics)   |
                       +--------+--------+
                                |
                       +--------v-----------+
                       |  HTTP Server       |
                       |  GET /metrics      |  :8080 (configurable)
                       |  GET /healthz      |
                       |  GET /readyz       |
                       |  (pkg/server)      |
                       +--------+-----------+
                                |
                       +--------v--------+
                       |   Prometheus    |
                       +--------+--------+
                                |
                       +--------v--------+
                       |    Grafana      |
                       +-----------------+
```

The exporter is **strictly read-only** -- it only performs `get`, `list`, and `watch` operations against the Kubernetes API. It never creates, updates, or deletes any resources.

## Store Interface

The in-memory data layer is behind a `store.Store` interface, making the backing implementation swappable:

```go
type Store interface {
    ReplaceClaims(gvr string, items []ClaimInfo)
    ReplaceXRs(gvr string, items []XRInfo)
    ReplaceMRs(gvr string, items []MRInfo)
    EnrichClaimCompositions()
    EnrichXRClaims()
    EnrichMRClaims()
    SnapshotClaims() []ClaimInfo
    SnapshotXRs() []XRInfo
    SnapshotMRs() []MRInfo
    ClaimCount() int
    XRCount() int
    MRCount() int
}
```

The default implementation is `MemoryStore`, a thread-safe in-memory store using `sync.RWMutex`. To add a different backend (e.g., SQLite, DynamoDB, PostgreSQL), implement the `Store` interface and pass it to the poller and server constructors.

### Persistent Store

For workloads that restart frequently, the exporter supports persisting the in-memory snapshot to durable storage so metrics survive restarts without waiting for a full poll cycle.

The `PersistentStore` interface extends `Store`:

```go
type PersistentStore interface {
    Store
    Persist(ctx context.Context) error  // called after each poll cycle
    Restore(ctx context.Context) error  // called once at startup
}
```

All persistent backends use a **decorator pattern**: they wrap the core `MemoryStore`, delegate all 7 `Store` methods to it (so reads remain fast for Prometheus scraping), and add persistence on top.

#### S3 Backend

Set `STORE_BACKEND=s3` to persist snapshots to an S3 bucket (or any S3-compatible provider like MinIO, LocalStack, etc.).

```bash
export STORE_BACKEND=s3
export S3_BUCKET=my-xp-tracker-bucket
export S3_KEY_PREFIX=xp-tracker          # optional, default: xp-tracker
export S3_REGION=us-east-1               # optional, default: us-east-1
export S3_ENDPOINT=http://minio:9000     # optional, for S3-compatible providers
```

The snapshot is a single JSON file at `s3://<bucket>/<prefix>/snapshot.json`, overwritten after every poll cycle. On startup, the exporter attempts to restore from S3; if the key doesn't exist or S3 is unreachable, it starts with an empty store and logs a warning.

## Configuration

xp-tracker discovers XR GVRs from Crossplane v1 and v2 `CompositeResourceDefinition` objects, derives legacy claim GVRs where `claimNames` is present, and discovers provider MR GVRs from Active `ManagedResourceDefinition` objects at startup and on a configurable re-discovery interval.
Environment variables are still used for namespace filtering, annotation/label keys, polling and discovery cadence, server address, and optional additive GVR overrides.

| Variable | Required | Default | Description |
|---|---|---|---|
| `CLAIM_GVRS` | no (deprecated) | `""` | Optional additional claim GVRs (`group/version/resource`), merged with XRD discovery |
| `XR_GVRS` | no (deprecated) | `""` | Optional additional XR GVRs (`group/version/resource`), merged with XRD discovery |
| `KUBE_NAMESPACE_SCOPE` | no | `""` (all) | Comma-separated namespace filter |
| `CREATOR_ANNOTATION_KEY` | no | `""` | Annotation key for claim creator |
| `TEAM_ANNOTATION_KEY` | no | `""` | Annotation key for claim team |
| `COMPOSITION_LABEL_KEY` | no | `crossplane.io/composition-name` | Label key on XRs for composition |
| `COMPOSITE_LABEL_KEY` | no | `crossplane.io/composite` | Label key on MRs linking them to a composite |
| `MR_XR_PARENT_LABELS` | no | `true` | Include `xr_kind` / `xr_namespace` / `xr_group` on `crossplane_mr_*` (set `false` to omit) |
| `MR_GVRS` | no | `""` | Additional MR GVRs merged with MRD discovery |
| `POLL_INTERVAL_SECONDS` | no | `30` | Seconds between polling cycles |
| `DISCOVERY_INTERVAL_SECONDS` | no | `300` | Seconds between XRD/MRD re-discovery cycles; `0` disables periodic re-discovery |
| `METRICS_ADDR` | no | `:8080` | Listen address for HTTP metrics |
| `LOG_LEVEL` | no | `info` | Structured log level: `debug`, `info`, `warn`, or `error` |
| `STORE_BACKEND` | no | `memory` | Persistent store backend: `memory` or `s3` |
| `S3_BUCKET` | when `s3` | `""` | S3 bucket name |
| `S3_KEY_PREFIX` | no | `xp-tracker` | S3 key prefix for snapshot file |
| `S3_REGION` | no | `us-east-1` | AWS region for S3 client |
| `S3_ENDPOINT` | no | `""` | Custom S3 endpoint (MinIO, LocalStack) |

### XRD discovery

At startup (and every `DISCOVERY_INTERVAL_SECONDS` by default), xp-tracker lists XRDs from `apiextensions.crossplane.io/v1` and `apiextensions.crossplane.io/v2` and derives:

1. XR GVR from `spec.group` + selected `spec.versions[].name` + `spec.names.plural`
2. Claim GVR from `spec.group` + selected `spec.versions[].name` + `spec.claimNames.plural` (when present on legacy XRDs)

Version selection is deterministic: first `referenceable` version, otherwise first `served` version.

XRDs whose spec cannot yield a GVR are skipped (logged and counted in `xp_tracker_xrd_discovery_skipped_total`) instead of failing discovery. Listing XRDs still fails fatally. If no XR GVRs can be discovered from XRDs, startup fails with a clear error. A failed periodic refresh keeps the previous inventory. Claims are optional because native Crossplane v2 XRDs do not support them. Deprecated `CLAIM_GVRS` / `XR_GVRS` values are merged into the discovered sets (deduplicated) and do not replace discovery. Set `DISCOVERY_INTERVAL_SECONDS=0` to disable periodic re-discovery.

### Provider MR discovery

xp-tracker lists `ManagedResourceDefinition` objects (`apiextensions.crossplane.io/v1alpha1`) and derives MR GVRs from Active MRDs (`spec.state: Active`) at startup and on each re-discovery cycle. Provider attribution uses the `pkg.crossplane.io/package` label or a `Provider` owner reference. Additional GVRs from `MR_GVRS` are merged in. Active MRDs whose spec cannot yield a GVR are skipped (logged and counted in `xp_tracker_mrd_discovery_skipped_total`) instead of failing discovery. Only MRs with the composite label are polled; typed XR parent fields and legacy claim linkage are enriched from the backing XR.

### Additional claim/XR GVRs (deprecated)

`CLAIM_GVRS` and `XR_GVRS` are deprecated additive overrides in `group/version/resource` format. They are merged with XRD discovery (same pattern as `MR_GVRS`). For example:

```
CLAIM_GVRS=platform.example.org/v1alpha1/postgresqlinstances,platform.example.org/v1alpha1/kafkatopics
XR_GVRS=platform.example.org/v1alpha1/xpostgresqlinstances,platform.example.org/v1alpha1/xkafkatopics
```

## Metrics Reference

All metrics are Prometheus **gauges** that are recomputed on each scrape from the in-memory store.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `crossplane_claims_total` | Gauge | `group`, `kind`, `version`, `namespace`, `creator`, `team`, `claim_name`, `synced`, `ready`, `reason`, `paused`, `deleting` | Total number of claims |
| `crossplane_claims_ready` | Gauge | same as `crossplane_claims_total` | Number of claims with Ready=True |
| `crossplane_claims_status_synced` | Gauge | same as `crossplane_claims_total` | Per-claim Synced status (1=true, 0=false) |
| `crossplane_claims_status_ready` | Gauge | same as `crossplane_claims_total` | Per-claim Ready status (1=true, 0=false) |
| `crossplane_claims_created_timestamp_seconds` | Gauge | same as `crossplane_claims_total` | Unix creation timestamp |
| `crossplane_claims_deletion_timestamp_seconds` | Gauge | same as `crossplane_claims_total` | Unix deletion timestamp (while deleting) |
| `crossplane_xr_total` | Gauge | `group`, `kind`, `version`, `namespace`, `name`, `claim_name`, `claim_namespace`, `synced`, `ready`, `reason`, `paused`, `deleting` | Total number of XRs |
| `crossplane_xr_ready` | Gauge | same as `crossplane_xr_total` | Number of XRs with Ready=True |
| `crossplane_xr_status_synced` | Gauge | same as `crossplane_xr_total` | Per-XR Synced status (1=true, 0=false) |
| `crossplane_xr_status_ready` | Gauge | same as `crossplane_xr_total` | Per-XR Ready status (1=true, 0=false) |
| `crossplane_xr_created_timestamp_seconds` | Gauge | same as `crossplane_xr_total` | Unix creation timestamp |
| `crossplane_xr_deletion_timestamp_seconds` | Gauge | same as `crossplane_xr_total` | Unix deletion timestamp (while deleting) |
| `crossplane_mr_total` | Gauge | `group`, `kind`, `version`, `namespace`, `name`, `xr_name`, `xr_kind`, `xr_namespace`, `xr_group`, `claim_name`, `claim_namespace`, `provider`, `provider_config`, `external_name`, `management_policies`, `synced`, `ready`, `reason`, `paused`, `deleting` | Total number of XR-linked MRs (`xr_kind`/`xr_namespace`/`xr_group` omitted when `MR_XR_PARENT_LABELS=false`) |
| `crossplane_mr_ready` | Gauge | same as `crossplane_mr_total` | Number of MRs with Ready=True |
| `crossplane_mr_status_synced` | Gauge | same as `crossplane_mr_total` | Per-MR Synced status (1=true, 0=false) |
| `crossplane_mr_status_ready` | Gauge | same as `crossplane_mr_total` | Per-MR Ready status (1=true, 0=false) |
| `crossplane_mr_created_timestamp_seconds` | Gauge | same as `crossplane_mr_total` | Unix creation timestamp |
| `crossplane_mr_deletion_timestamp_seconds` | Gauge | same as `crossplane_mr_total` | Unix deletion timestamp (while deleting) |

### Label details

- **group** -- API group from the GVR (e.g. `platform.example.org`)
- **kind** -- Resource kind (e.g. `PostgresqlInstance`)
- **version** -- API version from the GVR (e.g. `v1alpha1`)
- **namespace** -- Kubernetes namespace (empty for cluster-scoped XRs)
- **creator** -- Value of the annotation specified by `CREATOR_ANNOTATION_KEY` (claims only)
- **team** -- Value of the annotation specified by `TEAM_ANNOTATION_KEY` (claims only)
- **claim_name** -- Claim metadata name (claims); XR/MR claim linkage from labels or enrichment
- **claim_namespace** -- Claim namespace linked to an XR or MR
- **name** -- XR or MR metadata name
- **xr_name** -- Composite name from the composite label (MRs only); primary XR → MR ownership edge
- **xr_kind** / **xr_namespace** / **xr_group** -- Typed backing XR identity on MR series (default on; empty until the XR is in the store)
- **provider** / **provider_config** / **external_name** / **management_policies** -- MR provider attribution and cloud identity
- **synced** / **ready** -- Crossplane condition statuses as labels (`true`/`false`)
- **reason** -- Ready condition reason (e.g. `Available`, `Creating`)
- **paused** -- Whether `crossplane.io/paused` is set (`true`/`false`)
- **deleting** -- Whether `metadata.deletionTimestamp` is set (`true`/`false`)

### Example output

Output from `curl localhost:8080/metrics` with the sample resources applied (`make samples-apply`):

```
# HELP crossplane_claims_ready Number of Ready Crossplane claims by group, kind, namespace, creator, claim_name, and status.
# TYPE crossplane_claims_ready gauge
crossplane_claims_ready{claim_name="widget-a",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1
crossplane_claims_ready{claim_name="gadget-a",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",ready="false",synced="true",team="platform"} 0
# HELP crossplane_claims_total Number of Crossplane claims by group, kind, namespace, creator, claim_name, and status.
# TYPE crossplane_claims_total gauge
crossplane_claims_total{claim_name="widget-a",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1
crossplane_claims_total{claim_name="gadget-a",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",ready="false",synced="true",team="platform"} 1
# HELP crossplane_claims_status_synced Synced status for Crossplane claims (1=true, 0=false).
# TYPE crossplane_claims_status_synced gauge
crossplane_claims_status_synced{claim_name="widget-a",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1
# HELP crossplane_claims_status_ready Ready status for Crossplane claims (1=true, 0=false).
# TYPE crossplane_claims_status_ready gauge
crossplane_claims_status_ready{claim_name="widget-a",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",ready="true",synced="true",team="platform"} 1
# HELP crossplane_xr_ready Number of Ready Crossplane XRs by group, kind, namespace, name, and status.
# TYPE crossplane_xr_ready gauge
crossplane_xr_ready{claim_name="widget-a",claim_namespace="team-alpha",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 0
crossplane_xr_ready{claim_name="widget-b",claim_namespace="team-beta",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1
# HELP crossplane_xr_total Number of Crossplane composite resources (XRs) by group, kind, namespace, name, and status.
# TYPE crossplane_xr_total gauge
crossplane_xr_total{claim_name="widget-a",claim_namespace="team-alpha",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 1
crossplane_xr_total{claim_name="widget-b",claim_namespace="team-beta",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1
# HELP crossplane_xr_status_synced Synced status for Crossplane XRs (1=true, 0=false).
# TYPE crossplane_xr_status_synced gauge
crossplane_xr_status_synced{claim_name="widget-a",claim_namespace="team-alpha",group="samples.xptracker.dev",kind="XGadget",name="xgadget-a",namespace="",ready="false",synced="true"} 1
# HELP crossplane_xr_status_ready Ready status for Crossplane XRs (1=true, 0=false).
# TYPE crossplane_xr_status_ready gauge
crossplane_xr_status_ready{claim_name="widget-b",claim_namespace="team-beta",group="samples.xptracker.dev",kind="XWidget",name="xwidget-a",namespace="",ready="true",synced="true"} 1
```

## Health Endpoints

The exporter exposes two health endpoints for Kubernetes probes:

| Endpoint | Purpose | Behaviour |
|---|---|---|
| `GET /healthz` | Liveness probe | Always returns `200 OK` with body `ok` |
| `GET /readyz` | Readiness probe | Returns `503 Service Unavailable` until the first poll cycle completes, then `200 OK` with body `ok` |

The base Deployment manifests configure Kubernetes liveness and readiness probes against these endpoints. The readiness probe prevents traffic from reaching the exporter until it has populated the in-memory store with at least one polling cycle.

## Deployment

The exporter ships with [Kustomize](https://kustomize.io/) manifests and a thin [Helm](https://helm.sh/) chart that wraps the same `deploy/base` resources.

### Prerequisites

- A Kubernetes cluster with Crossplane installed
- Crossplane CRDs (XRDs) and Compositions deployed
- `kubectl` with access to the cluster
- Helm 3.x (only if using the chart)

### Quick start (base)

The base deploys to the `crossplane-system` namespace:

```bash
# Review what will be applied
kubectl kustomize deploy/base

# Apply
kubectl apply -k deploy/base
```

### Helm chart

The chart at `charts/xp-tracker/` mirrors ServiceAccount, RBAC, ConfigMap, Deployment, and Service from `deploy/base`. Image tags are pinned (defaults to `Chart.appVersion`; empty/`latest` tags are rejected):

```bash
# Review rendered manifests
helm template xp-tracker charts/xp-tracker --namespace crossplane-system

# Install
helm upgrade --install xp-tracker charts/xp-tracker \
  --namespace crossplane-system \
  --create-namespace \
  --set image.tag=v0.4.0
```

See [docs/deployment/helm.md](docs/deployment/helm.md) for values, RBAC scoping, and configuration.

### Using the example overlay

The example overlay shows how to customise attribution labels, add a ServiceMonitor, and pin the image tag:

```bash
# Review
kubectl kustomize deploy/overlays/example

# Apply
kubectl apply -k deploy/overlays/example
```

### Creating your own overlay

```
deploy/overlays/my-env/
  kustomization.yaml
```

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

patches:
  - target:
      kind: ConfigMap
      name: crossplane-metrics-exporter
    patch: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: crossplane-metrics-exporter
      data:
        # Optional additive overrides (deprecated; merged with XRD discovery):
        # CLAIM_GVRS: "myorg.io/v1alpha1/databases,myorg.io/v1alpha1/caches"
        # XR_GVRS: "myorg.io/v1alpha1/xdatabases,myorg.io/v1alpha1/xcaches"
        CREATOR_ANNOTATION_KEY: "myorg.io/created-by"
        TEAM_ANNOTATION_KEY: "myorg.io/team"

images:
  - name: ghcr.io/kanzifucius/xp-tracker
    newTag: v0.2.0
```

## RBAC

The exporter needs **read-only** access to the Crossplane claim and XR resources it polls. The base manifests include a `ClusterRole` with broad read access (`apiGroups: ["*"]`, `resources: ["*"]`, `verbs: ["get", "list", "watch"]`).

> **Warning:** The base ClusterRole is intentionally broad for quick-start convenience. For production, scope it down to only the API groups and resources you actually poll.

For production, restrict the ClusterRole to only the specific API groups and resources you need:

```yaml
rules:
  - apiGroups: ["platform.example.org"]
    resources: ["postgresqlinstances", "xpostgresqlinstances", "kafkatopics", "xkafkatopics"]
    verbs: ["get", "list", "watch"]
```

The example overlay at `deploy/overlays/example/` includes a scoped ClusterRole patch that demonstrates this pattern. See [RBAC docs](docs/deployment/rbac.md) for details.

The ClusterRoleBinding binds this role to the `crossplane-metrics-exporter` ServiceAccount in the `crossplane-system` namespace.

## Prometheus Scraping

### With Prometheus Operator (ServiceMonitor)

The example overlay includes a `ServiceMonitor`. If you use the Prometheus Operator, apply the example overlay or add the ServiceMonitor to your own overlay.

### Without Prometheus Operator

Add a scrape config to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: crossplane-metrics-exporter
    kubernetes_sd_configs:
      - role: endpoints
        namespaces:
          names:
            - crossplane-system
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_name]
        regex: crossplane-metrics-exporter
        action: keep
      - source_labels: [__meta_kubernetes_endpoint_port_name]
        regex: metrics
        action: keep
```

Or, if you use annotations-based discovery, the Service already has the standard labels for scraping.

## Grafana Queries

Example PromQL queries for dashboards:

```promql
# Total claims by namespace
sum by (namespace)(crossplane_claims_total)

# Ready claims by team
sum by (team)(crossplane_claims_ready)

# Claims by creator
sum by (creator)(crossplane_claims_total)

# Claim readiness ratio by namespace
sum by (namespace)(crossplane_claims_ready) / sum by (namespace)(crossplane_claims_total)

# XR readiness ratio by kind
sum by (kind)(crossplane_xr_ready) / sum by (kind)(crossplane_xr_total)

# Not-ready claims (total minus ready)
sum by (namespace)(crossplane_claims_total) - sum by (namespace)(crossplane_claims_ready)

# All XRs grouped by kind
sum by (kind)(crossplane_xr_total)
```

## Potential Prometheus Alerts

Alongside dashboards, you should define alert rules for exporter health and resource readiness. A practical baseline is:

```promql
# Polling errors detected
increase(xp_tracker_poll_errors_total[10m]) > 0

# Poll duration p99 too high (tune threshold for your poll interval)
histogram_quantile(0.99, sum by (le) (rate(xp_tracker_poll_duration_seconds_bucket[15m]))) > 20

# No fully successful poll recently (tune 90s ≈ 3× POLL_INTERVAL_SECONDS)
(time() - xp_tracker_last_successful_poll_timestamp_seconds) > 90

# Claim readiness ratio degraded
(sum(crossplane_claims_ready) / sum(crossplane_claims_total)) < 0.8

# XR readiness ratio degraded
(sum(crossplane_xr_ready) / sum(crossplane_xr_total)) < 0.8
```

For a production-ready `PrometheusRule` manifest with severities, `for` windows, and tuning guidance, see [`docs/metrics/prometheus-alerts.md`](docs/metrics/prometheus-alerts.md).

## Docker

Build and push the container image locally:

```bash
# Build (defaults to ghcr.io/kanzifucius/xp-tracker:latest)
make docker-build

# Override image/tag
make docker-build IMAGE=myregistry.io/xp-tracker TAG=v0.1.0

# Push
make docker-push IMAGE=myregistry.io/xp-tracker TAG=v0.1.0
```

The Dockerfile uses a multi-stage build with BuildKit multi-arch support (`linux/amd64` + `linux/arm64`). Build stage: `golang:1.26-alpine`. Runtime stage: `gcr.io/distroless/static-debian12:nonroot`. The final image is ~10 MB and runs as a non-root user.

### Version and build info

The binary embeds `version`, `commit`, and `date` via `-ldflags` at build time. These are logged at startup and can be used to verify which build is running:

```json
{"level":"INFO","msg":"xp-tracker starting","version":"v0.2.0","commit":"abc1234","date":"2026-02-15T10:00:00Z"}
```

When building locally, `make build` automatically injects the current git tag, commit SHA, and timestamp. The CI and Release workflows pass these as Docker build args.

## CI/CD

The project uses GitHub Actions for continuous integration and releases. Container images are published to `ghcr.io/kanzifucius/xp-tracker`.

### CI (`.github/workflows/ci.yml`)

Runs on every push to `main` and on pull requests:

1. **Test & Lint** -- `go mod verify`, `go vet`, `golangci-lint`, `go test -race`
2. **Build & Push** -- Builds a multi-arch container image (`linux/amd64` + `linux/arm64`)
   - On PRs: build only (validates the Dockerfile)
   - On `main` push: builds and pushes with tags `latest` and `sha-<short>`, then
     generates an SPDX SBOM and attaches Sigstore-backed provenance + SBOM
     attestations to GHCR (no long-lived signing secrets)

### Release (`.github/workflows/release.yml`)

Triggered by pushing a semver tag (`v*`):

1. **Test & Lint** -- Same gate as CI
2. **Build, Push & Release** -- Builds multi-arch image with embedded version info, pushes to GHCR, creates a GitHub Release, generates an SPDX SBOM (release asset), and attaches provenance + SBOM attestations

**Image tags produced by a release (e.g. `v1.2.3`):**

| Tag | Example |
|---|---|
| `{{version}}` | `1.2.3` |
| `{{major}}.{{minor}}` | `1.2` |
| `{{major}}` (if >= 1) | `1` |

Pre-release tags (containing `-rc`, `-alpha`, `-beta`) are marked as pre-releases on GitHub.

### Verifying image attestations

Consumers can verify SLSA provenance and the SPDX SBOM with the GitHub CLI
(`gh attestation verify`). See [SECURITY.md](SECURITY.md#container-image-provenance-and-sbom)
for the exact commands.

### Creating a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

This triggers the release workflow, which:
- Runs tests and lint
- Builds `linux/amd64` + `linux/arm64` images
- Pushes to `ghcr.io/kanzifucius/xp-tracker:0.1.0` (and `:0.1`)
- Creates a GitHub Release with auto-generated release notes
- Attaches an SPDX SBOM and GitHub Artifact Attestations (provenance + SBOM)

## Local Development

### Prerequisites

- Go 1.26.9+
- A Kubernetes cluster (kind, k3d, minikube, etc.) with Crossplane installed
- `golangci-lint` for linting
- [mise](https://mise.jdx.dev/) (recommended) -- run `mise install` to set up pinned tool versions

### Build and run locally

```bash
# Build
make build

# Run against a local cluster (uses KUBECONFIG or ~/.kube/config)
# Ensure your XRDs are already installed in the cluster.
export POLL_INTERVAL_SECONDS=10
./bin/xp-tracker

# In another terminal
curl localhost:8080/metrics
```

### Testing

```bash
# Run all tests with race detector
make test

# Lint
make lint

# Vet
go vet ./...
```

### Using kindplane (recommended)

[kindplane](https://github.com/kanzifucius/kindplane) bootstraps Kind clusters pre-configured with Crossplane and providers -- ideal for local development. This repo includes a `kindplane.yaml` that sets up Crossplane 2.0 with `provider-nop` for dummy resources and sample XRDs/Claims under `hack/samples/`.

```bash
# Install kindplane
curl -fsSL https://raw.githubusercontent.com/kanzifucius/kindplane/main/install.sh | bash

# Create the cluster (uses kindplane.yaml in repo root)
kindplane up

# Apply sample XRDs, Compositions, and Claims (8 claims across 3 namespaces)
make samples-apply

# Run the exporter against the sample resources (discovered automatically from XRDs)
export CREATOR_ANNOTATION_KEY="xptracker.dev/created-by"
export TEAM_ANNOTATION_KEY="xptracker.dev/team"
make run

# In another terminal -- check metrics
curl -s localhost:8080/metrics | grep crossplane_

# Clean up
make samples-delete
kindplane down
```

### Using kind (manual)

If you prefer to set up the cluster manually without kindplane:

```bash
# Create a cluster
kind create cluster --name xp-dev

# Install Crossplane
helm repo add crossplane-stable https://charts.crossplane.io/stable
helm install crossplane crossplane-stable/crossplane --namespace crossplane-system --create-namespace

# Install your XRDs and Compositions, then create some claims

# Run the exporter locally
make run
```

## Project Structure

```
.
├── cmd/
│   └── exporter/
│       └── main.go                  # Entrypoint -- config, client, poller, server, signal handling
├── pkg/
│   ├── config/                      # Environment variable parsing and validation
│   ├── kube/
│   │   ├── client.go                # Dynamic client factory (in-cluster + kubeconfig fallback)
│   │   ├── convert.go               # Unstructured -> ClaimInfo/XRInfo/MRInfo conversion
│   │   ├── discovery.go             # GVR discovery from XRDs and ManagedResourceDefinitions
│   │   └── poller.go                # Ticker-based polling loop with claim linkage enrichment
│   ├── metrics/
│   │   ├── claim_collector.go       # ClaimCollector (Describe/Collect)
│   │   ├── xr_collector.go          # XRCollector (Describe/Collect)
│   │   ├── mr_collector.go          # MRCollector (Describe/Collect)
│   │   └── self.go                  # Self-monitoring metrics (xp_tracker_* prefix)
│   ├── server/
│   │   └── server.go                # HTTP server with custom Prometheus registry
│   └── store/
│       ├── store.go                 # Store interface + MemoryStore implementation
│       └── s3store.go               # S3Store persistent backend (decorator over MemoryStore)
├── charts/
│   └── xp-tracker/                  # Thin Helm chart wrapping deploy/base (pinned image tags)
├── deploy/
│   ├── base/                        # Kustomize base (SA, RBAC, ConfigMap, Deployment, Service)
│   └── overlays/
│       └── example/                 # Example overlay with ServiceMonitor
├── hack/
│   └── samples/                     # Sample XRDs, Compositions, and Claims for local dev
│       ├── namespaces.yaml
│       ├── functions.yaml
│       ├── xrds.yaml
│       ├── compositions.yaml
│       └── claims.yaml
├── .github/
│   └── workflows/
│       ├── ci.yml                   # CI pipeline (test, lint, build, push on main)
│       └── release.yml              # Release pipeline (tag-triggered, multi-arch, GitHub Release)
├── Dockerfile                       # Multi-stage multi-arch build (distroless runtime)
├── Makefile                         # build, test, lint, docker-build, deploy, samples targets
├── kindplane.yaml                   # kindplane config for local dev (Crossplane + provider-nop)
├── .mise.toml                       # Tool version pinning (Go, golangci-lint, kubectl)
├── go.mod
└── go.sum
```

## Troubleshooting

### No metrics are returned

- Check the exporter logs for polling errors: `kubectl logs -n crossplane-system deploy/crossplane-metrics-exporter`
- Verify your XRDs are installed and healthy: `kubectl get compositeresourcedefinitions.apiextensions.crossplane.io`
- If you use deprecated additive `CLAIM_GVRS` / `XR_GVRS`, confirm they match installed CRDs; they merge with XRD discovery rather than replacing it.
- If using `KUBE_NAMESPACE_SCOPE`, ensure the namespaces exist and contain claims

### Metrics show 0 for ready counts

- Crossplane claims and XRs report readiness via `status.conditions` with `type: Ready` and `status: "True"`. If your resources use a different condition, the exporter won't detect them as ready.

### RBAC errors in logs

- The ClusterRole needs `get` and `list` permissions for discovered claim/XR resources and `compositeresourcedefinitions.apiextensions.crossplane.io`.

### Stale metrics after deleting resources

- The exporter does a full replace on each polling cycle. Deleted resources will disappear from metrics after the next poll (default: 30 seconds).

### `claim_name` / `claim_namespace` labels are empty

- On legacy XRs, the exporter reads `crossplane.io/claim-name` and `crossplane.io/claim-namespace` labels. If they are missing, it backfills them from the claim whose `spec.resourceRef.name` matches the XR name; the labels stay empty until the claim has been bound to its XR.
- On MRs, the exporter reads the same claim labels, then falls back to the XR named by the `COMPOSITE_LABEL_KEY` label (default: `crossplane.io/composite`). MRs without that label are not tracked at all.
- Native Crossplane v2 XRs have no claims, so these labels are always empty for them. See [Environment Variables](docs/configuration/environment-variables.md) for the full enrichment rules.

### Self-monitoring metrics

xp-tracker also exposes metrics about its own operation under the `xp_tracker_` prefix. These are useful for alerting on poller failures or slow S3 persistence.

| Metric | Type | Description |
|---|---|---|
| `xp_tracker_poll_duration_seconds` | Histogram | Duration of each poll cycle |
| `xp_tracker_poll_errors_total` | Counter | Total number of per-GVR poll errors |
| `xp_tracker_last_successful_poll_timestamp_seconds` | Gauge | Unix timestamp of the last poll cycle with no GVR errors |
| `xp_tracker_store_claims` | Gauge | Current number of claims in the store |
| `xp_tracker_store_xrs` | Gauge | Current number of XRs in the store |
| `xp_tracker_s3_persist_duration_seconds` | Histogram | Duration of each S3 persist operation |
| `xp_tracker_mrd_discovery_skipped_total` | Counter | Active MRDs skipped during discovery because their spec was unusable (`reason` label) |
| `xp_tracker_xrd_discovery_skipped_total` | Counter | XRDs skipped during discovery because their spec was unusable (`reason` label) |
| `xp_tracker_discovery_runs_total` | Counter | XRD/MRD discovery runs (`result`=`success` or `error`) |
| `xp_tracker_discovery_duration_seconds` | Histogram | Duration of each XRD/MRD discovery run |

### Single replica requirement

- Running more than one replica will result in double-counted metrics since each replica independently polls and serves metrics. Keep `replicas: 1`.
