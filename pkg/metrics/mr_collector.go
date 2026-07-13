package metrics

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	mrLabels = []string{
		"group", "kind", "version", "namespace", "name", "xr_name", "claim_name", "claim_namespace",
		"provider", "provider_config", "external_name", "management_policies",
		"synced", "ready", "reason", "paused", "deleting",
	}

	mrTotalDesc = prometheus.NewDesc(
		"crossplane_mr_total",
		"Number of Crossplane provider managed resources by group, kind, namespace, name, and status.",
		mrLabels,
		nil,
	)

	mrReadyDesc = prometheus.NewDesc(
		"crossplane_mr_ready",
		"Number of Ready Crossplane provider managed resources by group, kind, namespace, name, and status.",
		mrLabels,
		nil,
	)

	mrStatusSyncedDesc = prometheus.NewDesc(
		"crossplane_mr_status_synced",
		"Synced status for Crossplane provider managed resources (1=true, 0=false).",
		mrLabels,
		nil,
	)

	mrStatusReadyDesc = prometheus.NewDesc(
		"crossplane_mr_status_ready",
		"Ready status for Crossplane provider managed resources (1=true, 0=false).",
		mrLabels,
		nil,
	)

	mrCreatedTimestampDesc = prometheus.NewDesc(
		"crossplane_mr_created_timestamp_seconds",
		"Unix creation timestamp of Crossplane provider managed resources.",
		mrLabels,
		nil,
	)

	mrDeletionTimestampDesc = prometheus.NewDesc(
		"crossplane_mr_deletion_timestamp_seconds",
		"Unix deletion timestamp of Crossplane provider managed resources (emitted only while deleting).",
		mrLabels,
		nil,
	)
)

// mrAggKey is the label tuple used to aggregate MR metrics.
type mrAggKey struct {
	Group              string
	Kind               string
	Version            string
	Namespace          string
	Name               string
	XRName             string
	ClaimName          string
	ClaimNS            string
	Provider           string
	ProviderConfig     string
	ExternalName       string
	ManagementPolicies string
	Synced             string
	Ready              string
	Reason             string
	Paused             string
	Deleting           string
}

// mrAggVal holds aggregated counts for an MR label tuple.
type mrAggVal struct {
	Total       int
	Ready       int
	SyncedCount int
	CreatedAt   time.Time
	DeletedAt   time.Time
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
	ch <- mrCreatedTimestampDesc
	ch <- mrDeletionTimestampDesc
}

// Collect snapshots the store, aggregates by label tuple, and emits gauge metrics.
func (c *MRCollector) Collect(ch chan<- prometheus.Metric) {
	mrs := c.store.SnapshotMRs()

	agg := make(map[mrAggKey]*mrAggVal)
	for _, mr := range mrs {
		key := mrAggKey{
			Group:              mr.Group,
			Kind:               mr.Kind,
			Version:            mr.Version,
			Namespace:          mr.Namespace,
			Name:               mr.Name,
			XRName:             mr.XRName,
			ClaimName:          mr.ClaimName,
			ClaimNS:            mr.ClaimNS,
			Provider:           mr.Provider,
			ProviderConfig:     mr.ProviderConfig,
			ExternalName:       mr.ExternalName,
			ManagementPolicies: mr.ManagementPolicies,
			Synced:             boolToLabel(mr.Synced),
			Ready:              boolToLabel(mr.Ready),
			Reason:             mr.Reason,
			Paused:             boolToLabel(mr.Paused),
			Deleting:           boolToLabel(!mr.DeletedAt.IsZero()),
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
		if !mr.CreatedAt.IsZero() && (v.CreatedAt.IsZero() || mr.CreatedAt.Before(v.CreatedAt)) {
			v.CreatedAt = mr.CreatedAt
		}
		if !mr.DeletedAt.IsZero() && (v.DeletedAt.IsZero() || mr.DeletedAt.Before(v.DeletedAt)) {
			v.DeletedAt = mr.DeletedAt
		}
	}

	for key, val := range agg {
		labels := []string{
			key.Group, key.Kind, key.Version, key.Namespace, key.Name,
			key.XRName, key.ClaimName, key.ClaimNS,
			key.Provider, key.ProviderConfig, key.ExternalName, key.ManagementPolicies,
			key.Synced, key.Ready, key.Reason, key.Paused, key.Deleting,
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

		if !val.CreatedAt.IsZero() {
			m, err = prometheus.NewConstMetric(mrCreatedTimestampDesc, prometheus.GaugeValue, float64(val.CreatedAt.Unix()), labels...)
			if err != nil {
				slog.Error("failed to create mr_created_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}

		if !val.DeletedAt.IsZero() {
			m, err = prometheus.NewConstMetric(mrDeletionTimestampDesc, prometheus.GaugeValue, float64(val.DeletedAt.Unix()), labels...)
			if err != nil {
				slog.Error("failed to create mr_deletion_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}
	}
}
