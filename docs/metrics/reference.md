# Metrics Reference

xp-tracker exposes twenty-one Prometheus **gauge** metrics for Crossplane resources (claims, XRs, MRs, and packages), plus twelve **self-monitoring** metrics for operational visibility.

## Claim metrics

### `crossplane_claims_total`

Total number of legacy Crossplane claims, broken down by label tuple. Crossplane v2 namespaced and modern cluster-scoped XRs do not create claim metrics because they do not support claims.

| Label | Description |
|---|---|
| `group` | API group from the GVR (e.g. `platform.example.org`) |
| `kind` | Resource kind (e.g. `PostgresqlInstance`) |
| `version` | API version from the GVR (e.g. `v1alpha1`) |
| `namespace` | Kubernetes namespace |
| `creator` | Value of the `CREATOR_ANNOTATION_KEY` annotation |
| `team` | Value of the `TEAM_ANNOTATION_KEY` annotation |
| `claim_name` | Claim metadata name |
| `synced` | Crossplane `Synced` condition status (`true`/`false`) |
| `ready` | Crossplane `Ready` condition status (`true`/`false`) |
| `reason` | Ready condition reason (e.g. `Available`, `Creating`) |
| `paused` | Whether the `crossplane.io/paused` annotation is set (`true`/`false`) |
| `deleting` | Whether `metadata.deletionTimestamp` is set (`true`/`false`) |

### `crossplane_claims_ready`

Number of Crossplane claims with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_claims_total`.

### `crossplane_claims_status_synced`

Per-claim Synced condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_claims_total`.

### `crossplane_claims_status_ready`

Per-claim Ready condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_claims_total`.

### `crossplane_claims_created_timestamp_seconds`

Unix creation timestamp (`metadata.creationTimestamp`) for each claim. Same label set as `crossplane_claims_total`. Skipped when creation time is unknown.

### `crossplane_claims_deletion_timestamp_seconds`

Unix deletion timestamp (`metadata.deletionTimestamp`) for each claim. Same label set as `crossplane_claims_total`. Emitted only while the claim is being deleted.

## XR metrics

### `crossplane_xr_total`

Total number of Crossplane composite resources (XRs), broken down by label tuple.

| Label | Description |
|---|---|
| `group` | API group from the GVR |
| `kind` | Resource kind (e.g. `XPostgreSQLInstance`) |
| `version` | API version from the GVR |
| `namespace` | Kubernetes namespace; populated for Crossplane v2 namespaced XRs |
| `name` | XR metadata name |
| `parent_kind` | Kind of the immediate parent XR from `metadata.ownerReferences` (validated against inventory). Empty for roots. Omitted when `XR_PARENT_LABELS=false` |
| `parent_name` | Name of the immediate parent XR. Empty for roots (the explicit “no parent” signal). Omitted when `XR_PARENT_LABELS=false` |
| `parent_namespace` | Namespace of the parent XR when namespaced; empty for cluster-scoped parents and roots. Omitted when `XR_PARENT_LABELS=false` |
| `claim_name` | Claim name linked to a legacy XR; empty for native v2 XRs |
| `claim_namespace` | Claim namespace linked to a legacy XR; empty for native v2 XRs |
| `synced` | Crossplane `Synced` condition status (`true`/`false`) |
| `ready` | Crossplane `Ready` condition status (`true`/`false`) |
| `reason` | Ready condition reason |
| `paused` | Whether the `crossplane.io/paused` annotation is set (`true`/`false`) |
| `deleting` | Whether `metadata.deletionTimestamp` is set (`true`/`false`) |

### `crossplane_xr_ready`

Number of XRs with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_xr_total`.

### `crossplane_xr_status_synced`

Per-XR Synced condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_xr_total`.

### `crossplane_xr_status_ready`

Per-XR Ready condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_xr_total`.

### `crossplane_xr_created_timestamp_seconds`

Unix creation timestamp for each XR. Same label set as `crossplane_xr_total`. Skipped when creation time is unknown.

### `crossplane_xr_deletion_timestamp_seconds`

Unix deletion timestamp for each XR. Same label set as `crossplane_xr_total`. Emitted only while the XR is being deleted.

## MR metrics

### `crossplane_mr_total`

Total number of XR-linked provider managed resources (MRs), broken down by label tuple.

| Label | Description |
|---|---|
| `group` | API group from the GVR |
| `kind` | Resource kind (e.g. `NopResource`) |
| `version` | API version from the GVR |
| `namespace` | Kubernetes namespace |
| `name` | MR metadata name |
| `xr_name` | Composite (XR) name from the composite label (often the root in nested trees) |
| `xr_kind` | Kind of the backing XR (from store enrichment); empty if the XR is not yet in the store. Omitted when `MR_XR_PARENT_LABELS=false` |
| `xr_namespace` | Namespace of the backing XR (empty for cluster-scoped XRs). Omitted when `MR_XR_PARENT_LABELS=false` |
| `xr_group` | API group of the backing XR. Omitted when `MR_XR_PARENT_LABELS=false` |
| `owner_xr_name` | Immediate composing XR name from `ownerReferences` (else unique parent `resourceRefs` hit). Distinct from `xr_name`. Omitted when `MR_OWNER_XR_LABELS=false` |
| `owner_xr_kind` | Immediate composing XR kind. Omitted when `MR_OWNER_XR_LABELS=false` |
| `owner_xr_namespace` | Immediate composing XR namespace (empty when cluster-scoped). Omitted when `MR_OWNER_XR_LABELS=false` |
| `owner_xr_group` | Immediate composing XR API group. Omitted when `MR_OWNER_XR_LABELS=false` |
| `claim_name` | Claim name linked to the MR (legacy claim path only); empty for native v2 resources |
| `claim_namespace` | Claim namespace linked to the MR (legacy claim path only); empty for native v2 resources |
| `provider` | Provider package name from MRD discovery (e.g. `provider-nop`) |
| `provider_config` | `spec.providerConfigRef.name` |
| `external_name` | Cloud resource identifier from the `crossplane.io/external-name` annotation (empty until the provider sets it) |
| `management_policies` | Joined `spec.managementPolicies` (e.g. `Observe`, `*`; empty when unset) |
| `synced` | Crossplane `Synced` condition status (`true`/`false`) |
| `ready` | Crossplane `Ready` condition status (`true`/`false`) |
| `reason` | Ready condition reason |
| `paused` | Whether the `crossplane.io/paused` annotation is set (`true`/`false`) |
| `deleting` | Whether `metadata.deletionTimestamp` is set (`true`/`false`) |

### `crossplane_mr_ready`

Number of MRs with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_mr_total`.

### `crossplane_mr_status_synced`

Per-MR Synced condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_mr_total`.

### `crossplane_mr_status_ready`

Per-MR Ready condition status as a gauge (`1` for `true`, `0` for `false`). Same label set as `crossplane_mr_total`.

### `crossplane_mr_created_timestamp_seconds`

Unix creation timestamp for each MR. Same label set as `crossplane_mr_total`. Skipped when creation time is unknown.

### `crossplane_mr_deletion_timestamp_seconds`

Unix deletion timestamp for each MR. Same label set as `crossplane_mr_total`. Emitted only while the MR is being deleted.

## Relationship edge metrics (opt-in)

These are **separate series** for node-graph panels. Defaults are **off**. Prefer Phase 1/2 labels (`parent_*`, `owner_xr_*`) for PromQL joins; enable edges only when a panel needs an edge model.

### `crossplane_xr_child_info`

Gauge `1` per nested child XR with a resolved parent. Enabled with `XR_CHILD_INFO_METRIC=true`. Cardinality ≈ number of nested child XRs.

| Label | Description |
|---|---|
| `parent_group` / `parent_kind` / `parent_namespace` / `parent_name` | Parent XR identity |
| `child_group` / `child_kind` / `child_namespace` / `child_name` | Child XR identity |

### `crossplane_mr_owner_info`

Gauge `1` per MR with a resolved immediate owner XR. Enabled with `MR_OWNER_INFO_METRIC=true`. Cardinality ≈ MR count — **discourage** when fleets have ~1200+ MRs; use `owner_xr_*` labels on existing `crossplane_mr_*` series instead.

| Label | Description |
|---|---|
| `owner_group` / `owner_kind` / `owner_namespace` / `owner_name` | Immediate owner XR |
| `mr_group` / `mr_kind` / `mr_namespace` / `mr_name` | Managed resource |

## Dashboard helper metrics (opt-in)

Defaults are **off**. Enable only when dashboards need stuck-after-ready / recreate-vs-rename helpers.

### `crossplane_xr_ready_last_transition_timestamp_seconds` / `crossplane_mr_ready_last_transition_timestamp_seconds`

Unix timestamp of the Ready condition `lastTransitionTime`. Enabled with `CONDITION_TRANSITION_TIMESTAMPS=true`. Identity labels: `group`, `kind`, `version`, `namespace`, `name` (plus `uid` / `composition_revision` when `RESOURCE_IDENTITY_LABELS=true`).

### `crossplane_xr_condition_info` / `crossplane_mr_condition_info`

Info-style gauge `1` for the Ready condition with `type`, `reason`, and truncated `message` (max 128 runes). Enabled with `CONDITION_MESSAGE_INFO=true`. Treat messages as potentially high-cardinality / PII.

## Package metrics

Low-cardinality inventory for Crossplane package meta-types: **Provider**, **Configuration**, and **Function**. Package revisions (`*Revision`) are **not** scraped.

### `crossplane_packages_total`

Total number of package meta-types, broken down by a small label tuple.

| Label | Description |
|---|---|
| `kind` | Package kind (`Provider`, `Configuration`, or `Function`) |
| `name` | Package metadata name (e.g. `provider-nop`) |
| `healthy` | Crossplane `Healthy` condition status (`true`/`false`) |
| `installed` | Crossplane `Installed` condition status (`true`/`false`) |

### `crossplane_packages_healthy`

Number of packages with `status.conditions` containing `type: Healthy` and `status: "True"`. Same label set as `crossplane_packages_total`.

### `crossplane_packages_installed`

Number of packages with `status.conditions` containing `type: Installed` and `status: "True"`. Same label set as `crossplane_packages_total`.

## Example PromQL

```promql
# Claims older than 15 minutes and still not ready
(time() - crossplane_claims_created_timestamp_seconds > 900) and on(group, kind, namespace, claim_name) crossplane_claims_status_ready == 0

# Resources stuck deleting for more than 10 minutes
time() - crossplane_mr_deletion_timestamp_seconds > 600

# Not-ready resources by Ready condition reason
sum by (reason) (crossplane_claims_status_ready == 0)

# Paused MRs that still look ready
crossplane_mr_status_ready == 1 and on(group, kind, namespace, name) crossplane_mr_total{paused="true"}

# Unhealthy Providers / Functions
crossplane_packages_total{healthy="false"}

# Package health ratio
sum(crossplane_packages_healthy) / sum(crossplane_packages_total)
```

## Aggregation behaviour

Metrics are aggregated by their full label tuple. Because resource names are included, series are emitted per resource.

Claim/XR/MR cardinality tracks inventory size plus status labels. Package series stay small: one series per Provider/Configuration/Function object, with only four labels.

## Label notes

- **Empty labels**: if an annotation key is not configured or the annotation is not present on a resource, the label value is an empty string (`""`).
- **XR claim linkage**: `claim_name` and `claim_namespace` on legacy XR metrics come from XR labels when present. If those labels are absent, xp-tracker backfills them from the claim whose `spec.resourceRef.name` matches the XR name. Native v2 XRs leave both labels empty — that is expected; claims are uncommon on Crossplane v2.
- **XR → XR nesting (primary)**: with `XR_PARENT_LABELS` enabled (the default), `parent_kind` / `parent_name` / `parent_namespace` come from the child’s `metadata.ownerReferences` (prefer `controller: true`), then validated against the inventoried XR set by kind/name/group **and** exact ownerReference UID. Roots emit empty `parent_name` (and empty kind/namespace) — do not invent sentinels. Claim kinds are never treated as `parent_*`. Ambiguous, UID-mismatched, or non-XR owners stay empty rather than guessing. Prefer these labels over kind allowlists or name-suffix heuristics.
- **MR → XR ownership (composite label)**: `xr_name` comes from the composite label. With `MR_XR_PARENT_LABELS` enabled (the default), xp-tracker also sets `xr_kind`, `xr_namespace`, and `xr_group` from the backing XR in the store (prefer same namespace, else cluster-scoped). If more than one XR shares that name in the preferred scope (distinct kinds/groups), the typed labels stay empty rather than guessing. Prefer these labels for joins on modern fleets; do not require `claim_*`.
- **MR → immediate XR (placement)**: with `MR_OWNER_XR_LABELS` enabled (the default), `owner_xr_*` names the XR that immediately owns the MR via `ownerReferences` (prefer `controller: true`, exact UID match), falling back to a unique XR `resourceRefs` match. Never copied from `xr_name`. In nested trees `xr_name` may name the root while `owner_xr_*` names the intermediate Network/KeyVault XR — that difference is intentional.
- **MR claim linkage (legacy)**: `claim_name` and `claim_namespace` on MR metrics come from MR labels when present. Otherwise, xp-tracker copies them from the backing XR when that XR has claim fields. Empty on native v2 trees.
- **MR scope**: only provider MRs with the composite label are tracked.
- **Composition**: composition is resolved internally for enrichment but is not exposed as a Prometheus label dimension.
- **Namespace for XRs**: native Crossplane v2 composite resources are namespaced by default; legacy XRs are cluster-scoped and have an empty `namespace` label.
- **Paused**: `paused="true"` when the `crossplane.io/paused` annotation equals `true` (case-insensitive).
- **Deleting**: `deleting="true"` when `metadata.deletionTimestamp` is set; the matching `*_deletion_timestamp_seconds` gauge is emitted only in that case.

## Self-monitoring metrics

xp-tracker exposes operational metrics about itself under the `xp_tracker_` prefix. These are useful for alerting on poller failures, slow poll cycles, or S3 persistence issues.

### `xp_tracker_poll_duration_seconds`

Histogram tracking the duration of each poll cycle (all GVRs combined).

**Default buckets:** 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60 seconds.

### `xp_tracker_poll_errors_total`

Counter of per-GVR poll errors. Incremented each time a List call for a specific GVR fails.

### `xp_tracker_last_successful_poll_timestamp_seconds`

Unix timestamp (seconds) of the most recent poll cycle that completed with **no** per-GVR List errors. Remains `0` until the first fully successful cycle. Partial failures leave the previous value unchanged so freshness alerts can detect sustained poll problems. See [`prometheus-alerts.md`](prometheus-alerts.md) for `XpTrackerPollStale`.

### `xp_tracker_store_claims`

Gauge showing the current number of claims in the in-memory store, updated after each poll.

### `xp_tracker_store_xrs`

Gauge showing the current number of XRs in the in-memory store, updated after each poll.

### `xp_tracker_store_mrs`

Gauge showing the current number of provider MRs in the in-memory store, updated after each poll.

### `xp_tracker_store_packages`

Gauge showing the current number of package meta-types (Provider/Configuration/Function) in the in-memory store, updated after each poll.

### `xp_tracker_s3_persist_duration_seconds`

Histogram tracking the duration of S3 snapshot persistence. Only emitted when `STORE_BACKEND=s3`.

**Default buckets:** 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30 seconds.

### `xp_tracker_mrd_discovery_skipped_total`

Counter of Active `ManagedResourceDefinition` objects skipped during MR GVR discovery because their spec could not yield a usable group/version/resource. Partitioned by `reason` (`missing_group`, `missing_plural`, `missing_versions`, `no_storage_or_served_version`, `other`). Known reasons are initialised at 0 so alerts on `> 0` work immediately after scrape.

A non-zero value usually means malformed cluster state (for example truncated MRDs from [crossplane/crossplane#7817](https://github.com/crossplane/crossplane/issues/7817)); the exporter continues with the remaining MR types.

### `xp_tracker_xrd_discovery_skipped_total`

Counter of `CompositeResourceDefinition` objects skipped during claim/XR GVR discovery because their spec could not yield a usable group/version/resource. Partitioned by `reason` (`missing_group`, `missing_plural`, `missing_versions`, `no_referenceable_or_served_version`, `other`). Known reasons are initialised at 0 so alerts on `> 0` work immediately after scrape.

A non-zero value usually means malformed cluster state; the exporter continues with the remaining XRDs (listing XRDs still fails fatally). If every XRD is skipped, startup still fails with no XR GVRs discovered.

### `xp_tracker_discovery_runs_total`

Counter of XRD/MRD discovery runs (startup and periodic refresh). Partitioned by `result` (`success` or `error`). Known results are initialised at 0.

### `xp_tracker_discovery_duration_seconds`

Histogram tracking the duration of each XRD/MRD discovery run.

**Default buckets:** Prometheus default buckets.

### Example PromQL for self-monitoring

```promql
# Average poll duration over the last 5 minutes
rate(xp_tracker_poll_duration_seconds_sum[5m]) / rate(xp_tracker_poll_duration_seconds_count[5m])

# Poll error rate
rate(xp_tracker_poll_errors_total[5m])

# Seconds since last fully successful poll
time() - xp_tracker_last_successful_poll_timestamp_seconds

# Current store size
xp_tracker_store_claims + xp_tracker_store_xrs

# 99th percentile S3 persist latency
histogram_quantile(0.99, rate(xp_tracker_s3_persist_duration_seconds_bucket[5m]))

# MRDs skipped at discovery (cluster data-quality)
sum(xp_tracker_mrd_discovery_skipped_total)

# XRDs skipped at discovery (cluster data-quality)
sum(xp_tracker_xrd_discovery_skipped_total)

# Discovery failure rate
rate(xp_tracker_discovery_runs_total{result="error"}[15m])
```
