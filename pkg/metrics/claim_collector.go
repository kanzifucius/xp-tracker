// Package metrics implements Prometheus collectors for Crossplane claims and XRs.
package metrics

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	claimLabels = []string{"group", "kind", "version", "namespace", "creator", "team", "claim_name", "synced", "ready", "reason", "paused", "deleting"}

	claimTotalDesc = prometheus.NewDesc(
		"crossplane_claims_total",
		"Number of Crossplane claims by group, kind, namespace, creator, claim_name, and status.",
		claimLabels,
		nil,
	)

	claimReadyDesc = prometheus.NewDesc(
		"crossplane_claims_ready",
		"Number of Ready Crossplane claims by group, kind, namespace, creator, claim_name, and status.",
		claimLabels,
		nil,
	)

	claimStatusSyncedDesc = prometheus.NewDesc(
		"crossplane_claims_status_synced",
		"Synced status for Crossplane claims (1=true, 0=false).",
		claimLabels,
		nil,
	)

	claimStatusReadyDesc = prometheus.NewDesc(
		"crossplane_claims_status_ready",
		"Ready status for Crossplane claims (1=true, 0=false).",
		claimLabels,
		nil,
	)

	claimCreatedTimestampDesc = prometheus.NewDesc(
		"crossplane_claims_created_timestamp_seconds",
		"Unix creation timestamp of Crossplane claims.",
		claimLabels,
		nil,
	)

	claimDeletionTimestampDesc = prometheus.NewDesc(
		"crossplane_claims_deletion_timestamp_seconds",
		"Unix deletion timestamp of Crossplane claims (emitted only while deleting).",
		claimLabels,
		nil,
	)
)

// claimAggKey is the label tuple used to aggregate claim metrics.
type claimAggKey struct {
	Group     string
	Kind      string
	Version   string
	Namespace string
	Creator   string
	Team      string
	ClaimName string
	Synced    string
	Ready     string
	Reason    string
	Paused    string
	Deleting  string
}

// claimAggVal holds aggregated counts for a claim label tuple.
type claimAggVal struct {
	Total       int
	Ready       int
	SyncedCount int
	CreatedAt   time.Time
	DeletedAt   time.Time
}

// ClaimCollector implements prometheus.Collector for Crossplane claims.
type ClaimCollector struct {
	store store.Store
}

// NewClaimCollector creates a new ClaimCollector.
func NewClaimCollector(s store.Store) *ClaimCollector {
	return &ClaimCollector{store: s}
}

// Describe sends the metric descriptors to the channel.
func (c *ClaimCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- claimTotalDesc
	ch <- claimReadyDesc
	ch <- claimStatusSyncedDesc
	ch <- claimStatusReadyDesc
	ch <- claimCreatedTimestampDesc
	ch <- claimDeletionTimestampDesc
}

// Collect snapshots the store, aggregates by label tuple, and emits gauge metrics.
func (c *ClaimCollector) Collect(ch chan<- prometheus.Metric) {
	claims := c.store.SnapshotClaims()

	agg := make(map[claimAggKey]*claimAggVal)
	for _, claim := range claims {
		key := claimAggKey{
			Group:     claim.Group,
			Kind:      claim.Kind,
			Version:   claim.Version,
			Namespace: claim.Namespace,
			Creator:   claim.Creator,
			Team:      claim.Team,
			ClaimName: claim.Name,
			Synced:    boolToLabel(claim.Synced),
			Ready:     boolToLabel(claim.Ready),
			Reason:    claim.Reason,
			Paused:    boolToLabel(claim.Paused),
			Deleting:  boolToLabel(!claim.DeletedAt.IsZero()),
		}
		v, ok := agg[key]
		if !ok {
			v = &claimAggVal{}
			agg[key] = v
		}
		v.Total++
		if claim.Ready {
			v.Ready++
		}
		if claim.Synced {
			v.SyncedCount++
		}
		if !claim.CreatedAt.IsZero() && (v.CreatedAt.IsZero() || claim.CreatedAt.Before(v.CreatedAt)) {
			v.CreatedAt = claim.CreatedAt
		}
		if !claim.DeletedAt.IsZero() && (v.DeletedAt.IsZero() || claim.DeletedAt.Before(v.DeletedAt)) {
			v.DeletedAt = claim.DeletedAt
		}
	}

	for key, val := range agg {
		labels := []string{
			key.Group, key.Kind, key.Version, key.Namespace, key.Creator, key.Team, key.ClaimName,
			key.Synced, key.Ready, key.Reason, key.Paused, key.Deleting,
		}

		m, err := prometheus.NewConstMetric(
			claimTotalDesc,
			prometheus.GaugeValue,
			float64(val.Total),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create claim_total metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			claimReadyDesc,
			prometheus.GaugeValue,
			float64(val.Ready),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create claim_ready metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			claimStatusSyncedDesc,
			prometheus.GaugeValue,
			float64(val.SyncedCount),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create claim_status_synced metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			claimStatusReadyDesc,
			prometheus.GaugeValue,
			float64(val.Ready),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create claim_status_ready metric", "error", err)
			continue
		}
		ch <- m

		if !val.CreatedAt.IsZero() {
			m, err = prometheus.NewConstMetric(
				claimCreatedTimestampDesc,
				prometheus.GaugeValue,
				float64(val.CreatedAt.Unix()),
				labels...,
			)
			if err != nil {
				slog.Error("failed to create claim_created_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}

		if !val.DeletedAt.IsZero() {
			m, err = prometheus.NewConstMetric(
				claimDeletionTimestampDesc,
				prometheus.GaugeValue,
				float64(val.DeletedAt.Unix()),
				labels...,
			)
			if err != nil {
				slog.Error("failed to create claim_deletion_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}
	}
}

func boolToLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
