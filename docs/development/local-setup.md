# Local Development Setup

## Prerequisites

- Go 1.25+ (or use [mise](https://mise.jdx.dev/): `mise install`)
- Docker (for container builds)
- kubectl
- [kindplane](https://github.com/kanzifucius/kindplane) (recommended for local dev)
- [golangci-lint](https://golangci-lint.run/) for linting

## Cluster setup

=== "kindplane (Recommended)"

    [kindplane](https://github.com/kanzifucius/kindplane) bootstraps Kind clusters pre-configured with Crossplane and providers. This repo includes a `kindplane.yaml` that sets up:

    - Kind cluster named `xp-tracker-dev`
    - Crossplane 2.0 with `provider-kubernetes` and `provider-nop`
    - kube-prometheus-stack (Grafana on NodePort 30300)
    - Sample XRDs, Compositions, and Claims under `hack/samples/`

    ```bash
    # Install kindplane
    curl -fsSL https://raw.githubusercontent.com/kanzifucius/kindplane/main/install.sh | bash

    # Create the cluster (uses kindplane.yaml in repo root)
    kindplane up

    # Apply sample resources (8 claims across 3 namespaces, 5 creators, 3 teams)
    make samples-apply

    # Run the exporter against the sample resources
    make run-local

    # In another terminal -- check metrics
    curl -s localhost:8080/metrics | grep crossplane_

    # Check bookkeeping
    curl -s localhost:8080/bookkeeping | jq .

    # Clean up
    make samples-delete
    kindplane down
    ```

=== "Manual Kind Setup"

    If you prefer to set up the cluster manually without kindplane:

    ```bash
    # Create a cluster
    kind create cluster --name xp-dev

    # Install Crossplane
    helm repo add crossplane-stable https://charts.crossplane.io/stable
    helm install crossplane crossplane-stable/crossplane \
      --namespace crossplane-system --create-namespace

    # Install your XRDs and Compositions, then create some claims

    # Run the exporter locally
    export CLAIM_GVRS="your.org/v1alpha1/yourclaims"
    export XR_GVRS="your.org/v1alpha1/yourxrs"
    make run
    ```

## Build and run

```bash
# Build the binary
make build

# Run against a local cluster (uses KUBECONFIG or ~/.kube/config)
export CLAIM_GVRS="samples.xptracker.dev/v1alpha1/widgets,samples.xptracker.dev/v1alpha1/gadgets"
export XR_GVRS="samples.xptracker.dev/v1alpha1/xwidgets,samples.xptracker.dev/v1alpha1/xgadgets"
export CREATOR_ANNOTATION_KEY="xptracker.dev/created-by"
export TEAM_ANNOTATION_KEY="xptracker.dev/team"
export POLL_INTERVAL_SECONDS=10
./bin/xp-tracker
```

## Testing

| Command | Description |
|---|---|
| `make test` | Run tests with race detector |
| `make lint` | Run golangci-lint |
| `make vet` | Run `go vet` |
| `make check` | Run all checks (vet + lint + test) |

## Docker

```bash
# Build (defaults to ghcr.io/kanzifucius/xp-tracker:latest)
make docker-build

# Override image/tag
make docker-build IMAGE=myregistry.io/xp-tracker TAG=v0.1.0

# Push
make docker-push IMAGE=myregistry.io/xp-tracker TAG=v0.1.0
```

## Tool versions

The project uses [mise](https://mise.jdx.dev/) for tool version pinning via `.mise.toml`:

| Tool | Version |
|---|---|
| Go | 1.25.5 |
| golangci-lint | 1.64.8 |
| kubectl | 1.34.2 |

Run `mise install` to install the pinned versions.

## Makefile targets

Run `make help` to see all available targets. Key groups:

- **build** -- `build`, `clean`
- **test** -- `test`, `lint`, `vet`, `check`
- **docker** -- `docker-build`, `docker-push`
- **deploy** -- `deploy`, `undeploy`
- **dev** -- `dev`, `run`, `run-local`
- **samples** -- `samples-apply`, `samples-delete`
