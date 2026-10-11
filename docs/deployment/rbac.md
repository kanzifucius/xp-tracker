# RBAC

xp-tracker needs **read-only** access to the Crossplane claim, XR, managed-resource, and package resources it polls, plus XRD and MRD discovery resources.

## Minimum RBAC for discovery and polling

At startup the exporter lists Crossplane discovery APIs, then polls the claim, XR, and MR types those definitions describe, plus a fixed set of package meta-types. The **minimum** ClusterRole therefore has three parts:

1. **Discovery** — `get`, `list`, and `watch` on:
   - `compositeresourcedefinitions` in `apiextensions.crossplane.io` (v1 and v2 XRDs)
   - `managedresourcedefinitions` in `apiextensions.crossplane.io` (provider MR GVRs)
2. **Package inventory** — `get`, `list`, and `watch` on:
   - `providers`, `configurations`, and `functions` in `pkg.crossplane.io`
   - Do **not** grant package revisions (`providerrevisions`, `configurationrevisions`, `functionrevisions`); xp-tracker does not list them
3. **Polling** — `get`, `list`, and `watch` on every claim, XR, and MR resource plural you expect metrics for (the types discovered from XRDs/MRDs, or any static overrides you still set)

Without the discovery rules the exporter fails at startup (for example with `no XR GVRs discovered from XRDs` or list-forbidden errors on XRDs/MRDs), even if claim/XR plurals are already granted. Without the package rules, package inventory gauges stay empty (list errors are counted in `xp_tracker_poll_errors_total`).

!!! tip
    Include claim plurals when tracking legacy `LegacyCluster` XRDs that define `claimNames`. Include managed-resource plurals (and the MRD discovery rule above) when you want XR-linked MR metrics. Composition is resolved internally and is **not** a separate RBAC or metric dimension.

## Default ClusterRole

The base Kustomize manifests include a ClusterRole with broad read access:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: crossplane-metrics-exporter
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
```

!!! warning
    The base ClusterRole is intentionally broad for quick-start convenience. For production, scope it down to the discovery APIs plus only the API groups and resources you actually poll. The base manifest includes a warning comment to this effect; see `deploy/overlays/example/` for a discovery-complete least-privilege patch.

## Scoping for production

Restrict the ClusterRole to discovery plus your platform types. For example, if you track `postgresqlinstances` and `kafkatopics` on `platform.example.org`:

```yaml
rules:
  - apiGroups: ["apiextensions.crossplane.io"]
    resources:
      - compositeresourcedefinitions
      - managedresourcedefinitions
    verbs: ["get", "list", "watch"]
  - apiGroups: ["pkg.crossplane.io"]
    resources:
      - providers
      - configurations
      - functions
    verbs: ["get", "list", "watch"]
  - apiGroups: ["platform.example.org"]
    resources:
      - postgresqlinstances
      - xpostgresqlinstances
      - kafkatopics
      - xkafkatopics
    verbs: ["get", "list", "watch"]
```

Add further `apiGroups` / `resources` entries for each provider MR plural you poll (those GVRs come from Active `ManagedResourceDefinition` objects).

You can override the ClusterRole via a Kustomize patch in your overlay. The example overlay at `deploy/overlays/example/` includes a scoped ClusterRole patch that demonstrates this pattern, including the discovery and package rules:

```yaml
patches:
  - target:
      kind: ClusterRole
      name: crossplane-metrics-exporter
    patch: |
      apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRole
      metadata:
        name: crossplane-metrics-exporter
      rules:
        - apiGroups: ["apiextensions.crossplane.io"]
          resources:
            - compositeresourcedefinitions
            - managedresourcedefinitions
          verbs: ["get", "list", "watch"]
        - apiGroups: ["pkg.crossplane.io"]
          resources:
            - providers
            - configurations
            - functions
          verbs: ["get", "list", "watch"]
        - apiGroups: ["platform.example.org"]
          resources:
            - postgresqlinstances
            - xpostgresqlinstances
            - kafkatopics
            - xkafkatopics
          verbs: ["get", "list", "watch"]
```

## Binding

The ClusterRoleBinding binds the ClusterRole to the `crossplane-metrics-exporter` ServiceAccount in the `crossplane-system` namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: crossplane-metrics-exporter
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: crossplane-metrics-exporter
subjects:
  - kind: ServiceAccount
    name: crossplane-metrics-exporter
    namespace: crossplane-system
```

## Why read-only?

xp-tracker is strictly a metrics exporter. It never creates, updates, or deletes resources. The `get`, `list`, and `watch` verbs are the minimum required to poll the API server for resource metadata.
