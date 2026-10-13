# Installation

## Container image

xp-tracker is distributed as a multi-arch container image on GitHub Container Registry:

```bash
docker pull ghcr.io/kanzifucius/xp-tracker:latest
```

Supported architectures: `linux/amd64`, `linux/arm64`.

## Deploy or build

=== "Kustomize Base"

    The recommended way to deploy xp-tracker with plain manifests. The base deploys to the `crossplane-system` namespace and discovers claim/XR GVRs from XRDs at startup:

    ```bash
    # Review what will be applied
    kubectl kustomize deploy/base

    # Apply
    kubectl apply -k deploy/base
    ```

=== "Helm"

    Thin chart wrapping the same resources as `deploy/base`. Image tags are pinned (defaults to `Chart.appVersion`; `:latest` is rejected):

    ```bash
    helm upgrade --install xp-tracker charts/xp-tracker \
      --namespace crossplane-system \
      --create-namespace \
      --set image.tag=v0.4.0
    ```

    See [Helm deployment](../deployment/helm.md) for values, RBAC, and configuration.

=== "Kustomize Overlay"

    The example overlay demonstrates how to customise annotation keys, add a ServiceMonitor, and pin the image tag:

    ```bash
    kubectl apply -k deploy/overlays/example
    ```

    See [Kustomize deployment](../deployment/kustomize.md) for details on creating your own overlay.

=== "Build from Source"

    Requires Go 1.26+:

    ```bash
    git clone https://github.com/kanzifucius/xp-tracker.git
    cd xp-tracker
    make build
    ```

    The binary is output to `bin/xp-tracker`.

## Prerequisites

- A Kubernetes cluster with [Crossplane](https://www.crossplane.io/) installed
- Crossplane CRDs (XRDs) and Compositions deployed
- `kubectl` with access to the cluster
