package metrics

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	mrTotalDesc = prometheus.NewDesc(
		"crossplane_mr_total",
		"Number of Crossplane provider managed resources by group, kind, namespace, name, and status.",
		[]string{"group", "kind", "namespace", "name", "xr_name", "claim_name", "claim_namespace", "provider", "provider_config", "synced", "ready"},
		nil,
	)

	mrReadyDesc = prometheus.NewDesc(
		"crossplane_mr_ready",
		"Number of Ready Crossplane provider managed resources by group, kind, namespace, name, and status.",
		[]string{"group", "kind", "namespace", "name", "xr_name", "claim_name", "claim_namespace", "provider", "provider_config", "synced", "ready"},
		nil,
	)

	mrStatusSyncedDesc = prometheus.NewDesc(
		"crossplane_mr_status_synced",
		"Synced status for Crossplane provider managed resources (1=true, 0=false).",
		[]string{"group", "kind", "namespace", "name", "xr_name", "claim_name", "claim_namespace", "provider", "provider_config", "synced", "ready"},
		nil,
	)

	mrStatusReadyDesc = prometheus.NewDesc(
		"crossplane_mr_status_ready",
		"Ready status for Crossplane provider managed resources (1=true, 0=false).",
		[]string{"group", "kind", "namespace", "name", "xr_name", "claim_name", "claim_namespace", "provider", "provider_config", "synced", "ready"},
		nil,
	)
)

// mrAggKey is the label tuple used to aggregate MR metrics.
type mrAggKey struct {
	Group          string
	Kind           string
	Namespace      string
	Name           string
	XRName         string
	ClaimName      string
	ClaimNS        string
	Provider       string
	ProviderConfig string
	Synced         string
	Ready          string
}

// mrAggVal holds aggregated counts for an MR label tuple.
type mrAggVal struct {
	Total       int
	Ready       int
	SyncedCount int
}

// MRCollector implements prometheus.Collector for Crossplane provider managed resources.
type MRCollector struct {
	store store.Store
}

// NewMRCollector creates a new MRCollector.
func NewMRCollector(s store.Store) *MRCollector {
	return &MRCollector{store: s}
}

// Describe sends the metric descriptors to the channel.
func (c *MRCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mrTotalDesc
	ch <- mrReadyDesc
	ch <- mrStatusSyncedDesc
	ch <- mrStatusReadyDesc
}

// Collect snapshots the store, aggregates by label tuple, and emits gauge metrics.
func (c *MRCollector) Collect(ch chan<- prometheus.Metric) {
	mrs := c.store.SnapshotMRs()

	agg := make(map[mrAggKey]*mrAggVal)
	for _, mr := range mrs {
		key := mrAggKey{
			Group:          mr.Group,
			Kind:           mr.Kind,
			Namespace:      mr.Namespace,
			Name:           mr.Name,
			XRName:         mr.XRName,
			ClaimName:      mr.ClaimName,
			ClaimNS:        mr.ClaimNS,
			Provider:       mr.Provider,
			ProviderConfig: mr.ProviderConfig,
			Synced:         boolToLabel(mr.Synced),
			Ready:          boolToLabel(mr.Ready),
		}
		v, ok := agg[key]
		if !ok {
			v = &mrAggVal{}
			agg[key] = v
		}
		v.Total++
		if mr.Ready {
			v.Ready++
		}
		if mr.Synced {
			v.SyncedCount++
		}
	}

	for key, val := range agg {
		labels := []string{
			key.Group, key.Kind, key.Namespace, key.Name,
			key.XRName, key.ClaimName, key.ClaimNS,
			key.Provider, key.ProviderConfig,
			key.Synced, key.Ready,
		}

		m, err := prometheus.NewConstMetric(mrTotalDesc, prometheus.GaugeValue, float64(val.Total), labels...)
		if err != nil {
			slog.Error("failed to create mr_total metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(mrReadyDesc, prometheus.GaugeValue, float64(val.Ready), labels...)
		if err != nil {
			slog.Error("failed to create mr_ready metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(mrStatusSyncedDesc, prometheus.GaugeValue, float64(val.SyncedCount), labels...)
		if err != nil {
			slog.Error("failed to create mr_status_synced metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(mrStatusReadyDesc, prometheus.GaugeValue, float64(val.Ready), labels...)
		if err != nil {
			slog.Error("failed to create mr_status_ready metric", "error", err)
			continue
		}
		ch <- m
	}
}
