# Helm Deployment

xp-tracker ships a thin Helm chart at `charts/xp-tracker/` that wraps the same
resources as [`deploy/base`](kustomize.md) (ServiceAccount, ClusterRole,
ClusterRoleBinding, ConfigMap, Deployment, and Service). It does not change
exporter behaviour; it only packages those manifests with values for image,
namespace, and configuration.

## Prerequisites

- [Helm](https://helm.sh/) 3.x
- A Kubernetes cluster with Crossplane installed
- Crossplane CRDs (XRDs) and Compositions deployed
- `kubectl` with access to the cluster

## Install from a local checkout

```bash
git clone https://github.com/kanzifucius/xp-tracker.git
cd xp-tracker

# Review rendered manifests (no cluster required)
helm template xp-tracker charts/xp-tracker \
  --namespace crossplane-system

# Install into crossplane-system (matches deploy/base default namespace)
helm upgrade --install xp-tracker charts/xp-tracker \
  --namespace crossplane-system \
  --create-namespace
```

Pin a specific published image tag (recommended for production):

```bash
helm upgrade --install xp-tracker charts/xp-tracker \
  --namespace crossplane-system \
  --create-namespace \
  --set image.tag=v0.4.0
```

## Image tags

The chart **refuses** an empty or `latest` tag. By default `image.tag` is empty
and falls back to `Chart.appVersion` (currently a pinned release such as
`v0.4.0`). Override with `--set image.tag=<release>` when you need another
published tag from [GHCR](https://github.com/kanzifucius/xp-tracker/pkgs/container/xp-tracker).

## Configuration

Exporter settings map to the ConfigMap keys used by `deploy/base`. Non-empty
entries under `config` are rendered as ConfigMap data:

```yaml
config:
  POLL_INTERVAL_SECONDS: "60"
  CREATOR_ANNOTATION_KEY: "platform.example.org/created-by"
  TEAM_ANNOTATION_KEY: "platform.example.org/team"
```

```bash
helm upgrade --install xp-tracker charts/xp-tracker \
  --namespace crossplane-system \
  --create-namespace \
  --set config.POLL_INTERVAL_SECONDS=60 \
  --set config.CREATOR_ANNOTATION_KEY=platform.example.org/created-by \
  --set config.TEAM_ANNOTATION_KEY=platform.example.org/team
```

See [environment variables](../configuration/environment-variables.md) for the
full list.

## RBAC

Defaults match `deploy/base`: broad read-only cluster access for a quick start.
For production, set scoped `rbac.rules` (see [RBAC](rbac.md)):

```yaml
rbac:
  create: true
  rules:
    - apiGroups: ["apiextensions.crossplane.io"]
      resources:
        - compositeresourcedefinitions
        - managedresourcedefinitions
      verbs: ["get", "list", "watch"]
    - apiGroups: ["platform.example.org"]
      resources:
        - postgresqlinstances
        - xpostgresqlinstances
      verbs: ["get", "list", "watch"]
```

## Single replica

!!! warning
    Keep `replicaCount: 1`. Multiple replicas each poll and serve metrics
    independently, which double-counts series.

## Relationship to Kustomize

| Path | Use when |
|---|---|
| `deploy/base` + overlays | You already manage manifests with Kustomize |
| `charts/xp-tracker` | You prefer Helm values / `helm upgrade` |

Both paths deploy the same exporter binary and the same resource kinds. Prefer
one or the other per environment to avoid duplicate Deployments.

## Uninstall

```bash
helm uninstall xp-tracker --namespace crossplane-system
```

Cluster-scoped RBAC objects created by the chart are removed with the release.
