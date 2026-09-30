#!/usr/bin/env bash
# Install Crossplane 2.0.x from the official stable Helm chart. No providers.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1091
set -a
. "${ROOT_DIR}/hack/e2e/versions.env"
set +a

if ! command -v helm >/dev/null 2>&1; then
  echo "helm is required" >&2
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

helm repo add crossplane-stable "${CROSSPLANE_CHART_REPO}" --force-update
helm repo update crossplane-stable

helm upgrade --install crossplane "crossplane-stable/${CROSSPLANE_CHART_NAME}" \
  --namespace "${CROSSPLANE_NAMESPACE}" \
  --create-namespace \
  --version "${CROSSPLANE_CHART_VERSION}" \
  --wait \
  --timeout 5m

kubectl -n "${CROSSPLANE_NAMESPACE}" wait --for=condition=Available deploy/crossplane --timeout=5m
kubectl -n "${CROSSPLANE_NAMESPACE}" wait --for=condition=Available deploy/crossplane-rbac-manager --timeout=5m
