# Bookkeeping Endpoint

In addition to Prometheus metrics, xp-tracker exposes a JSON endpoint that returns the full in-memory snapshot of claims, XRs, and MRs. This is useful for ad-hoc debugging, CLI tools, or external integrations that don't want to go through PromQL.

## Endpoint

```
GET /bookkeeping
```

Returns `Content-Type: application/json; charset=utf-8` with HTTP 200.

## Response format

```json
{
  "claims": [
    {
      "group": "platform.example.org",
      "version": "v1alpha1",
      "kind": "PostgreSQLInstance",
      "namespace": "team-a",
      "name": "db-123",
      "creator": "alice@example.com",
      "team": "payments",
      "composition": "postgres-small",
      "paused": false,
      "deleting": false,
      "ready": true,
      "reason": "Ready",
      "ageSeconds": 12345
    }
  ],
  "xrs": [
    {
      "group": "platform.example.org",
      "version": "v1alpha1",
      "kind": "XPostgreSQLInstance",
      "namespace": "",
      "name": "db-123-xyz",
      "composition": "postgres-small",
      "paused": false,
      "deleting": false,
      "ready": true,
      "reason": "Ready",
      "ageSeconds": 12300
    }
  ],
  "mrs": [
    {
      "group": "nop.crossplane.io",
      "version": "v1alpha1",
      "kind": "NopResource",
      "namespace": "default",
      "name": "nop-abc",
      "xrName": "xwidget-a",
      "claimName": "widget-a",
      "claimNamespace": "team-alpha",
      "provider": "provider-nop",
      "providerConfig": "default",
      "externalName": "cloud-nop-abc",
      "managementPolicies": "*",
      "paused": false,
      "deleting": false,
      "ready": true,
      "reason": "Available",
      "ageSeconds": 1200
    }
  ],
  "generatedAt": "2026-02-13T20:50:00Z"
}
```

## Fields

### Claim fields

| Field | Type | Description |
|---|---|---|
| `group` | string | API group from the GVR |
| `version` | string | API version from the GVR |
| `kind` | string | Resource kind |
| `namespace` | string | Kubernetes namespace |
| `name` | string | Resource name |
| `creator` | string | Value of the creator annotation (empty if not set) |
| `team` | string | Value of the team annotation (empty if not set) |
| `composition` | string | Composition name (enriched from backing XR) |
| `paused` | boolean | Whether the `crossplane.io/paused` annotation is set |
| `deleting` | boolean | Whether `metadata.deletionTimestamp` is set |
| `ready` | boolean | Whether the Ready condition is True |
| `reason` | string | Ready condition reason |
| `ageSeconds` | integer | Seconds since `metadata.creationTimestamp` |

### XR fields

| Field | Type | Description |
|---|---|---|
| `group` | string | API group from the GVR |
| `version` | string | API version from the GVR |
| `kind` | string | Resource kind |
| `namespace` | string | Namespace (usually empty for cluster-scoped XRs) |
| `name` | string | Resource name |
| `composition` | string | Composition name (from label) |
| `paused` | boolean | Whether the `crossplane.io/paused` annotation is set |
| `deleting` | boolean | Whether `metadata.deletionTimestamp` is set |
| `ready` | boolean | Whether the Ready condition is True |
| `reason` | string | Ready condition reason |
| `ageSeconds` | integer | Seconds since `metadata.creationTimestamp` |

### MR fields

| Field | Type | Description |
|---|---|---|
| `group` | string | API group from the GVR |
| `version` | string | API version from the GVR |
| `kind` | string | Resource kind |
| `namespace` | string | Kubernetes namespace |
| `name` | string | Resource name |
| `xrName` | string | Composite (XR) name from the composite label |
| `claimName` | string | Claim name (from MR labels or XR enrichment) |
| `claimNamespace` | string | Claim namespace |
| `provider` | string | Provider package name from MRD discovery |
| `providerConfig` | string | `spec.providerConfigRef.name` |
| `externalName` | string | Cloud resource identifier from `crossplane.io/external-name` |
| `managementPolicies` | string | Joined `spec.managementPolicies` |
| `paused` | boolean | Whether the `crossplane.io/paused` annotation is set |
| `deleting` | boolean | Whether `metadata.deletionTimestamp` is set |
| `ready` | boolean | Whether the Ready condition is True |
| `reason` | string | Ready condition reason |
| `ageSeconds` | integer | Seconds since `metadata.creationTimestamp` |

### Top-level fields

| Field | Type | Description |
|---|---|---|
| `generatedAt` | string | ISO 8601 / RFC 3339 UTC timestamp of when the response was generated |

## Usage examples

```bash
# Full snapshot
curl -s localhost:8080/bookkeeping | jq .

# Count claims by namespace
curl -s localhost:8080/bookkeeping | jq '[.claims[] | .namespace] | group_by(.) | map({(.[0]): length}) | add'

# List not-ready claims
curl -s localhost:8080/bookkeeping | jq '[.claims[] | select(.ready == false)]'

# Get all XR compositions
curl -s localhost:8080/bookkeeping | jq '[.xrs[].composition] | unique'

# List paused or deleting MRs
curl -s localhost:8080/bookkeeping | jq '[.mrs[] | select(.paused == true or .deleting == true)]'

# Find MRs for a specific claim
curl -s localhost:8080/bookkeeping | jq '[.mrs[] | select(.claimName == "widget-a")]'
```
