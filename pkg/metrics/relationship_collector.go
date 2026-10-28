package metrics

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

var (
	xrChildInfoLabels = []string{
		"parent_group", "parent_kind", "parent_namespace", "parent_name",
		"child_group", "child_kind", "child_namespace", "child_name",
	}

	mrOwnerInfoLabels = []string{
		"owner_group", "owner_kind", "owner_namespace", "owner_name",
		"mr_group", "mr_kind", "mr_namespace", "mr_name",
	}
)

// RelationshipCollector emits optional edge info metrics for node-graph panels.
// XR→XR edges come from parent_* on child XRs. MR→XR edges are opt-in and
// discouraged at large MR counts (~1200+).
type RelationshipCollector struct {
	store           store.Store
	emitXRChildInfo bool
	emitMROwnerInfo bool
	xrChildDesc     *prometheus.Desc
	mrOwnerDesc     *prometheus.Desc
}

// NewRelationshipCollector creates a RelationshipCollector.
// Both edge families default off in production config.
func NewRelationshipCollector(s store.Store, emitXRChildInfo, emitMROwnerInfo bool) *RelationshipCollector {
	c := &RelationshipCollector{
		store:           s,
		emitXRChildInfo: emitXRChildInfo,
		emitMROwnerInfo: emitMROwnerInfo,
	}
	if emitXRChildInfo {
		c.xrChildDesc = prometheus.NewDesc(
			"crossplane_xr_child_info",
			"XR→XR nesting edge (1 per child XR with a resolved parent). Opt-in via XR_CHILD_INFO_METRIC.",
			xrChildInfoLabels,
			nil,
		)
	}
	if emitMROwnerInfo {
		c.mrOwnerDesc = prometheus.NewDesc(
			"crossplane_mr_owner_info",
			"MR→XR immediate-owner edge (1 per MR with a resolved owner_xr). Opt-in via MR_OWNER_INFO_METRIC; prefer owner_xr_* labels when MR count is large.",
			mrOwnerInfoLabels,
			nil,
		)
	}
	return c
}

// Describe sends enabled edge metric descriptors.
func (c *RelationshipCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.xrChildDesc != nil {
		ch <- c.xrChildDesc
	}
	if c.mrOwnerDesc != nil {
		ch <- c.mrOwnerDesc
	}
}

// Collect emits enabled edge gauges.
func (c *RelationshipCollector) Collect(ch chan<- prometheus.Metric) {
	if c.emitXRChildInfo {
		c.collectXRChildInfo(ch)
	}
	if c.emitMROwnerInfo {
		c.collectMROwnerInfo(ch)
	}
}

func (c *RelationshipCollector) collectXRChildInfo(ch chan<- prometheus.Metric) {
	for _, xr := range c.store.SnapshotXRs() {
		if xr.ParentName == "" {
			continue
		}
		m, err := prometheus.NewConstMetric(
			c.xrChildDesc,
			prometheus.GaugeValue,
			1,
			xr.ParentGroup, xr.ParentKind, xr.ParentNamespace, xr.ParentName,
			xr.Group, xr.Kind, xr.Namespace, xr.Name,
		)
		if err != nil {
			slog.Error("failed to create xr_child_info metric", "error", err)
			continue
		}
		ch <- m
	}
}

func (c *RelationshipCollector) collectMROwnerInfo(ch chan<- prometheus.Metric) {
	for _, mr := range c.store.SnapshotMRs() {
		if mr.OwnerXRName == "" {
			continue
		}
		m, err := prometheus.NewConstMetric(
			c.mrOwnerDesc,
			prometheus.GaugeValue,
			1,
			mr.OwnerXRGroup, mr.OwnerXRKind, mr.OwnerXRNamespace, mr.OwnerXRName,
			mr.Group, mr.Kind, mr.Namespace, mr.Name,
		)
		if err != nil {
			slog.Error("failed to create mr_owner_info metric", "error", err)
			continue
		}
		ch <- m
	}
}
