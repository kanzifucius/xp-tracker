package metrics

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	xrTotalDesc = prometheus.NewDesc(
		"crossplane_xr_total",
		"Number of Crossplane composite resources (XRs) by group, kind, namespace, composition, name, and status.",
		[]string{"group", "kind", "namespace", "composition", "name", "synced", "ready"},
		nil,
	)

	xrReadyDesc = prometheus.NewDesc(
		"crossplane_xr_ready",
		"Number of Ready Crossplane XRs by group, kind, namespace, composition, name, and status.",
		[]string{"group", "kind", "namespace", "composition", "name", "synced", "ready"},
		nil,
	)

	xrStatusSyncedDesc = prometheus.NewDesc(
		"crossplane_xr_status_synced",
		"Synced status for Crossplane XRs (1=true, 0=false).",
		[]string{"group", "kind", "namespace", "composition", "name", "synced", "ready"},
		nil,
	)

	xrStatusReadyDesc = prometheus.NewDesc(
		"crossplane_xr_status_ready",
		"Ready status for Crossplane XRs (1=true, 0=false).",
		[]string{"group", "kind", "namespace", "composition", "name", "synced", "ready"},
		nil,
	)
)

// xrAggKey is the label tuple used to aggregate XR metrics.
type xrAggKey struct {
	Group       string
	Kind        string
	Namespace   string
	Composition string
	Name        string
	Synced      string
	Ready       string
}

// xrAggVal holds aggregated counts for an XR label tuple.
type xrAggVal struct {
	Total       int
	Ready       int
	SyncedCount int
}

// XRCollector implements prometheus.Collector for Crossplane composite resources.
type XRCollector struct {
	store store.Store
}

// NewXRCollector creates a new XRCollector.
func NewXRCollector(s store.Store) *XRCollector {
	return &XRCollector{store: s}
}

// Describe sends the metric descriptors to the channel.
func (c *XRCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- xrTotalDesc
	ch <- xrReadyDesc
	ch <- xrStatusSyncedDesc
	ch <- xrStatusReadyDesc
}

// Collect snapshots the store, aggregates by label tuple, and emits gauge metrics.
func (c *XRCollector) Collect(ch chan<- prometheus.Metric) {
	xrs := c.store.SnapshotXRs()

	agg := make(map[xrAggKey]*xrAggVal)
	for _, xr := range xrs {
		key := xrAggKey{
			Group:       xr.Group,
			Kind:        xr.Kind,
			Namespace:   xr.Namespace,
			Composition: xr.Composition,
			Name:        xr.Name,
			Synced:      boolToLabel(xr.Synced),
			Ready:       boolToLabel(xr.Ready),
		}
		v, ok := agg[key]
		if !ok {
			v = &xrAggVal{}
			agg[key] = v
		}
		v.Total++
		if xr.Ready {
			v.Ready++
		}
		if xr.Synced {
			v.SyncedCount++
		}
	}

	for key, val := range agg {
		m, err := prometheus.NewConstMetric(
			xrTotalDesc,
			prometheus.GaugeValue,
			float64(val.Total),
			key.Group, key.Kind, key.Namespace, key.Composition, key.Name, key.Synced, key.Ready,
		)
		if err != nil {
			slog.Error("failed to create xr_total metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			xrReadyDesc,
			prometheus.GaugeValue,
			float64(val.Ready),
			key.Group, key.Kind, key.Namespace, key.Composition, key.Name, key.Synced, key.Ready,
		)
		if err != nil {
			slog.Error("failed to create xr_ready metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			xrStatusSyncedDesc,
			prometheus.GaugeValue,
			float64(val.SyncedCount),
			key.Group, key.Kind, key.Namespace, key.Composition, key.Name, key.Synced, key.Ready,
		)
		if err != nil {
			slog.Error("failed to create xr_status_synced metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			xrStatusReadyDesc,
			prometheus.GaugeValue,
			float64(val.Ready),
			key.Group, key.Kind, key.Namespace, key.Composition, key.Name, key.Synced, key.Ready,
		)
		if err != nil {
			slog.Error("failed to create xr_status_ready metric", "error", err)
			continue
		}
		ch <- m
	}
}
