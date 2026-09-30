#!/usr/bin/env bash
# Assert xp-tracker HTTP endpoints against a running exporter.
# Shared by CI (.github/workflows/e2e.yml) and `make e2e` / `make e2e-assert`.
set -euo pipefail

XP_TRACKER_URL="${XP_TRACKER_URL:-http://127.0.0.1:8080}"
TIMEOUT_SECONDS="${E2E_ASSERT_TIMEOUT_SECONDS:-90}"
METRICS_FILE="${E2E_METRICS_FILE:-}"

# Expected claim inventory from hack/samples/claims.yaml (4 Widget + 4 Gadget).
EXPECTED_CLAIMS=8
EXPECTED_WIDGETS=4
EXPECTED_GADGETS=4
# Expected v2 Apps from hack/samples/v2-namespaced.yaml (counted as XRs, not claims).
EXPECTED_APPS=2
EXPECTED_XWIDGETS=4
EXPECTED_XGADGETS=4

CLAIM_LABELS="group kind version namespace creator team claim_name synced ready reason paused deleting"
XR_LABELS="group kind version namespace name claim_name claim_namespace synced ready reason paused deleting"

die() {
  echo "assert-metrics: $*" >&2
  exit 1
}

http_code() {
  local path="$1"
  local code
  if ! code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "${XP_TRACKER_URL}${path}")"; then
    echo "000"
    return 0
  fi
  echo "${code}"
}

wait_http_ok() {
  local path="$1"
  local start now code
  start="$(date +%s)"
  while true; do
    code="$(http_code "${path}")"
    if [ "${code}" = "200" ]; then
      echo "${path} -> 200"
      return 0
    fi
    now="$(date +%s)"
    if [ $((now - start)) -ge "${TIMEOUT_SECONDS}" ]; then
      die "${path} did not return 200 within ${TIMEOUT_SECONDS}s (last status ${code})"
    fi
    sleep 2
  done
}

fetch_metrics() {
  if [ -n "${METRICS_FILE}" ]; then
    cat "${METRICS_FILE}"
    return 0
  fi
  curl -sS --max-time 10 "${XP_TRACKER_URL}/metrics"
}

label_values() {
  local metric="$1"
  local label="$2"
  local metrics="$3"
  printf '%s\n' "${metrics}" | awk -v metric="${metric}" -v label="${label}" '
    index($0, metric "{") == 1 {
      rest = substr($0, length(metric) + 2)
      n = split(rest, parts, ",")
      for (i = 1; i <= n; i++) {
        line = parts[i]
        sub(/}.*/, "", line)
        eq = index(line, "=")
        if (eq < 2) continue
        key = substr(line, 1, eq - 1)
        if (key != label) continue
        val = substr(line, eq + 1)
        gsub(/^"/, "", val)
        gsub(/"$/, "", val)
        print val
      }
    }
  ' | sort -u
}

sum_metric() {
  local metric="$1"
  local metrics="$2"
  printf '%s\n' "${metrics}" | awk -v metric="${metric}" '
    index($0, metric "{") == 1 { sum += $NF }
    END { print sum + 0 }
  '
}

sum_metric_kind() {
  local metric="$1"
  local kind="$2"
  local metrics="$3"
  printf '%s\n' "${metrics}" | awk -v metric="${metric}" -v kind="${kind}" '
    index($0, metric "{") == 1 && $0 ~ "(^|,)kind=\"" kind "\"" { sum += $NF }
    END { print sum + 0 }
  '
}

first_series() {
  local metric="$1"
  local metrics="$2"
  printf '%s\n' "${metrics}" | awk -v metric="${metric}" 'index($0, metric "{") == 1 { print; exit }'
}

label_keys_from_series() {
  local series="$1"
  printf '%s\n' "${series}" | awk '
    {
      start = index($0, "{")
      end = index($0, "}")
      if (start == 0 || end <= start) next
      body = substr($0, start + 1, end - start - 1)
      n = split(body, parts, ",")
      for (i = 1; i <= n; i++) {
        eq = index(parts[i], "=")
        if (eq < 2) continue
        print substr(parts[i], 1, eq - 1)
      }
    }
  '
}

require_labels() {
  local metric="$1"
  local expected="$2"
  local metrics="$3"
  local series keys missing label
  series="$(first_series "${metric}" "${metrics}")"
  [ -n "${series}" ] || { echo "no ${metric} series found"; return 1; }
  keys="$(label_keys_from_series "${series}")"
  missing=""
  for label in ${expected}; do
    if ! printf '%s\n' "${keys}" | grep -qx "${label}"; then
      missing="${missing} ${label}"
    fi
  done
  if [ -n "${missing}" ]; then
    echo "${metric} missing label keys:${missing} (series: ${series})"
    return 1
  fi
  echo "${metric} label keys include: ${expected}"
}

require_values() {
  local metric="$1"
  local label="$2"
  local metrics="$3"
  shift 3
  local values want
  values="$(label_values "${metric}" "${label}" "${metrics}")"
  for want in "$@"; do
    if ! printf '%s\n' "${values}" | grep -qx "${want}"; then
      echo "${metric} missing ${label}=${want} (have: $(printf '%s ' ${values}))"
      return 1
    fi
  done
}

assert_inventory() {
  local metrics="$1"
  printf '%s\n' "${metrics}" | grep -q '^crossplane_claims_total{' || { echo "metrics missing crossplane_claims_total"; return 1; }
  printf '%s\n' "${metrics}" | grep -q '^crossplane_xr_total{' || { echo "metrics missing crossplane_xr_total"; return 1; }

  local claims widgets gadgets apps xwidgets xgadgets
  claims="$(sum_metric crossplane_claims_total "${metrics}")"
  widgets="$(sum_metric_kind crossplane_claims_total Widget "${metrics}")"
  gadgets="$(sum_metric_kind crossplane_claims_total Gadget "${metrics}")"
  apps="$(sum_metric_kind crossplane_xr_total App "${metrics}")"
  xwidgets="$(sum_metric_kind crossplane_xr_total XWidget "${metrics}")"
  xgadgets="$(sum_metric_kind crossplane_xr_total XGadget "${metrics}")"

  [ "${claims}" -eq "${EXPECTED_CLAIMS}" ] || { echo "expected ${EXPECTED_CLAIMS} claims, got ${claims}"; return 1; }
  [ "${widgets}" -eq "${EXPECTED_WIDGETS}" ] || { echo "expected ${EXPECTED_WIDGETS} Widget claims, got ${widgets}"; return 1; }
  [ "${gadgets}" -eq "${EXPECTED_GADGETS}" ] || { echo "expected ${EXPECTED_GADGETS} Gadget claims, got ${gadgets}"; return 1; }
  [ "${apps}" -eq "${EXPECTED_APPS}" ] || { echo "expected ${EXPECTED_APPS} v2 App XRs, got ${apps}"; return 1; }
  [ "${xwidgets}" -eq "${EXPECTED_XWIDGETS}" ] || { echo "expected ${EXPECTED_XWIDGETS} XWidget XRs, got ${xwidgets}"; return 1; }
  [ "${xgadgets}" -eq "${EXPECTED_XGADGETS}" ] || { echo "expected ${EXPECTED_XGADGETS} XGadget XRs, got ${xgadgets}"; return 1; }

  require_labels crossplane_claims_total "${CLAIM_LABELS}" "${metrics}"
  require_labels crossplane_xr_total "${XR_LABELS}" "${metrics}"
  require_values crossplane_claims_total namespace "${metrics}" team-alpha team-beta team-gamma
  require_values crossplane_claims_total kind "${metrics}" Widget Gadget
  require_values crossplane_xr_total kind "${metrics}" App XWidget XGadget
}

if [ -z "${METRICS_FILE}" ]; then
  wait_http_ok /healthz
  wait_http_ok /readyz
fi

echo "Waiting for sample inventory on /metrics..."
start="$(date +%s)"
last_err="metrics not fetched"
while true; do
  metrics="$(fetch_metrics)" || die "failed to fetch /metrics"
  if last_err="$(assert_inventory "${metrics}" 2>&1)"; then
    printf '%s\n' "${last_err}"
    break
  fi
  now="$(date +%s)"
  if [ $((now - start)) -ge "${TIMEOUT_SECONDS}" ]; then
    printf '%s\n' "${metrics}" | grep -E '^crossplane_(claims|xr)_total' || true
    die "inventory assertions did not pass within ${TIMEOUT_SECONDS}s: ${last_err}"
  fi
  sleep 3
done

if [ -z "${METRICS_FILE}" ]; then
  bookkeeping_code="$(http_code /bookkeeping)"
  if [ "${bookkeeping_code}" != "404" ]; then
    die "expected /bookkeeping 404 (JSON API must not be served), got ${bookkeeping_code}"
  fi
  echo "/bookkeeping -> 404"
fi

echo "xp-tracker e2e assertions passed against ${XP_TRACKER_URL}"
