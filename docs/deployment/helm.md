# Helm Deployment

xp-tracker ships a thin Helm chart at `charts/xp-tracker/` that wraps the same
resources as [`deploy/base`](kustomize.md) (ServiceAccount, ClusterRole,
ClusterRoleBinding, ConfigMap, Deployment, and Service). It does not change
exporter behaviour; it only packages those manifests with values for image,
namespace, and configuration.

Published chart releases are available as OCI artefacts on GHCR:

`oci://ghcr.io/kanzifucius/charts/xp-tracker`

Each git tag `v*` publishes chart version and `appVersion` equal to the tag
without the leading `v` (for example `v0.5.0-rc.1` → chart `0.5.0-rc.1`).

## Prerequisites

- [Helm](https://helm.sh/) 3.8+ (OCI support)
- A Kubernetes cluster with Crossplane installed
- Crossplane CRDs (XRDs) and Compositions deployed
- `kubectl` with access to the cluster

## Install from OCI (recommended)

```bash
helm upgrade --install xp-tracker oci://ghcr.io/kanzifucius/charts/xp-tracker \
  --version 0.4.0 \
  --namespace crossplane-system \
  --create-namespace
```

RC example (after a pre-release tag has published):

```bash
helm upgrade --install xp-tracker oci://ghcr.io/kanzifucius/charts/xp-tracker \
  --version 0.5.0-rc.1 \
  --namespace crossplane-system \
  --create-namespace
```

Inspect without installing:

```bash
helm show chart oci://ghcr.io/kanzifucius/charts/xp-tracker --version 0.4.0
helm pull oci://ghcr.io/kanzifucius/charts/xp-tracker --version 0.4.0
```

There is no `helm repo add` for OCI — use the full `oci://` URL. Chart tarballs
are also attached to [GitHub Releases](https://github.com/kanzifucius/xp-tracker/releases)
for offline installs.

If the GHCR package is private, log in first:

```bash
helm registry login ghcr.io
```

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

Pin a specific published image tag (recommended for production when not using
the matching OCI chart version):

```bash
helm upgrade --install xp-tracker charts/xp-tracker \
  --namespace crossplane-system \
  --create-namespace \
  --set image.tag=0.4.0
```

## Image tags

The chart **refuses** an empty or `latest` tag. By default `image.tag` is empty
and falls back to `Chart.appVersion` (a pinned GHCR tag such as `0.4.0`, without
a leading `v`). Override with `--set image.tag=<release>` when you need another
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
helm upgrade --install xp-tracker oci://ghcr.io/kanzifucius/charts/xp-tracker \
  --version 0.4.0 \
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
| `charts/xp-tracker` or OCI chart | You prefer Helm values / `helm upgrade` |

Both paths deploy the same exporter binary and the same resource kinds. Prefer
one or the other per environment to avoid duplicate Deployments.

## Uninstall

```bash
helm uninstall xp-tracker --namespace crossplane-system
```

Cluster-scoped RBAC objects created by the chart are removed with the release.
