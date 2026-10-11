#!/usr/bin/env bash
# Apply hack/samples in CI/local e2e order. Function Healthy is non-fatal.
# After claim/XR inventory exists, ensure at least one XR-linked ClusterNopResource
# so crossplane_mr_total assertions do not depend on composition pipeline readiness.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SAMPLES="${ROOT_DIR}/hack/samples"
WAIT_TIMEOUT="${E2E_RESOURCE_WAIT_SECONDS:-180}"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

count_resources() {
  local resource="$1"
  shift
  kubectl get "${resource}" "$@" --no-headers 2>/dev/null | wc -l | tr -d ' '
}

wait_established() {
  local name="$1"
  echo "Waiting for XRD ${name} to become Established..."
  kubectl wait --for=condition=Established "xrd/${name}" --timeout=120s
  kubectl wait --for=condition=Established "crd/${name}" --timeout=120s
}

wait_count() {
  local resource="$1"
  local expected="$2"
  shift 2
  local got=0
  local start
  start="$(date +%s)"
  while true; do
    got="$(count_resources "${resource}" "$@")"
    if [ "${got}" -ge "${expected}" ]; then
      echo "${resource}: ${got} (need >= ${expected})"
      return 0
    fi
    if [ $(($(date +%s) - start)) -ge "${WAIT_TIMEOUT}" ]; then
      echo "timed out waiting for ${expected} ${resource} (got ${got})" >&2
      kubectl get "${resource}" "$@" || true
      return 1
    fi
    sleep 3
  done
}

# Apply at least one ClusterNopResource labelled with crossplane.io/composite so the
# exporter polls it. Does not require function Healthy or provider reconcile.
apply_xr_linked_mrs() {
  local xr_name
  xr_name="$(kubectl get xwidgets.samples.xptracker.dev -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -z "${xr_name}" ]; then
    echo "no XWidget XR found to link sample MRs" >&2
    return 1
  fi

  echo "Applying XR-linked ClusterNopResource for XR ${xr_name}..."
  cat <<EOF | kubectl apply -f -
apiVersion: nop.crossplane.io/v1alpha1
kind: ClusterNopResource
metadata:
  name: e2e-xr-linked-nop-1
  labels:
    crossplane.io/composite: ${xr_name}
EOF

  # Optional second MR for a different XR when present (still non-fatal if only one XR).
  local xr_name_2
  xr_name_2="$(kubectl get xwidgets.samples.xptracker.dev -o jsonpath='{.items[1].metadata.name}' 2>/dev/null || true)"
  if [ -n "${xr_name_2}" ]; then
    cat <<EOF | kubectl apply -f -
apiVersion: nop.crossplane.io/v1alpha1
kind: ClusterNopResource
metadata:
  name: e2e-xr-linked-nop-2
  labels:
    crossplane.io/composite: ${xr_name_2}
EOF
  fi

  wait_count clusternopresources.nop.crossplane.io 1
  echo "XR-linked ClusterNopResource inventory is present (linked to ${xr_name})."
}

kubectl apply -f "${SAMPLES}/namespaces.yaml"
kubectl apply -f "${SAMPLES}/functions.yaml"
kubectl apply -f "${SAMPLES}/xrds.yaml"
wait_established xwidgets.samples.xptracker.dev
wait_established xgadgets.samples.xptracker.dev

echo "Waiting for function-patch-and-transform to become Healthy (non-fatal if this times out)..."
if kubectl wait --for=condition=Healthy function/function-patch-and-transform --timeout=90s; then
  echo "function-patch-and-transform is Healthy"
else
  echo "function-patch-and-transform did not become Healthy (image pull from xpkg.upbound.io may have timed out); continuing"
fi

kubectl apply -f "${SAMPLES}/compositions.yaml"
kubectl apply -f "${SAMPLES}/claims.yaml"

# v2-namespaced.yaml includes the XRD, composition, and Apps. Apps can fail
# until the XRD/CRD is Established, so apply, wait, then re-apply.
if ! kubectl apply -f "${SAMPLES}/v2-namespaced.yaml"; then
  echo "first apply of v2-namespaced.yaml returned non-zero; waiting for XRD then retrying"
fi
wait_established apps.samples.xptracker.dev
kubectl apply -f "${SAMPLES}/v2-namespaced.yaml"

echo "Waiting for sample claims, XRs, and v2 Apps to appear..."
wait_count widgets.samples.xptracker.dev 4 -A
wait_count gadgets.samples.xptracker.dev 4 -A
wait_count xwidgets.samples.xptracker.dev 4
wait_count xgadgets.samples.xptracker.dev 4
wait_count apps.samples.xptracker.dev 2 -A

apply_xr_linked_mrs

echo "Sample inventory is present (8 claims, 8 claim XRs, 2 v2 Apps, >=1 XR-linked MR)."
