# Bookkeeping Endpoint

In addition to Prometheus metrics, xp-tracker exposes a JSON endpoint that returns the full in-memory snapshot of claims and XRs. This is useful for ad-hoc debugging, CLI tools, or external integrations that don't want to go through PromQL.

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
      "kind": "PostgreSQLInstance",
      "namespace": "team-a",
      "name": "db-123",
      "creator": "alice@example.com",
      "team": "payments",
      "composition": "postgres-small",
      "ready": true,
      "reason": "Ready",
      "ageSeconds": 12345
    }
  ],
  "xrs": [
    {
      "group": "platform.example.org",
      "kind": "XPostgreSQLInstance",
      "namespace": "",
      "name": "db-123-xyz",
      "composition": "postgres-small",
      "ready": true,
      "reason": "Ready",
      "ageSeconds": 12300
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
| `kind` | string | Resource kind |
| `namespace` | string | Kubernetes namespace |
| `name` | string | Resource name |
| `creator` | string | Value of the creator annotation (empty if not set) |
| `team` | string | Value of the team annotation (empty if not set) |
| `composition` | string | Composition name (enriched from backing XR) |
| `ready` | boolean | Whether the Ready condition is True |
| `reason` | string | Ready condition reason |
| `ageSeconds` | integer | Seconds since `metadata.creationTimestamp` |

### XR fields

| Field | Type | Description |
|---|---|---|
| `group` | string | API group from the GVR |
| `kind` | string | Resource kind |
| `namespace` | string | Namespace (usually empty for cluster-scoped XRs) |
| `name` | string | Resource name |
| `composition` | string | Composition name (from label) |
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
```

## Notes

- The endpoint reflects the **last completed polling cycle** and is eventually consistent.
- No authentication is required. The endpoint is intended for cluster-internal use. Restrict access via Kubernetes NetworkPolicy if needed.
- In large clusters the payload may be substantial. Pagination and filtering may be added in future versions.
