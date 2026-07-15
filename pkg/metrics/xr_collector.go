package metrics

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	xrLabels = []string{"group", "kind", "version", "namespace", "name", "claim_name", "claim_namespace", "synced", "ready", "reason", "paused", "deleting"}

	xrTotalDesc = prometheus.NewDesc(
		"crossplane_xr_total",
		"Number of Crossplane composite resources (XRs) by group, kind, namespace, name, and status.",
		xrLabels,
		nil,
	)

	xrReadyDesc = prometheus.NewDesc(
		"crossplane_xr_ready",
		"Number of Ready Crossplane XRs by group, kind, namespace, name, and status.",
		xrLabels,
		nil,
	)

	xrStatusSyncedDesc = prometheus.NewDesc(
		"crossplane_xr_status_synced",
		"Synced status for Crossplane XRs (1=true, 0=false).",
		xrLabels,
		nil,
	)

	xrStatusReadyDesc = prometheus.NewDesc(
		"crossplane_xr_status_ready",
		"Ready status for Crossplane XRs (1=true, 0=false).",
		xrLabels,
		nil,
	)

	xrCreatedTimestampDesc = prometheus.NewDesc(
		"crossplane_xr_created_timestamp_seconds",
		"Unix creation timestamp of Crossplane composite resources.",
		xrLabels,
		nil,
	)

	xrDeletionTimestampDesc = prometheus.NewDesc(
		"crossplane_xr_deletion_timestamp_seconds",
		"Unix deletion timestamp of Crossplane composite resources (emitted only while deleting).",
		xrLabels,
		nil,
	)
)

// xrAggKey is the label tuple used to aggregate XR metrics.
type xrAggKey struct {
	Group     string
	Kind      string
	Version   string
	Namespace string
	Name      string
	ClaimName string
	ClaimNS   string
	Synced    string
	Ready     string
	Reason    string
	Paused    string
	Deleting  string
}

// xrAggVal holds aggregated counts for an XR label tuple.
type xrAggVal struct {
	Total       int
	Ready       int
	SyncedCount int
	CreatedAt   time.Time
	DeletedAt   time.Time
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
	ch <- xrCreatedTimestampDesc
	ch <- xrDeletionTimestampDesc
}

// Collect snapshots the store, aggregates by label tuple, and emits gauge metrics.
func (c *XRCollector) Collect(ch chan<- prometheus.Metric) {
	xrs := c.store.SnapshotXRs()

	agg := make(map[xrAggKey]*xrAggVal)
	for _, xr := range xrs {
		key := xrAggKey{
			Group:     xr.Group,
			Kind:      xr.Kind,
			Version:   xr.Version,
			Namespace: xr.Namespace,
			Name:      xr.Name,
			ClaimName: xr.ClaimName,
			ClaimNS:   xr.ClaimNS,
			Synced:    boolToLabel(xr.Synced),
			Ready:     boolToLabel(xr.Ready),
			Reason:    xr.Reason,
			Paused:    boolToLabel(xr.Paused),
			Deleting:  boolToLabel(!xr.DeletedAt.IsZero()),
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
		if !xr.CreatedAt.IsZero() && (v.CreatedAt.IsZero() || xr.CreatedAt.Before(v.CreatedAt)) {
			v.CreatedAt = xr.CreatedAt
		}
		if !xr.DeletedAt.IsZero() && (v.DeletedAt.IsZero() || xr.DeletedAt.Before(v.DeletedAt)) {
			v.DeletedAt = xr.DeletedAt
		}
	}

	for key, val := range agg {
		labels := []string{
			key.Group, key.Kind, key.Version, key.Namespace, key.Name, key.ClaimName, key.ClaimNS,
			key.Synced, key.Ready, key.Reason, key.Paused, key.Deleting,
		}

		m, err := prometheus.NewConstMetric(
			xrTotalDesc,
			prometheus.GaugeValue,
			float64(val.Total),
			labels...,
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
			labels...,
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
			labels...,
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
			labels...,
		)
		if err != nil {
			slog.Error("failed to create xr_status_ready metric", "error", err)
			continue
		}
		ch <- m

		if !val.CreatedAt.IsZero() {
			m, err = prometheus.NewConstMetric(
				xrCreatedTimestampDesc,
				prometheus.GaugeValue,
				float64(val.CreatedAt.Unix()),
				labels...,
			)
			if err != nil {
				slog.Error("failed to create xr_created_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}

		if !val.DeletedAt.IsZero() {
			m, err = prometheus.NewConstMetric(
				xrDeletionTimestampDesc,
				prometheus.GaugeValue,
				float64(val.DeletedAt.Unix()),
				labels...,
			)
			if err != nil {
				slog.Error("failed to create xr_deletion_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}
	}
}
