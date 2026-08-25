# AGENTS.md

## Cursor Cloud specific instructions

This repository is a **single Go product**: `xp-tracker`, a read-only Prometheus
exporter for Crossplane that exposes business-level inventory metrics (resource
counts by `creator`, `team`, `namespace`, `composition`). The entrypoint is
`cmd/exporter`; the binary serves `:8080` with `/metrics`, `/bookkeeping`,
`/healthz`, and `/readyz`.

### Toolchain

- **Go** is provided by the base image and the pinned toolchain (`go 1.25.5`,
  see `go.mod` / `.mise.toml`) is fetched automatically on first `go` invocation.
- **golangci-lint 2.9.0** is installed to `/usr/local/bin` by the environment
  update script, so `make lint` works with no PATH changes. `mise` itself is
  **not** installed; the versions it pins are provided directly instead.

### Standard dev commands (no cluster required)

These are the CI-equivalent checks and use fakes — **no Kubernetes cluster is
needed**. See the `Makefile` for the full list.

- `make build` — build `bin/xp-tracker`
- `make vet` — `go vet ./...`
- `make lint` — `golangci-lint run ./...`
- `make test` — `go test -race -count=1 ./...`
- `make check` / `make ci` — run the above together

### Running the exporter end-to-end (requires a Crossplane cluster)

The binary **exits at startup** with `no XR GVRs discovered from XRDs` unless it
can reach a cluster that has Crossplane XRDs installed. The `README`/`Makefile`
`make dev` path uses the external `kindplane` CLI, which is **not installed
here**. Reproduce an equivalent local cluster manually instead:

1. Docker is not running by default. Docker 29 in this VM needs
   `fuse-overlayfs`, `iptables-legacy`, and the `containerd-snapshotter` feature
   **disabled** (`/etc/docker/daemon.json`), then start `dockerd` in the
   background. `kind`/`kubectl`/`helm` must also be installed. These are heavy
   E2E-only dependencies and are deliberately **not** in the update script.
2. `kind` runs its containers via Docker as **root**, so create the cluster with
   `sudo kind create cluster`, then export a readable kubeconfig:
   `sudo kind get kubeconfig --name <cluster> > /tmp/kubeconfig` and
   `export KUBECONFIG=/tmp/kubeconfig`.
3. Install **Crossplane v2** (chart `crossplane-stable/crossplane`, e.g.
   `2.0.x`). v2 is required: the exporter discovers both `apiextensions.crossplane.io`
   `v1` **and** `v2` CompositeResourceDefinitions plus `v1alpha1`
   ManagedResourceDefinitions, and those CRDs only exist on Crossplane 2.x.
4. Apply the samples in `hack/samples/`. `make samples-apply` waits for
   `function-patch-and-transform` to become `Healthy`, which pulls an image from
   `xpkg.upbound.io` and **may time out** — that is not fatal for the exporter.
   The XRDs still become `Established` and claims/XRs are counted regardless of
   composition readiness, so you can apply `hack/samples/claims.yaml` directly.
5. Run the exporter against the cluster with annotation keys matching the
   samples:
   `KUBECONFIG=/tmp/kubeconfig CREATOR_ANNOTATION_KEY=xptracker.dev/created-by TEAM_ANNOTATION_KEY=xptracker.dev/team POLL_INTERVAL_SECONDS=10 ./bin/xp-tracker`
   Then `curl localhost:8080/metrics` (series prefixed `crossplane_` and
   `xp_tracker_store_`) and `curl localhost:8080/bookkeeping` (JSON snapshot).

GVRs are auto-discovered; `CLAIM_GVRS`/`XR_GVRS` env vars are deprecated static
overrides. All configuration is via environment variables — see
`deploy/base/configmap.yaml` and the `README` for the full list.
