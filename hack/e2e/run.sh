#!/usr/bin/env bash
# Local/CI driver for the Crossplane kind e2e job.
# Usage: hack/e2e/run.sh [--skip-build] [--skip-cluster] [--keep-cluster]
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1091
set -a
. "${ROOT_DIR}/hack/e2e/versions.env"
set +a

SKIP_BUILD=0
SKIP_CLUSTER=0
KEEP_CLUSTER=0
for arg in "$@"; do
  case "${arg}" in
    --skip-build) SKIP_BUILD=1 ;;
    --skip-cluster) SKIP_CLUSTER=1 ;;
    --keep-cluster) KEEP_CLUSTER=1 ;;
    *)
      echo "unknown argument: ${arg}" >&2
      echo "usage: $0 [--skip-build] [--skip-cluster] [--keep-cluster]" >&2
      exit 2
      ;;
  esac
done

E2E_LOG_DIR="${E2E_LOG_DIR:-${ROOT_DIR}/.e2e-logs}"
mkdir -p "${E2E_LOG_DIR}"
EXPORTER_LOG="${E2E_LOG_DIR}/xp-tracker.log"
EXPORTER_PID=""
CLUSTER_CREATED=0

cleanup() {
  if [ -n "${EXPORTER_PID}" ]; then
    kill "${EXPORTER_PID}" 2>/dev/null || true
    wait "${EXPORTER_PID}" 2>/dev/null || true
  fi
  if [ "${CLUSTER_CREATED}" -eq 1 ] && [ "${KEEP_CLUSTER}" -eq 0 ]; then
    kind delete cluster --name "${KIND_CLUSTER_NAME}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required for e2e" >&2
    exit 1
  fi
}

if [ "${SKIP_BUILD}" -eq 0 ]; then
  require_cmd go
  echo "Building bin/xp-tracker..."
  (cd "${ROOT_DIR}" && go build -o bin/xp-tracker ./cmd/exporter)
fi

if [ ! -x "${ROOT_DIR}/bin/xp-tracker" ] && [ ! -f "${ROOT_DIR}/bin/xp-tracker" ]; then
  echo "bin/xp-tracker is missing; run make build or omit --skip-build" >&2
  exit 1
fi

if [ "${SKIP_CLUSTER}" -eq 0 ]; then
  require_cmd docker
  require_cmd kind
  require_cmd helm
  require_cmd kubectl
  if kind get clusters 2>/dev/null | grep -qx "${KIND_CLUSTER_NAME}"; then
    echo "Deleting existing kind cluster ${KIND_CLUSTER_NAME}..."
    kind delete cluster --name "${KIND_CLUSTER_NAME}"
  fi
  echo "Creating one-node kind cluster ${KIND_CLUSTER_NAME} (${KIND_NODE_IMAGE})..."
  kind create cluster \
    --name "${KIND_CLUSTER_NAME}" \
    --image "${KIND_NODE_IMAGE}" \
    --config "${ROOT_DIR}/hack/e2e/kind.yaml" \
    --wait 120s
  CLUSTER_CREATED=1
else
  require_cmd helm
  require_cmd kubectl
fi

"${ROOT_DIR}/hack/e2e/install-crossplane.sh"
"${ROOT_DIR}/hack/e2e/apply-samples.sh"

echo "Starting xp-tracker..."
: >"${EXPORTER_LOG}"
(
  cd "${ROOT_DIR}"
  CREATOR_ANNOTATION_KEY=xptracker.dev/created-by \
    TEAM_ANNOTATION_KEY=xptracker.dev/team \
    POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-5}" \
    ./bin/xp-tracker
) >"${EXPORTER_LOG}" 2>&1 &
EXPORTER_PID=$!

sleep 2
if ! kill -0 "${EXPORTER_PID}" 2>/dev/null; then
  echo "xp-tracker exited during startup; log follows:" >&2
  cat "${EXPORTER_LOG}" >&2
  exit 1
fi

XP_TRACKER_URL="${XP_TRACKER_URL:-http://127.0.0.1:8080}" \
  "${ROOT_DIR}/hack/e2e/assert-metrics.sh"
