# Environment Variables

All xp-tracker configuration is via environment variables. There are no config files or command-line flags.

## Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `CLAIM_GVRS` | Yes | -- | Comma-separated claim GVRs in `group/version/resource` format |
| `XR_GVRS` | Yes | -- | Comma-separated XR GVRs in `group/version/resource` format |
| `KUBE_NAMESPACE_SCOPE` | No | `""` (all) | Comma-separated namespace filter |
| `CREATOR_ANNOTATION_KEY` | No | `""` | Annotation key for claim creator attribution |
| `TEAM_ANNOTATION_KEY` | No | `""` | Annotation key for team attribution |
| `COMPOSITION_LABEL_KEY` | No | `crossplane.io/composition-name` | Label key on XRs for composition name |
| `POLL_INTERVAL_SECONDS` | No | `30` | Seconds between polling cycles |
| `METRICS_ADDR` | No | `:8080` | Listen address for the HTTP metrics server |
| `STORE_BACKEND` | No | `memory` | Persistent store backend: `memory` or `s3` |
| `S3_BUCKET` | When `s3` | `""` | S3 bucket name |
| `S3_KEY_PREFIX` | No | `xp-tracker` | S3 key prefix for snapshot file |
| `S3_REGION` | No | `us-east-1` | AWS region for S3 client |
| `S3_ENDPOINT` | No | `""` | Custom S3 endpoint (MinIO, LocalStack) |

## GVR format

Each GVR must be specified in `group/version/resource` format. The resource name is the **plural lowercase** form (the same string you'd use with `kubectl get`).

```bash
CLAIM_GVRS="platform.example.org/v1alpha1/postgresqlinstances,platform.example.org/v1alpha1/kafkatopics"
XR_GVRS="platform.example.org/v1alpha1/xpostgresqlinstances,platform.example.org/v1alpha1/xkafkatopics"
```

!!! tip "Finding your GVRs"
    Use `kubectl api-resources` to find the correct group, version, and resource name for your Crossplane types:

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

## Deployment via ConfigMap

In the Kustomize manifests, environment variables are stored in a ConfigMap and injected via `envFrom`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: crossplane-metrics-exporter
data:
  CLAIM_GVRS: "platform.example.org/v1alpha1/postgresqlinstances"
  XR_GVRS: "platform.example.org/v1alpha1/xpostgresqlinstances"
  CREATOR_ANNOTATION_KEY: "platform.example.org/created-by"
  TEAM_ANNOTATION_KEY: "platform.example.org/team"
  POLL_INTERVAL_SECONDS: "30"
  METRICS_ADDR: ":8080"
```
