# Environment Variables

All xp-tracker configuration is via environment variables. There are no config files or command-line flags.

## Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `CLAIM_GVRS` | No (deprecated) | `""` | Optional additional claim GVRs (`group/version/resource`), merged with XRD discovery (deduplicated) |
| `XR_GVRS` | No (deprecated) | `""` | Optional additional XR GVRs (`group/version/resource`), merged with XRD discovery (deduplicated) |
| `KUBE_NAMESPACE_SCOPE` | No | `""` (all) | Comma-separated namespace filter |
| `CREATOR_ANNOTATION_KEY` | No | `""` | Annotation key for claim creator attribution |
| `TEAM_ANNOTATION_KEY` | No | `""` | Annotation key for team attribution |
| `COMPOSITION_LABEL_KEY` | No | `crossplane.io/composition-name` | Label key on XRs for composition name (internal enrichment only; not exposed as a metric label) |
| `COMPOSITE_LABEL_KEY` | No | `crossplane.io/composite` | Label key on MRs linking them to a composite (XR) |
| `MR_XR_PARENT_LABELS` | No | `true` | When true, `crossplane_mr_*` include typed XR parent labels `xr_kind`, `xr_namespace`, `xr_group`. Set `false`/`0`/`no` to omit them |
| `XR_PARENT_LABELS` | No | `true` | When true, `crossplane_xr_*` include `parent_kind`, `parent_name`, `parent_namespace` from XR `ownerReferences` (validated against inventory). Roots keep empty `parent_name`. Set `false`/`0`/`no` to omit them |
| `MR_OWNER_XR_LABELS` | No | `true` | When true, `crossplane_mr_*` include `owner_xr_name`, `owner_xr_kind`, `owner_xr_namespace`, `owner_xr_group` for the immediate composing XR. Set `false`/`0`/`no` to omit them |
| `MR_GVRS` | No | `""` | Additional MR GVRs to poll (`group/version/resource`), merged with MRD discovery |
| `POLL_INTERVAL_SECONDS` | No | `30` | Seconds between polling cycles |
| `DISCOVERY_INTERVAL_SECONDS` | No | `300` | Seconds between XRD/MRD re-discovery cycles. `0` disables periodic re-discovery (startup discovery still runs once) |
| `METRICS_ADDR` | No | `:8080` | Listen address for the HTTP metrics server |
| `LOG_LEVEL` | No | `info` | Structured log level: `debug`, `info`, `warn`, or `error` |
| `STORE_BACKEND` | No | `memory` | Persistent store backend: `memory` or `s3` |
| `S3_BUCKET` | When `s3` | `""` | S3 bucket name |
| `S3_KEY_PREFIX` | No | `xp-tracker` | S3 key prefix for snapshot file |
| `S3_REGION` | No | `us-east-1` | AWS region for S3 client |
| `S3_ENDPOINT` | No | `""` | Custom S3 endpoint (MinIO, LocalStack) |

## XRD discovery

xp-tracker discovers claim and XR GVRs from Crossplane v1 and v2 `CompositeResourceDefinition` (XRD) objects at startup and, by default, again every `DISCOVERY_INTERVAL_SECONDS` (300s). The exporter derives:

1. XR GVR from `spec.group` + selected `spec.versions[].name` + `spec.names.plural`
2. Claim GVR from `spec.group` + selected `spec.versions[].name` + `spec.claimNames.plural` (when present on a legacy XRD)

Version selection is deterministic: first `referenceable` version, otherwise first `served` version. An XRD's `spec.scope` controls whether its XR is listed across the cluster or in each selected namespace. Crossplane v2 defaults XRDs to `Namespaced`; v1 XRDs default to the claim-compatible `LegacyCluster` scope.

If an XRD's spec cannot yield a GVR (for example, no version with `referenceable: true` or `served: true`), that XRD is skipped with a warning and counted in `xp_tracker_xrd_discovery_skipped_total` rather than failing discovery. Listing XRDs still fails fatally.

Startup requires at least one XR discovered from XRDs. Claims are optional: native Crossplane v2 XRDs do not support them. If `CLAIM_GVRS` or `XR_GVRS` are set, those values are merged into the discovered sets (deduplicated) and a deprecation warning is logged. They do not replace discovery. The same merge is reapplied on every re-discovery cycle.

Set `DISCOVERY_INTERVAL_SECONDS=0` to disable periodic re-discovery (startup discovery still runs). A failed refresh leaves the previous poll inventory in place and increments `xp_tracker_discovery_runs_total{result="error"}`.

## Provider MR discovery

xp-tracker discovers provider Managed Resource (MR) GVRs from Crossplane `ManagedResourceDefinition` objects at startup and on each re-discovery cycle. The exporter:

1. Lists `managedresourcedefinitions.apiextensions.crossplane.io/v1alpha1`
2. Keeps only MRDs with `spec.state: Active` (types with a live CRD)
3. Derives each MR GVR from `spec.group` + storage/served version + `spec.names.plural`
4. Attributes the provider package from `pkg.crossplane.io/package` or a `Provider` owner reference
5. Merges any additional GVRs from `MR_GVRS` (deduplicated)

If an Active MRD's spec cannot yield a GVR (for example, no version with `storage: true` or `served: true`), that MRD is skipped with a warning and counted in `xp_tracker_mrd_discovery_skipped_total` rather than failing discovery. A known cause is truncated MRDs written by the Crossplane package establisher ([crossplane/crossplane#7817](https://github.com/crossplane/crossplane/issues/7817)); repair the cluster object (or upgrade Crossplane once the fix lands) so the missing MR type is tracked again.

During polling, only MRs with the composite label (`crossplane.io/composite` by default) are tracked. Claim linkage is enriched from MR claim labels or the backing XR when a legacy claim exists. Namespaced v2 MRs are matched to an XR in the same namespace.

An empty MR GVR list is valid (for example, when MRD conversion is disabled — use `MR_GVRS` in that case).

## Additional claim/XR GVRs (deprecated)

`CLAIM_GVRS` and `XR_GVRS` are deprecated additive overrides. Each GVR must be specified in `group/version/resource` format. The resource name is the **plural lowercase** form (the same string you'd use with `kubectl get`). Values are merged with XRD discovery and deduplicated; discovery remains required for startup.

```bash
CLAIM_GVRS="platform.example.org/v1alpha1/postgresqlinstances,platform.example.org/v1alpha1/kafkatopics"
XR_GVRS="platform.example.org/v1alpha1/xpostgresqlinstances,platform.example.org/v1alpha1/xkafkatopics"
```

!!! tip "Finding your GVRs for additive overrides"
    Use `kubectl api-resources` to find the correct group, version, and resource name:

    ```bash
    kubectl api-resources | grep platform.example.org
    ```

## Namespace filtering

By default, xp-tracker polls all namespaces. To restrict to specific namespaces:

```bash
KUBE_NAMESPACE_SCOPE="team-a,team-b,team-c"
```

!!! note
    Namespace filtering applies to all namespace-scoped resources: claims, v2 namespaced XRs, and namespaced MRs. Cluster-scoped XRs and MRs are always polled globally.

## Annotation keys

The `CREATOR_ANNOTATION_KEY` and `TEAM_ANNOTATION_KEY` variables tell xp-tracker which annotations on claims contain the creator and team information. These are used as Prometheus labels for attribution-based queries.

```bash
CREATOR_ANNOTATION_KEY="myorg.io/created-by"
TEAM_ANNOTATION_KEY="myorg.io/team"
```

If the annotation is not present on a claim, the label value will be an empty string.

## Composition label

!!! note
    Composition is resolved internally and persisted in store snapshots, but it is **not** exposed as a Prometheus label on any metric. Most deployments can leave `COMPOSITION_LABEL_KEY` at its default.

The `COMPOSITION_LABEL_KEY` tells xp-tracker which label on XRs contains the Composition name. The default (`crossplane.io/composition-name`) works with standard Crossplane installations. For v2 XRs without this label, xp-tracker falls back to `spec.crossplane.compositionRef.name`.

Claims get their composition value through a two-step enrichment:

1. The claim's `spec.resourceRef.name` identifies the backing XR
2. The XR's composition label value is copied to the claim

Legacy XRs get their claim linkage through a symmetric enrichment when labels are missing:

1. The XR's `crossplane.io/claim-name` and `crossplane.io/claim-namespace` labels are used when present
2. Otherwise, xp-tracker finds the claim whose `spec.resourceRef.name` matches the XR name and copies the claim's name and namespace

Native Crossplane v2 XRs have no claims, so their claim metric labels remain empty.

## XR parent labels (XR nesting)

With `XR_PARENT_LABELS=true` (the default), each `crossplane_xr_*` series carries the immediate parent XR from Kubernetes `metadata.ownerReferences`:

1. Prefer the `controller: true` owner whose GVK is XR-like (not a Claim, not a core Kubernetes / Crossplane package type)
2. Else use a single XR-like owner; multiple non-controller XR owners stay empty (ambiguous)
3. After the poll cycle, validate the candidate against inventoried XRs and fill `parent_namespace` from the matched parent (empty when the parent is cluster-scoped)

Root XRs emit empty `parent_name` (and empty `parent_kind` / `parent_namespace`). That is the explicit “no parent” signal — dashboards should filter `parent_name=""` for roots rather than inventing sentinels. Claims remain on `claim_*` and are never treated as `parent_*`.

## Composite label (MRs)

The `COMPOSITE_LABEL_KEY` tells xp-tracker which label on provider MRs links them to a composite (XR). The default (`crossplane.io/composite`) matches standard Crossplane installations.

MRs are only polled when this label is present. After each poll, xp-tracker looks up the XR named by the composite label (prefer the MR's namespace, then a cluster-scoped XR) and:

1. Always copies typed XR identity into store fields used for `xr_kind`, `xr_namespace`, and `xr_group` metric labels when `MR_XR_PARENT_LABELS` is true (the default)
2. Copies legacy `claim_name` / `claim_namespace` from the XR only when those fields are not already set from MR labels

Primary ownership for modern (v2) fleets is XR → MR via `xr_name` plus the typed `xr_*` labels. Empty `claim_*` on native v2 trees is expected.

## MR owner XR labels (immediate placement)

With `MR_OWNER_XR_LABELS=true` (the default), each `crossplane_mr_*` series carries the **immediate** composing XR, distinct from composite-label `xr_*`:

1. Prefer an XR-like `metadata.ownerReferences` entry (`controller: true` first), validated against inventoried XRs
2. Else find a unique inventoried XR whose `spec.resourceRefs` / `spec.crossplane.resourceRefs` lists this MR (GVK+name; same namespace when namespaced)
3. Ambiguous matches stay empty — never copy `xr_name` into `owner_xr_name`

In flat trees both families usually agree. In nested trees `xr_name` may name the root while `owner_xr_*` names the intermediate XR.

## Deployment via ConfigMap

In the Kustomize manifests, environment variables are stored in a ConfigMap and injected via `envFrom`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: crossplane-metrics-exporter
data:
  # Optional additive overrides (deprecated; merged with XRD discovery):
  # CLAIM_GVRS: "platform.example.org/v1alpha1/postgresqlinstances"
  # XR_GVRS: "platform.example.org/v1alpha1/xpostgresqlinstances"
  CREATOR_ANNOTATION_KEY: "platform.example.org/created-by"
  TEAM_ANNOTATION_KEY: "platform.example.org/team"
  POLL_INTERVAL_SECONDS: "30"
  # DISCOVERY_INTERVAL_SECONDS: "300"  # 0 disables periodic re-discovery
  METRICS_ADDR: ":8080"
  # LOG_LEVEL: "info"  # debug|info|warn|error
```

## Logging

`LOG_LEVEL` controls the slog JSON handler used by the exporter process (`debug`, `info`, `warn`, or `error`). The default is `info`. Set `debug` temporarily when diagnosing poll or discovery issues; keep production at `info` or `warn` to limit volume.
