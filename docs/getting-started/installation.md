# Installation

## Container image

xp-tracker is distributed as a multi-arch container image on GitHub Container Registry:

```bash
docker pull ghcr.io/kanzifucius/xp-tracker:latest
```

Supported architectures: `linux/amd64`, `linux/arm64`.

## Deploy or build

=== "Kustomize Base"

    The recommended way to deploy xp-tracker. The base deploys to the `crossplane-system` namespace with placeholder GVRs that you must override:

    ```bash
    # Review what will be applied
    kubectl kustomize deploy/base

    # Apply
    kubectl apply -k deploy/base
    ```

=== "Kustomize Overlay"

    The example overlay demonstrates how to customise GVRs, add a ServiceMonitor, and pin the image tag:

    ```bash
    kubectl apply -k deploy/overlays/example
    ```

    See [Kustomize deployment](../deployment/kustomize.md) for details on creating your own overlay.

=== "Build from Source"

    Requires Go 1.25+:

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
