# RBAC

xp-tracker needs **read-only** access to the Crossplane claim and XR resources it polls.

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
    The base ClusterRole is intentionally broad for quick-start convenience. For production, scope it down to only the API groups and resources you actually poll. The base manifest includes a warning comment to this effect.

## Scoping for production

Restrict the ClusterRole to only the specific API groups and resources you need. For example, if you track `postgresqlinstances` and `kafkatopics`:

```yaml
rules:
  - apiGroups: ["platform.example.org"]
    resources:
      - postgresqlinstances
      - xpostgresqlinstances
      - kafkatopics
      - xkafkatopics
    verbs: ["get", "list", "watch"]
```

!!! tip
    Include both the claim resources and the XR resources. The exporter needs to read XRs to enrich claims with composition information.

You can override the ClusterRole via a Kustomize patch in your overlay. The example overlay at `deploy/overlays/example/` includes a scoped ClusterRole patch that demonstrates this pattern:

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
        - apiGroups: ["platform.example.org"]
          resources:
            - postgresqlinstances
            - xpostgresqlinstances
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
