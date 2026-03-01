# xp-tracker

<p align="center">
  <a href="https://github.com/kanzifucius/xp-tracker/actions/workflows/ci.yml"><img src="https://github.com/kanzifucius/xp-tracker/actions/workflows/ci.yml/badge.svg" alt="Build"></a>
  <a href="https://github.com/kanzifucius/xp-tracker/releases/latest"><img src="https://img.shields.io/github/v/release/kanzifucius/xp-tracker?style=flat" alt="Release"></a>
  <a href="https://kanzifucius.github.io/xp-tracker/"><img src="https://img.shields.io/badge/docs-GitHub%20Pages-blue?style=flat&logo=github" alt="Documentation"></a>
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License">
  <img src="https://img.shields.io/badge/Crossplane-2.0+-7C3AED?style=flat" alt="Crossplane">
</p>

A minimal, read-only Prometheus exporter for Crossplane claims and composite resources (XRs). It polls the Kubernetes API via the dynamic client, aggregates resource counts by meaningful labels, and exposes them on `/metrics`.

## Why xp-tracker?

### The gap in standard Crossplane metrics

Crossplane ships with controller-level Prometheus metrics out of the box -- reconcile duration, workqueue depth, API request latency, and similar operational signals. These are valuable for monitoring the health of the Crossplane controllers themselves, but they don't answer the questions platform teams actually ask:

- *How many claims exist per namespace?*
- *Who created them?*
- *Which team owns them?*
- *Which compositions are most popular?*
- *Is adoption growing over time?*

Standard Crossplane metrics have no concept of **creator**, **team**, **composition breakdown**, or **per-namespace inventory counts**. That is the gap xp-tracker fills.

### What xp-tracker adds

- **Business-level dimensions** -- Every metric is broken down by `creator`, `team`, `namespace`, and `composition`. These are the dimensions that matter when you're running a platform, not just an operator.
- **Inventory and adoption tracking** -- Get real answers to "how many claims of each type exist?", "which namespaces are using the platform?", and "which compositions are most adopted?" -- all via standard PromQL queries and Grafana dashboards.
- **Chargeback and showback** -- The `creator` + `team` + `namespace` labels make it straightforward to build cost-allocation or usage-reporting dashboards per team or business unit.
- **Dynamic, zero-codegen** -- Works with any Crossplane CRD without code generation or recompilation. Just configure your GVRs as environment variables and deploy.
- **JSON bookkeeping endpoint** -- Beyond Prometheus, the `/bookkeeping` endpoint returns a full snapshot of all tracked resources as JSON. Useful for CLI tooling, external integrations, audit trails, or any consumer that doesn't want to go through PromQL.

### Standard Crossplane metrics vs xp-tracker

| Dimension | Crossplane built-in | xp-tracker |
|---|---|---|
| Reconcile latency / errors | Yes | -- |
| Workqueue depth | Yes | -- |
| Claim count by namespace | -- | Yes |
| Claim count by creator | -- | Yes |
| Claim count by team | -- | Yes |
| Readiness ratio by composition | -- | Yes |
| XR count by kind / composition | -- | Yes |
| JSON resource inventory | -- | Yes |

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
                    +-----------+-----------+
                    |                       |
             List (dynamic client)   Watch (typed client)
                    |                       |
           +--------v--------+    +--------v-----------+
           |     Poller      |    |  ConfigMap Watcher  |
           |  (pkg/kube)     |    |  (pkg/kube)         |
           +--------+--------+    +--------+------------+
                    |                       |
                    |    per-namespace GVR configs
                    |    (label: xp-tracker.kanzi.io/config)
                    |                       |
                    +-----------+-----------+
                                |
                    polls central + namespace GVRs
                    (namespace claims scoped, XRs cluster-wide)
                                |
                       ReplaceClaims / ReplaceXRs
                       EnrichClaimCompositions
                                |
                       +--------v--------+
                       |   In-Memory     |
                       |   Store         |  thread-safe (sync.RWMutex)
                       |  (pkg/store)    |  implements store.Store interface
                       +--------+--------+
                                |
                        SnapshotClaims / SnapshotXRs
                                |
                       +--------v--------+
                       | Claim/XR        |
                       | Collectors      |  aggregate by label tuple
                       | (pkg/metrics)   |
                       +--------+--------+
                                |
                       +--------v-----------+
                       |  HTTP Server       |
                       |  GET /metrics      |  :8080 (configurable)
                       |  GET /bookkeeping  |
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
    EnrichClaimCompositions()
    SnapshotClaims() []ClaimInfo
    SnapshotXRs() []XRInfo
    ClaimCount() int
    XRCount() int
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

All configuration is via environment variables.

| Variable | Required | Default | Description |
|---|---|---|---|
| `CLAIM_GVRS` | no | -- | Comma-separated claim GVRs (`group/version/resource`) |
| `XR_GVRS` | no | -- | Comma-separated XR GVRs |
| `KUBE_NAMESPACE_SCOPE` | no | `""` (all) | Comma-separated namespace filter |
| `CREATOR_ANNOTATION_KEY` | no | `""` | Annotation key for claim creator |
| `TEAM_ANNOTATION_KEY` | no | `""` | Annotation key for claim team |
| `COMPOSITION_LABEL_KEY` | no | `crossplane.io/composition-name` | Label key on XRs for composition |
| `POLL_INTERVAL_SECONDS` | no | `30` | Seconds between polling cycles |
| `METRICS_ADDR` | no | `:8080` | Listen address for HTTP metrics |
| `STORE_BACKEND` | no | `memory` | Persistent store backend: `memory` or `s3` |
| `S3_BUCKET` | when `s3` | `""` | S3 bucket name |
| `S3_KEY_PREFIX` | no | `xp-tracker` | S3 key prefix for snapshot file |
| `S3_REGION` | no | `us-east-1` | AWS region for S3 client |
| `S3_ENDPOINT` | no | `""` | Custom S3 endpoint (MinIO, LocalStack) |

### GVR format

Each GVR must be in `group/version/resource` format. For example:

```
CLAIM_GVRS=platform.example.org/v1alpha1/postgresqlinstances,platform.example.org/v1alpha1/kafkatopics
XR_GVRS=platform.example.org/v1alpha1/xpostgresqlinstances,platform.example.org/v1alpha1/xkafkatopics
```

### Per-Namespace ConfigMaps

In addition to central environment variables, teams can opt into GVR monitoring by creating labeled ConfigMaps in their own namespaces. This enables a self-service model where tenants declare which Crossplane resources they want tracked -- without modifying the platform team's central configuration.

The exporter discovers ConfigMaps with the label `xp-tracker.kanzi.io/config: "gvrs"` using a Kubernetes informer. Changes are picked up automatically (hot reload) -- no restart required.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-team-xp-tracker       # any name
  namespace: team-a
  labels:
    xp-tracker.kanzi.io/config: "gvrs"
data:
  CLAIM_GVRS: "platform.example.org/v1alpha1/postgresqlinstances"
  XR_GVRS: "platform.example.org/v1alpha1/xpostgresqlinstances"
  CREATOR_ANNOTATION_KEY: "platform.example.org/created-by"   # optional override
  TEAM_ANNOTATION_KEY: "platform.example.org/team"             # optional override
```

**Scoping rules:**

- **Claims** from a namespace ConfigMap are polled **only within that namespace**.
- **XRs** are polled **cluster-wide** (Crossplane XRs are cluster-scoped).
- If a GVR appears in both the central config and a namespace ConfigMap, the **central config takes priority** and the namespace entry is silently skipped.
- `CLAIM_GVRS` and `XR_GVRS` in the central config are now **optional**, supporting pure namespace-driven deployments.

A sample ConfigMap is included at `deploy/base/sample-namespace-configmap.yaml`. See the [full documentation](docs/configuration/namespace-configmaps.md) for details on supported fields, deduplication, annotation inheritance, and troubleshooting.

## Metrics Reference

All metrics are Prometheus **gauges** that are recomputed on each scrape from the in-memory store.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `crossplane_claims_total` | Gauge | `group`, `kind`, `namespace`, `composition`, `creator`, `team`, `source` | Total number of claims |
| `crossplane_claims_ready` | Gauge | `group`, `kind`, `namespace`, `composition`, `creator`, `team`, `source` | Number of claims with Ready=True |
| `crossplane_xr_total` | Gauge | `group`, `kind`, `namespace`, `composition`, `source` | Total number of XRs |
| `crossplane_xr_ready` | Gauge | `group`, `kind`, `namespace`, `composition`, `source` | Number of XRs with Ready=True |

### Label details

- **group** -- API group from the GVR (e.g. `platform.example.org`)
- **kind** -- Resource kind (e.g. `PostgresqlInstance`)
- **namespace** -- Kubernetes namespace (empty for cluster-scoped XRs)
- **composition** -- Crossplane Composition name, extracted from the `COMPOSITION_LABEL_KEY` label on XRs and enriched onto claims via `spec.resourceRef`
- **creator** -- Value of the annotation specified by `CREATOR_ANNOTATION_KEY` (claims only)
- **team** -- Value of the annotation specified by `TEAM_ANNOTATION_KEY` (claims only)
- **source** -- Configuration origin: `"central"` for GVRs defined via environment variables, `"namespace"` for GVRs from per-namespace ConfigMaps

### Example output

Output from `curl localhost:8080/metrics` with the sample resources applied (`make samples-apply`):

```
# HELP crossplane_claims_ready Number of Ready Crossplane claims by group, kind, namespace, composition, creator, team and source.
# TYPE crossplane_claims_ready gauge
crossplane_claims_ready{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",source="central",team="platform"} 0
crossplane_claims_ready{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",source="central",team="platform"} 0
crossplane_claims_ready{composition="",creator="bob@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-beta",source="central",team="backend"} 0
crossplane_claims_ready{composition="",creator="carol@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-beta",source="central",team="backend"} 0
crossplane_claims_ready{composition="",creator="dave@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",source="central",team="platform"} 0
crossplane_claims_ready{composition="",creator="eve@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-gamma",source="central",team="data"} 0
# HELP crossplane_claims_total Number of Crossplane claims by group, kind, namespace, composition, creator, team and source.
# TYPE crossplane_claims_total gauge
crossplane_claims_total{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",source="central",team="platform"} 1
crossplane_claims_total{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",source="central",team="platform"} 2
crossplane_claims_total{composition="",creator="bob@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-beta",source="central",team="backend"} 1
crossplane_claims_total{composition="",creator="carol@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-beta",source="central",team="backend"} 1
crossplane_claims_total{composition="",creator="dave@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",source="central",team="platform"} 1
crossplane_claims_total{composition="",creator="eve@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-gamma",source="central",team="data"} 2
# HELP crossplane_xr_ready Number of Ready Crossplane XRs by group, kind, namespace, composition and source.
# TYPE crossplane_xr_ready gauge
crossplane_xr_ready{composition="",group="samples.xptracker.dev",kind="XGadget",namespace="",source="central"} 0
crossplane_xr_ready{composition="",group="samples.xptracker.dev",kind="XWidget",namespace="",source="central"} 0
# HELP crossplane_xr_total Number of Crossplane composite resources (XRs) by group, kind, namespace, composition and source.
# TYPE crossplane_xr_total gauge
crossplane_xr_total{composition="",group="samples.xptracker.dev",kind="XGadget",namespace="",source="central"} 4
crossplane_xr_total{composition="",group="samples.xptracker.dev",kind="XWidget",namespace="",source="central"} 4
```

## Bookkeeping JSON Endpoint

In addition to Prometheus metrics, the exporter exposes a JSON endpoint that returns the full in-memory snapshot of claims and XRs. This is useful for ad-hoc debugging, CLI tools, or external integrations that don't want to go through PromQL.

### Endpoint

```
GET /bookkeeping
```

Returns `Content-Type: application/json; charset=utf-8` with HTTP 200.

### Response format

```json
{
  "claims": [
    {
      "group": "platform.example.org",
      "kind": "PostgreSQLInstance",
      "namespace": "team-a",
      "name": "db-123",
      "creator": "alice@example.com",
      "team": "payments",
      "composition": "postgres-small",
      "source": "central",
      "ready": true,
      "reason": "Ready",
      "ageSeconds": 12345
    }
  ],
  "xrs": [
    {
      "group": "platform.example.org",
      "kind": "XPostgreSQLInstance",
      "namespace": "",
      "name": "db-123-xyz",
      "composition": "postgres-small",
      "source": "central",
      "ready": true,
      "reason": "Ready",
      "ageSeconds": 12300
    }
  ],
  "generatedAt": "2026-02-13T20:50:00Z"
}
```

### Fields

- **ageSeconds** -- seconds since `metadata.creationTimestamp`, computed at response time.
- **generatedAt** -- ISO 8601 / RFC 3339 UTC timestamp of when the response was rendered.

### Usage examples

```bash
# Full snapshot
curl -s localhost:8080/bookkeeping | jq .

# Count claims by namespace
curl -s localhost:8080/bookkeeping | jq '[.claims[] | .namespace] | group_by(.) | map({(.[0]): length}) | add'

# List not-ready claims
curl -s localhost:8080/bookkeeping | jq '[.claims[] | select(.ready == false)]'

# Get all XR compositions
curl -s localhost:8080/bookkeeping | jq '[.xrs[].composition] | unique'
```

### Notes

- The endpoint reflects the **last completed polling cycle** and is eventually consistent.
- No authentication is required; the endpoint is intended for cluster-internal use. Restrict access via Kubernetes NetworkPolicy if needed.
- In large clusters the payload may be substantial. Pagination/filtering may be added in future versions.

## Health Endpoints

The exporter exposes two health endpoints for Kubernetes probes:

| Endpoint | Purpose | Behaviour |
|---|---|---|
| `GET /healthz` | Liveness probe | Always returns `200 OK` with body `ok` |
| `GET /readyz` | Readiness probe | Returns `503 Service Unavailable` until the first poll cycle completes, then `200 OK` with body `ok` |

The base Deployment manifests configure Kubernetes liveness and readiness probes against these endpoints. The readiness probe prevents traffic from reaching the exporter until it has populated the in-memory store with at least one polling cycle.

## Deployment

The exporter ships with [Kustomize](https://kustomize.io/) manifests.

### Prerequisites

- A Kubernetes cluster with Crossplane installed
- Crossplane CRDs (XRDs) and Compositions deployed
- `kubectl` with access to the cluster

### Quick start (base)

The base deploys to the `crossplane-system` namespace:

```bash
# Review what will be applied
kubectl kustomize deploy/base

# Apply
kubectl apply -k deploy/base
```

### Using the example overlay

The example overlay shows how to customise GVRs, add a ServiceMonitor, and pin the image tag:

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
        CLAIM_GVRS: "myorg.io/v1alpha1/databases,myorg.io/v1alpha1/caches"
        XR_GVRS: "myorg.io/v1alpha1/xdatabases,myorg.io/v1alpha1/xcaches"
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

When using per-namespace ConfigMaps, the exporter also needs `get`, `list`, and `watch` access to ConfigMaps across namespaces. The base ClusterRole already includes this rule:

```yaml
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
```

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

# Ready claims by composition
sum by (composition)(crossplane_claims_ready)

# Claims by creator
sum by (creator)(crossplane_claims_total)

# Claim readiness ratio by namespace
sum by (namespace)(crossplane_claims_ready) / sum by (namespace)(crossplane_claims_total)

# XR readiness ratio by composition
sum by (composition)(crossplane_xr_ready) / sum by (composition)(crossplane_xr_total)

# Not-ready claims (total minus ready)
sum by (namespace)(crossplane_claims_total) - sum by (namespace)(crossplane_claims_ready)

# All XRs grouped by kind
sum by (kind)(crossplane_xr_total)
```

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

The Dockerfile uses a multi-stage build with BuildKit multi-arch support (`linux/amd64` + `linux/arm64`). Build stage: `golang:1.25-alpine`. Runtime stage: `gcr.io/distroless/static-debian12:nonroot`. The final image is ~10 MB and runs as a non-root user.

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
   - On `main` push: builds and pushes with tags `latest` and `sha-<short>`

### Release (`.github/workflows/release.yml`)

Triggered by pushing a semver tag (`v*`):

1. **Test & Lint** -- Same gate as CI
2. **Build, Push & Release** -- Builds multi-arch image with embedded version info, pushes to GHCR, creates a GitHub Release

**Image tags produced by a release (e.g. `v1.2.3`):**

| Tag | Example |
|---|---|
| `{{version}}` | `1.2.3` |
| `{{major}}.{{minor}}` | `1.2` |
| `{{major}}` (if >= 1) | `1` |

Pre-release tags (containing `-rc`, `-alpha`, `-beta`) are marked as pre-releases on GitHub.

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

## Local Development

### Prerequisites

- Go 1.25+
- A Kubernetes cluster (kind, k3d, minikube, etc.) with Crossplane installed
- `golangci-lint` for linting
- [mise](https://mise.jdx.dev/) (recommended) -- run `mise install` to set up pinned tool versions

### Build and run locally

```bash
# Build
make build

# Run against a local cluster (uses KUBECONFIG or ~/.kube/config)
export CLAIM_GVRS="platform.example.org/v1alpha1/postgresqlinstances"
export XR_GVRS="platform.example.org/v1alpha1/xpostgresqlinstances"
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

# Run the exporter against the sample resources
export CLAIM_GVRS="samples.xptracker.dev/v1alpha1/widgets,samples.xptracker.dev/v1alpha1/gadgets"
export XR_GVRS="samples.xptracker.dev/v1alpha1/xwidgets,samples.xptracker.dev/v1alpha1/xgadgets"
export CREATOR_ANNOTATION_KEY="xptracker.dev/created-by"
export TEAM_ANNOTATION_KEY="xptracker.dev/team"
make run

# In another terminal -- check metrics
curl -s localhost:8080/metrics | grep crossplane_

# Check bookkeeping
curl -s localhost:8080/bookkeeping | jq .

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
export CLAIM_GVRS="your.org/v1alpha1/yourclaims"
export XR_GVRS="your.org/v1alpha1/yourxrs"
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
│   │   └── namespace_config.go      # Per-namespace ConfigMap parsing + label constants
│   ├── kube/
│   │   ├── client.go                # Dynamic + typed client factory (in-cluster + kubeconfig fallback)
│   │   ├── configmap_watcher.go     # ConfigMap informer with label-selector discovery
│   │   ├── convert.go               # Unstructured -> ClaimInfo/XRInfo conversion
│   │   └── poller.go                # Ticker-based polling loop with composition enrichment
│   ├── metrics/
│   │   ├── claim_collector.go       # ClaimCollector (Describe/Collect)
│   │   ├── xr_collector.go          # XRCollector (Describe/Collect)
│   │   └── self.go                  # Self-monitoring metrics (xp_tracker_* prefix)
│   ├── server/
│   │   ├── server.go                # HTTP server with custom Prometheus registry
│   │   └── bookkeeping.go           # JSON bookkeeping endpoint (/bookkeeping)
│   └── store/
│       ├── store.go                 # Store interface + MemoryStore implementation
│       └── s3store.go               # S3Store persistent backend (decorator over MemoryStore)
├── deploy/
│   ├── base/                        # Kustomize base (SA, RBAC, ConfigMap, Deployment, Service)
│   │   └── sample-namespace-configmap.yaml  # Example per-namespace ConfigMap for teams
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
- Verify the `CLAIM_GVRS` and `XR_GVRS` values match your installed CRDs: `kubectl api-resources | grep your-group`
- If using `KUBE_NAMESPACE_SCOPE`, ensure the namespaces exist and contain claims

### Metrics show 0 for ready counts

- Crossplane claims and XRs report readiness via `status.conditions` with `type: Ready` and `status: "True"`. If your resources use a different condition, the exporter won't detect them as ready.

### RBAC errors in logs

- The ClusterRole needs `get` and `list` permissions for all configured GVRs. Check your ClusterRole rules match the API groups and resources in `CLAIM_GVRS` and `XR_GVRS`.

### Stale metrics after deleting resources

- The exporter does a full replace on each polling cycle. Deleted resources will disappear from metrics after the next poll (default: 30 seconds).

### Composition label is empty

- The exporter reads the composition from the `COMPOSITION_LABEL_KEY` label on XRs (default: `crossplane.io/composition-name`). If your XRs don't have this label, set `COMPOSITION_LABEL_KEY` to the correct label key.
- Claims get their composition via `spec.resourceRef.name` -> XR lookup. If the claim has no `spec.resourceRef`, the composition will be empty until the XR is created and linked.

### Self-monitoring metrics

xp-tracker also exposes metrics about its own operation under the `xp_tracker_` prefix. These are useful for alerting on poller failures or slow S3 persistence.

| Metric | Type | Description |
|---|---|---|
| `xp_tracker_poll_duration_seconds` | Histogram | Duration of each poll cycle |
| `xp_tracker_poll_errors_total` | Counter | Total number of per-GVR poll errors |
| `xp_tracker_store_claims` | Gauge | Current number of claims in the store |
| `xp_tracker_store_xrs` | Gauge | Current number of XRs in the store |
| `xp_tracker_namespace_configs` | Gauge | Number of active per-namespace ConfigMaps discovered |
| `xp_tracker_s3_persist_duration_seconds` | Histogram | Duration of each S3 persist operation |

### Single replica requirement

- Running more than one replica will result in double-counted metrics since each replica independently polls and serves metrics. Keep `replicas: 1`.
