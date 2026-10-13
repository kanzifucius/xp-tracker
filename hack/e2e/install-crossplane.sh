#!/usr/bin/env bash
# Install Crossplane 2.0.x from the official stable Helm chart, then provider-nop
# (or a sample Active MRD fallback) so e2e can assert XR-linked MR metrics.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1091
set -a
. "${ROOT_DIR}/hack/e2e/versions.env"
set +a

PROVIDER_WAIT_SECONDS="${E2E_PROVIDER_WAIT_SECONDS:-180}"
SAMPLES="${ROOT_DIR}/hack/samples"

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

# Pin the Provider package from versions.env (sample YAML documents the default).
sed \
  -e "s|name: provider-nop|name: ${PROVIDER_NOP_NAME}|" \
  -e "s|package: .*|package: ${PROVIDER_NOP_PACKAGE}|" \
  "${SAMPLES}/provider-nop.yaml" | kubectl apply -f -

echo "Waiting for provider-nop Healthy (non-fatal if the package pull times out)..."
if kubectl wait --for=condition=Healthy "provider.pkg.crossplane.io/${PROVIDER_NOP_NAME}" --timeout=90s; then
  echo "provider-nop is Healthy"
else
  echo "provider-nop did not become Healthy (image pull from xpkg.upbound.io may have timed out); continuing"
fi

mrd_active() {
  local state
  state="$(kubectl get mrd "${PROVIDER_NOP_MRD}" -o jsonpath='{.spec.state}' 2>/dev/null || true)"
  [ "${state}" = "Active" ]
}

crd_established() {
  kubectl get crd "${PROVIDER_NOP_CRD}" >/dev/null 2>&1
}

# Apply hack/samples/mrd-clusternop-fallback.yaml idempotently. provider-nop may have
# partially established the CRD (and sometimes a non-Active MRD); applying the full
# multi-doc file with client-side apply then fails with AlreadyExists on the CRD.
apply_clusternop_fallback() {
  local fallback="${SAMPLES}/mrd-clusternop-fallback.yaml"
  local field_manager=xp-tracker-e2e
  echo "Applying sample Active MRD fallback (idempotent)..."

  # First document: ManagedResourceDefinition (server-side apply is idempotent).
  awk 'BEGIN{p=1} /^---[[:space:]]*$/{exit} {print}' "${fallback}" | \
    kubectl apply --server-side --force-conflicts --field-manager="${field_manager}" -f -
  if ! mrd_active; then
    kubectl patch mrd "${PROVIDER_NOP_MRD}" --type=merge \
      --field-manager="${field_manager}" \
      -p '{"spec":{"state":"Active"}}'
  fi

  # Second document: CRD — skip when provider-nop already established it.
  if crd_established; then
    echo "CRD ${PROVIDER_NOP_CRD} already present; skipping fallback CRD apply"
  else
    awk 'BEGIN{p=0} /^---[[:space:]]*$/{p=1; next} p{print}' "${fallback}" | \
      kubectl apply --server-side --force-conflicts --field-manager="${field_manager}" -f -
  fi
}

echo "Waiting up to ${PROVIDER_WAIT_SECONDS}s for Active MRD ${PROVIDER_NOP_MRD} and CRD..."
start="$(date +%s)"
while true; do
  if mrd_active && crd_established; then
    echo "Active MRD ${PROVIDER_NOP_MRD} and CRD are present"
    break
  fi
  if [ $(($(date +%s) - start)) -ge "${PROVIDER_WAIT_SECONDS}" ]; then
    echo "provider-nop MRD/CRD not ready within ${PROVIDER_WAIT_SECONDS}s; applying sample Active MRD fallback"
    apply_clusternop_fallback
    # Crossplane may own the CRD after Active MRD reconcile; wait either way.
    kubectl wait --for=condition=Established "crd/${PROVIDER_NOP_CRD}" --timeout=120s
    if ! mrd_active; then
      echo "fallback MRD ${PROVIDER_NOP_MRD} is not Active" >&2
      kubectl get mrd "${PROVIDER_NOP_MRD}" -o yaml || true
      exit 1
    fi
    echo "Fallback Active MRD ${PROVIDER_NOP_MRD} is ready"
    break
  fi
  sleep 3
done
