# Environment Variables

All xp-tracker configuration is via environment variables. There are no config files or command-line flags.

## Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `CLAIM_GVRS` | No (deprecated) | `""` | Optional static claim GVR override in `group/version/resource` format |
| `XR_GVRS` | No (deprecated) | `""` | Optional static XR GVR override in `group/version/resource` format |
| `KUBE_NAMESPACE_SCOPE` | No | `""` (all) | Comma-separated namespace filter |
| `CREATOR_ANNOTATION_KEY` | No | `""` | Annotation key for claim creator attribution |
| `TEAM_ANNOTATION_KEY` | No | `""` | Annotation key for team attribution |
| `COMPOSITION_LABEL_KEY` | No | `crossplane.io/composition-name` | Label key on XRs for composition name |
| `COMPOSITE_LABEL_KEY` | No | `crossplane.io/composite` | Label key on MRs linking them to a composite (XR) |
| `MR_GVRS` | No | `""` | Additional MR GVRs to poll (`group/version/resource`), merged with MRD discovery |
| `POLL_INTERVAL_SECONDS` | No | `30` | Seconds between polling cycles |
| `METRICS_ADDR` | No | `:8080` | Listen address for the HTTP metrics server |
| `STORE_BACKEND` | No | `memory` | Persistent store backend: `memory` or `s3` |
| `S3_BUCKET` | When `s3` | `""` | S3 bucket name |
| `S3_KEY_PREFIX` | No | `xp-tracker` | S3 key prefix for snapshot file |
| `S3_REGION` | No | `us-east-1` | AWS region for S3 client |
| `S3_ENDPOINT` | No | `""` | Custom S3 endpoint (MinIO, LocalStack) |

## XRD discovery

xp-tracker now discovers claim and XR GVRs from Crossplane `CompositeResourceDefinition` (XRD) objects at startup. The exporter derives:

1. XR GVR from `spec.group` + selected `spec.versions[].name` + `spec.names.plural`
2. Claim GVR from `spec.group` + selected `spec.versions[].name` + `spec.claimNames.plural` (when present)

Version selection is deterministic: first `referenceable` version, otherwise first `served` version.

If no XRD-backed claim or XR resources can be discovered, startup fails with a clear error.

## Provider MR discovery

xp-tracker discovers provider Managed Resource (MR) GVRs from Crossplane `ManagedResourceDefinition` objects at startup. The exporter:

1. Lists `managedresourcedefinitions.apiextensions.crossplane.io/v1alpha1`
2. Keeps only MRDs with `spec.state: Active` (types with a live CRD)
3. Derives each MR GVR from `spec.group` + storage/served version + `spec.names.plural`
4. Attributes the provider package from `pkg.crossplane.io/package` or a `Provider` owner reference
5. Merges any additional GVRs from `MR_GVRS` (deduplicated)

During polling, only MRs with the composite label (`crossplane.io/composite` by default) are tracked. Claim linkage is enriched from MR claim labels or the backing XR.

An empty MR GVR list is valid (for example, when MRD conversion is disabled — use `MR_GVRS` in that case).

## Static GVR override format (deprecated)

Each GVR must be specified in `group/version/resource` format. The resource name is the **plural lowercase** form (the same string you'd use with `kubectl get`).

```bash
CLAIM_GVRS="platform.example.org/v1alpha1/postgresqlinstances,platform.example.org/v1alpha1/kafkatopics"
XR_GVRS="platform.example.org/v1alpha1/xpostgresqlinstances,platform.example.org/v1alpha1/xkafkatopics"
```

!!! tip "Finding your GVRs for override mode"
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
    Namespace filtering only applies to namespace-scoped resources (claims). Cluster-scoped XRs are always polled globally.

## Annotation keys

The `CREATOR_ANNOTATION_KEY` and `TEAM_ANNOTATION_KEY` variables tell xp-tracker which annotations on claims contain the creator and team information. These are used as Prometheus labels for attribution-based queries.

```bash
CREATOR_ANNOTATION_KEY="myorg.io/created-by"
TEAM_ANNOTATION_KEY="myorg.io/team"
```

If the annotation is not present on a claim, the label value will be an empty string.

## Composition label

The `COMPOSITION_LABEL_KEY` tells xp-tracker which label on XRs contains the Composition name. The default (`crossplane.io/composition-name`) works with standard Crossplane installations.

Claims get their composition value through a two-step enrichment:

1. The claim's `spec.resourceRef.name` identifies the backing XR
2. The XR's composition label value is copied to the claim

XRs get their claim linkage through a symmetric enrichment when labels are missing:

1. The XR's `crossplane.io/claim-name` and `crossplane.io/claim-namespace` labels are used when present
2. Otherwise, xp-tracker finds the claim whose `spec.resourceRef.name` matches the XR name and copies the claim's name and namespace

## Composite label (MRs)

The `COMPOSITE_LABEL_KEY` tells xp-tracker which label on provider MRs links them to a composite (XR). The default (`crossplane.io/composite`) matches standard Crossplane installations.

MRs are only polled when this label is present. Claim linkage is enriched in two steps:

1. Direct `crossplane.io/claim-name` and `crossplane.io/claim-namespace` labels on the MR are used when present
2. Otherwise, xp-tracker looks up the XR named by the composite label and copies the XR's claim name and namespace

## Deployment via ConfigMap

In the Kustomize manifests, environment variables are stored in a ConfigMap and injected via `envFrom`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: crossplane-metrics-exporter
data:
  # Optional overrides (deprecated when discovery is enabled):
  # CLAIM_GVRS: "platform.example.org/v1alpha1/postgresqlinstances"
  # XR_GVRS: "platform.example.org/v1alpha1/xpostgresqlinstances"
  CREATOR_ANNOTATION_KEY: "platform.example.org/created-by"
  TEAM_ANNOTATION_KEY: "platform.example.org/team"
  POLL_INTERVAL_SECONDS: "30"
  METRICS_ADDR: ":8080"
```
