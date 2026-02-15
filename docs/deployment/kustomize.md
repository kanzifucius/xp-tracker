# Kustomize Deployment

xp-tracker ships with [Kustomize](https://kustomize.io/) manifests organised as a base plus overlays.

## Base

The base at `deploy/base/` includes:

| Manifest | Description |
|---|---|
| `serviceaccount.yaml` | ServiceAccount for the exporter pod |
| `clusterrole.yaml` | ClusterRole with read-only access (see [RBAC](rbac.md)) |
| `clusterrolebinding.yaml` | Binds the ClusterRole to the ServiceAccount |
| `configmap.yaml` | Environment variables for the exporter |
| `deployment.yaml` | Single-replica Deployment |
| `service.yaml` | Service exposing port 8080 (`metrics`) |

The base deploys to the `crossplane-system` namespace and includes placeholder GVRs that must be overridden.

```bash
# Review rendered manifests
kubectl kustomize deploy/base

# Apply
kubectl apply -k deploy/base
```

## Example overlay

The example overlay at `deploy/overlays/example/` demonstrates:

- Patching the ConfigMap with real GVRs and annotation keys
- Adding a `ServiceMonitor` for Prometheus Operator
- Pinning the container image tag

```bash
kubectl apply -k deploy/overlays/example
```

## Creating your own overlay

Create a directory for your environment:

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
    newTag: v0.1.0
```

## Namespace

The base sets `namespace: crossplane-system`. To deploy to a different namespace, add a `namespace` field in your overlay's `kustomization.yaml`:

```yaml
namespace: monitoring
```

## Image override

Use the Kustomize `images` transformer to pin a specific tag:

```yaml
images:
  - name: ghcr.io/kanzifucius/xp-tracker
    newTag: v0.2.0
```

## Single replica requirement

!!! warning
    Running more than one replica will result in double-counted metrics since each replica independently polls and serves metrics. Always keep `replicas: 1`.
