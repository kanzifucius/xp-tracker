# Metrics Reference

xp-tracker exposes four Prometheus **gauge** metrics, recomputed on each scrape from the in-memory store.

## Claim metrics

### `crossplane_claims_total`

Total number of Crossplane claims, broken down by label tuple.

| Label | Description |
|---|---|
| `group` | API group from the GVR (e.g. `platform.example.org`) |
| `kind` | Resource kind (e.g. `PostgresqlInstance`) |
| `namespace` | Kubernetes namespace |
| `composition` | Crossplane Composition name (enriched from backing XR) |
| `creator` | Value of the `CREATOR_ANNOTATION_KEY` annotation |
| `team` | Value of the `TEAM_ANNOTATION_KEY` annotation |

### `crossplane_claims_ready`

Number of Crossplane claims with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_claims_total`.

## XR metrics

### `crossplane_xr_total`

Total number of Crossplane composite resources (XRs), broken down by label tuple.

| Label | Description |
|---|---|
| `group` | API group from the GVR |
| `kind` | Resource kind (e.g. `XPostgreSQLInstance`) |
| `namespace` | Kubernetes namespace (usually empty for cluster-scoped XRs) |
| `composition` | Crossplane Composition name (from `COMPOSITION_LABEL_KEY` label) |

### `crossplane_xr_ready`

Number of XRs with `status.conditions` containing `type: Ready` and `status: "True"`. Same label set as `crossplane_xr_total`.

## Example output

Output from `curl localhost:8080/metrics` with sample resources applied:

```prometheus
# HELP crossplane_claims_ready Number of Ready Crossplane claims by group, kind, namespace, composition and creator.
# TYPE crossplane_claims_ready gauge
crossplane_claims_ready{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",team="platform"} 0
crossplane_claims_ready{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",team="platform"} 0
crossplane_claims_ready{composition="",creator="bob@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-beta",team="backend"} 0

# HELP crossplane_claims_total Number of Crossplane claims by group, kind, namespace, composition and creator.
# TYPE crossplane_claims_total gauge
crossplane_claims_total{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Gadget",namespace="team-alpha",team="platform"} 1
crossplane_claims_total{composition="",creator="alice@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-alpha",team="platform"} 2
crossplane_claims_total{composition="",creator="bob@example.com",group="samples.xptracker.dev",kind="Widget",namespace="team-beta",team="backend"} 1

# HELP crossplane_xr_ready Number of Ready Crossplane XRs by group, kind, namespace and composition.
# TYPE crossplane_xr_ready gauge
crossplane_xr_ready{composition="",group="samples.xptracker.dev",kind="XGadget",namespace=""} 0
crossplane_xr_ready{composition="",group="samples.xptracker.dev",kind="XWidget",namespace=""} 0

# HELP crossplane_xr_total Number of Crossplane composite resources (XRs) by group, kind, namespace and composition.
# TYPE crossplane_xr_total gauge
crossplane_xr_total{composition="",group="samples.xptracker.dev",kind="XGadget",namespace=""} 4
crossplane_xr_total{composition="",group="samples.xptracker.dev",kind="XWidget",namespace=""} 4
```

## Aggregation behaviour

Metrics are aggregated by their full label tuple. For example, if two claims in namespace `team-a` have the same group, kind, composition, creator, and team, they are counted as a single time series with value `2`.

This means the cardinality is bounded by the number of **unique label combinations**, not the total number of resources.

## Label notes

- **Empty labels**: if an annotation key is not configured or the annotation is not present on a resource, the label value is an empty string (`""`).
- **Composition enrichment**: claims inherit their `composition` label from the backing XR via the `spec.resourceRef.name` linkage. If the claim has no resource reference yet, the composition will be empty.
- **Namespace for XRs**: composite resources are typically cluster-scoped, so the `namespace` label is usually empty.
