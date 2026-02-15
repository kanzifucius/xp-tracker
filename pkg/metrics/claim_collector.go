// Package metrics implements Prometheus collectors for Crossplane claims and XRs.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	claimTotalDesc = prometheus.NewDesc(
		"crossplane_claims_total",
		"Number of Crossplane claims by group, kind, namespace, composition and creator.",
		[]string{"group", "kind", "namespace", "composition", "creator", "team"},
		nil,
	)

	claimReadyDesc = prometheus.NewDesc(
		"crossplane_claims_ready",
		"Number of Ready Crossplane claims by group, kind, namespace, composition and creator.",
		[]string{"group", "kind", "namespace", "composition", "creator", "team"},
		nil,
	)
)

// claimAggKey is the label tuple used to aggregate claim metrics.
type claimAggKey struct {
	Group       string
	Kind        string
	Namespace   string
	Composition string
	Creator     string
	Team        string
}

// claimAggVal holds aggregated counts for a claim label tuple.
type claimAggVal struct {
	Total int
	Ready int
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
}

// Collect snapshots the store, aggregates by label tuple, and emits gauge metrics.
func (c *ClaimCollector) Collect(ch chan<- prometheus.Metric) {
	claims := c.store.SnapshotClaims()

	agg := make(map[claimAggKey]*claimAggVal)
	for _, claim := range claims {
		key := claimAggKey{
			Group:       claim.Group,
			Kind:        claim.Kind,
			Namespace:   claim.Namespace,
			Composition: claim.Composition,
			Creator:     claim.Creator,
			Team:        claim.Team,
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
	}

	for key, val := range agg {
		ch <- prometheus.MustNewConstMetric(
			claimTotalDesc,
			prometheus.GaugeValue,
			float64(val.Total),
			key.Group, key.Kind, key.Namespace, key.Composition, key.Creator, key.Team,
		)
		ch <- prometheus.MustNewConstMetric(
			claimReadyDesc,
			prometheus.GaugeValue,
			float64(val.Ready),
			key.Group, key.Kind, key.Namespace, key.Composition, key.Creator, key.Team,
		)
	}
}
