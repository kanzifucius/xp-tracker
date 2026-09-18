---
hide:
  - navigation
---

# xp-tracker

<p align="center">
  <a href="https://github.com/kanzifucius/xp-tracker/releases/latest"><img src="https://img.shields.io/github/v/release/kanzifucius/xp-tracker?style=flat" alt="Release"></a>
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License">
  <img src="https://img.shields.io/badge/Crossplane-2.0+-7C3AED?style=flat" alt="Crossplane">
</p>

A minimal, read-only Prometheus exporter for [Crossplane](https://www.crossplane.io/) claims, composite resources (XRs), and XR-linked provider managed resources (MRs).

It polls the Kubernetes API via the dynamic client, aggregates resource counts by meaningful labels, and exposes them as Prometheus gauge metrics on `/metrics`.

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

**Business-level dimensions**
:   Every metric is broken down by `creator`, `team`, and `namespace`, plus per-resource status labels (`ready`, `reason`, `paused`, `deleting`). These are the dimensions that matter when you're running a platform, not just an operator.

**Inventory and adoption tracking**
:   Get real answers to "how many claims of each type exist?", "which namespaces are using the platform?", and "which resources are not ready, and for what reason?" -- all via standard PromQL queries and Grafana dashboards.

**Chargeback and showback**
:   The `creator` + `team` + `namespace` labels make it straightforward to build cost-allocation or usage-reporting dashboards per team or business unit.

**Dynamic, zero-codegen**
:   Works with any Crossplane CRD without code generation or recompilation. Just configure your GVRs as environment variables and deploy.

### Standard Crossplane metrics vs xp-tracker

| Dimension | Crossplane built-in | xp-tracker |
|---|---|---|
| Reconcile latency / errors | :material-check: | -- |
| Workqueue depth | :material-check: | -- |
| Claim count by namespace | -- | :material-check: |
| Claim count by creator | -- | :material-check: |
| Claim count by team | -- | :material-check: |
| Readiness ratio by namespace / team | -- | :material-check: |
| XR count by kind / claim linkage | -- | :material-check: |
| MR count by provider / claim | -- | :material-check: |
| Stuck or not-ready resources by reason | -- | :material-check: |

!!! tip "In short"
    Crossplane tells you how the *controller* is doing. xp-tracker tells you what *resources* exist, who owns them, and whether they're healthy -- the information platform teams need to run an internal developer platform.

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

## How it works

```mermaid
graph TD
    A[Kubernetes API] -->|List / Watch| B[Poller<br/><small>pkg/kube</small>]
    B -->|ReplaceClaims / ReplaceXRs / ReplaceMRs<br/>Enrich claim linkage| C[In-Memory Store<br/><small>pkg/store</small>]
    C -->|SnapshotClaims / SnapshotXRs / SnapshotMRs| D[Claim, XR & MR Collectors<br/><small>pkg/metrics</small>]
    D --> E[HTTP Server<br/><small>pkg/server</small>]
    E -->|GET /metrics| F[Prometheus]
    F --> H[Grafana]

    style A fill:#326CE5,color:#fff,stroke:#326CE5
    style F fill:#E6522C,color:#fff,stroke:#E6522C
    style H fill:#F46800,color:#fff,stroke:#F46800
```

## Key features

- :material-shield-lock-outline: **Read-only** -- only `get`, `list`, and `watch` operations against the Kubernetes API. Never creates, updates, or deletes resources.
- :material-auto-fix: **Dynamic client** -- works with any Crossplane CRD without code generation. Claim, XR, and MR GVRs are discovered from XRDs and ManagedResourceDefinitions at startup.
- :material-chart-bar: **Claim metrics** -- total, ready, status, and timestamp gauges broken down by group, kind, version, namespace, creator, team, claim name, and status labels.
- :material-chart-donut: **XR metrics** -- total, ready, status, and timestamp gauges broken down by group, kind, version, namespace, name, linked claim, and status labels.
- :material-cube-outline: **MR metrics** -- total, ready, status, and timestamp gauges for XR-linked provider managed resources, broken down by provider, provider config, external name, management policies, linked XR/claim, and status labels.
- :material-link-variant: **Claim linkage enrichment** -- legacy XRs and MRs are enriched with `claim_name` / `claim_namespace` by following `spec.resourceRef` and the composite label when Crossplane labels are missing.
- :material-swap-horizontal: **Pluggable store** -- the in-memory data layer is behind a `store.Store` interface. An S3-backed persistent store is included for surviving restarts.
- :material-feather: **Lightweight** -- single binary, ~10 MB distroless container image, minimal resource footprint.
- :material-chip: **Multi-arch** -- container images built for `linux/amd64` and `linux/arm64`.

## Next steps

<div class="grid cards" markdown>

- :material-download: **[Installation](getting-started/installation.md)**

    ---

    Get xp-tracker running in your cluster

- :material-cog: **[Configuration](configuration/environment-variables.md)**

    ---

    All environment variables and their defaults

- :material-gauge: **[Metrics Reference](metrics/reference.md)**

    ---

    Every Prometheus gauge and its labels

- :material-kubernetes: **[Deployment](deployment/kustomize.md)**

    ---

    Kustomize base and overlays

- :material-heart-pulse: **[Health Endpoints](api/health.md)**

    ---

    Liveness and readiness probes for Kubernetes

- :material-chart-line: **[Grafana Queries](metrics/grafana-queries.md)**

    ---

    Example PromQL queries for dashboards

</div>
