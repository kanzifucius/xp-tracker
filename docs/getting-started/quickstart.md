# Quick Start

This guide takes you from zero to seeing Crossplane metrics in under five minutes.

## 1. Deploy xp-tracker

Create a Kustomize overlay for your environment:

```
deploy/overlays/my-env/
  kustomization.yaml
```

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

patches:
  - target:
      kind: ConfigMap
      name: crossplane-metrics-exporter
    patch: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: crossplane-metrics-exporter
      data:
        # Optional static overrides (deprecated):
        # CLAIM_GVRS: "myorg.io/v1alpha1/databases,myorg.io/v1alpha1/caches"
        # XR_GVRS: "myorg.io/v1alpha1/xdatabases,myorg.io/v1alpha1/xcaches"
        CREATOR_ANNOTATION_KEY: "myorg.io/created-by"
        TEAM_ANNOTATION_KEY: "myorg.io/team"

images:
  - name: ghcr.io/kanzifucius/xp-tracker
    newTag: latest
```

Apply it:

```bash
kubectl apply -k deploy/overlays/my-env
```

## 2. Verify the exporter is running

```bash
kubectl -n crossplane-system get pods -l app.kubernetes.io/name=crossplane-metrics-exporter
```

## 3. Check metrics

Port-forward to the exporter:

```bash
kubectl -n crossplane-system port-forward svc/crossplane-metrics-exporter 8080:8080
```

Verify the exporter is ready:

```bash
curl -s localhost:8080/readyz
# Should print "ok" once the first poll cycle completes
```

Then query the metrics endpoint:

```bash
curl -s localhost:8080/metrics | grep crossplane_
```

You should see output like:

```
# HELP crossplane_claims_total Number of Crossplane claims by group, kind, namespace, creator, claim_name, and status.
# TYPE crossplane_claims_total gauge
crossplane_claims_total{claim_name="db-1",creator="alice@example.com",deleting="false",group="myorg.io",kind="Database",namespace="team-a",paused="false",ready="true",reason="Available",synced="true",team="platform",version="v1alpha1"} 1
```

## Next steps

- [Configure environment variables](../configuration/environment-variables.md) to tune polling, namespaces, and annotation keys
- [Set up Prometheus scraping](../deployment/prometheus.md) for production monitoring
- [Build Grafana dashboards](../metrics/grafana-queries.md) with example PromQL queries
- [Understand health endpoints](../api/health.md) for Kubernetes probes
