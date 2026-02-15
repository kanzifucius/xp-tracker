# Prometheus Scraping

xp-tracker exposes metrics on `GET /metrics` (port 8080 by default). There are two main approaches to scraping.

## Scrape configuration

=== "ServiceMonitor (Prometheus Operator)"

    If you use the [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator), add a `ServiceMonitor` to your Kustomize overlay. The example overlay at `deploy/overlays/example/` includes one:

    ```yaml
    apiVersion: monitoring.coreos.com/v1
    kind: ServiceMonitor
    metadata:
      name: crossplane-metrics-exporter
      labels:
        app.kubernetes.io/name: crossplane-metrics-exporter
        app.kubernetes.io/component: exporter
    spec:
      selector:
        matchLabels:
          app.kubernetes.io/name: crossplane-metrics-exporter
      endpoints:
        - port: metrics
          interval: 30s
          path: /metrics
    ```

    !!! important "Prometheus selector configuration"
        If your Prometheus instance uses `serviceMonitorSelector`, make sure the ServiceMonitor labels match. If using kube-prometheus-stack, you may need to set `serviceMonitorSelectorNilUsesHelmValues: false` in your Prometheus Helm values so it discovers all ServiceMonitors regardless of labels.

=== "Scrape Config (Direct)"

    Add a scrape config directly to your `prometheus.yml`:

    ```yaml
    scrape_configs:
      - job_name: crossplane-metrics-exporter
        kubernetes_sd_configs:
          - role: endpoints
            namespaces:
              names:
                - crossplane-system
        relabel_configs:
          - source_labels: [__meta_kubernetes_service_name]
            regex: crossplane-metrics-exporter
            action: keep
          - source_labels: [__meta_kubernetes_endpoint_port_name]
            regex: metrics
            action: keep
    ```

## Scrape interval

Set the scrape interval to match or exceed your `POLL_INTERVAL_SECONDS` (default: 30s). Scraping faster than the poll interval won't provide more data -- the metrics are recomputed from the in-memory store on each scrape, and the store is only updated on each poll cycle.

## Verifying

Port-forward to the exporter and check the metrics endpoint:

```bash
kubectl -n crossplane-system port-forward svc/crossplane-metrics-exporter 8080:8080
curl -s localhost:8080/metrics | grep crossplane_
```

You should see the four gauge metrics: `crossplane_claims_total`, `crossplane_claims_ready`, `crossplane_xr_total`, `crossplane_xr_ready`.
