package metrics

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func mrLabelNames(includeXRParent, includeOwnerXR bool) []string {
	labels := []string{"group", "kind", "version", "namespace", "name", "xr_name"}
	if includeXRParent {
		labels = append(labels, "xr_kind", "xr_namespace", "xr_group")
	}
	if includeOwnerXR {
		labels = append(labels, "owner_xr_name", "owner_xr_kind", "owner_xr_namespace", "owner_xr_group")
	}
	labels = append(labels,
		"claim_name", "claim_namespace",
		"provider", "provider_config", "external_name", "management_policies",
		"synced", "ready", "reason", "paused", "deleting",
	)
	return labels
}

func newMRDescs(labels []string) (
	total, ready, statusSynced, statusReady, created, deletion *prometheus.Desc,
) {
	total = prometheus.NewDesc(
		"crossplane_mr_total",
		"Number of Crossplane provider managed resources by group, kind, namespace, name, and status.",
		labels,
		nil,
	)
	ready = prometheus.NewDesc(
		"crossplane_mr_ready",
		"Number of Ready Crossplane provider managed resources by group, kind, namespace, name, and status.",
		labels,
		nil,
	)
	statusSynced = prometheus.NewDesc(
		"crossplane_mr_status_synced",
		"Synced status for Crossplane provider managed resources (1=true, 0=false).",
		labels,
		nil,
	)
	statusReady = prometheus.NewDesc(
		"crossplane_mr_status_ready",
		"Ready status for Crossplane provider managed resources (1=true, 0=false).",
		labels,
		nil,
	)
	created = prometheus.NewDesc(
		"crossplane_mr_created_timestamp_seconds",
		"Unix creation timestamp of Crossplane provider managed resources.",
		labels,
		nil,
	)
	deletion = prometheus.NewDesc(
		"crossplane_mr_deletion_timestamp_seconds",
		"Unix deletion timestamp of Crossplane provider managed resources (emitted only while deleting).",
		labels,
		nil,
	)
	return total, ready, statusSynced, statusReady, created, deletion
}

// mrAggKey is the label tuple used to aggregate MR metrics.
type mrAggKey struct {
	Group              string
	Kind               string
	Version            string
	Namespace          string
	Name               string
	XRName             string
	XRKind             string
	XRNamespace        string
	XRGroup            string
	OwnerXRName        string
	OwnerXRKind        string
	OwnerXRNamespace   string
	OwnerXRGroup       string
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
	store            store.Store
	includeXRParent  bool
	includeOwnerXR   bool
	totalDesc        *prometheus.Desc
	readyDesc        *prometheus.Desc
	statusSyncedDesc *prometheus.Desc
	statusReadyDesc  *prometheus.Desc
	createdDesc      *prometheus.Desc
	deletionDesc     *prometheus.Desc
}

// NewMRCollector creates a new MRCollector.
// includeXRParentLabels adds composite-label typed identity (xr_kind/namespace/group).
// includeOwnerXRLabels adds immediate composing XR labels (owner_xr_*).
func NewMRCollector(s store.Store, includeXRParentLabels, includeOwnerXRLabels bool) *MRCollector {
	labels := mrLabelNames(includeXRParentLabels, includeOwnerXRLabels)
	total, ready, statusSynced, statusReady, created, deletion := newMRDescs(labels)
	return &MRCollector{
		store:            s,
		includeXRParent:  includeXRParentLabels,
		includeOwnerXR:   includeOwnerXRLabels,
		totalDesc:        total,
		readyDesc:        ready,
		statusSyncedDesc: statusSynced,
		statusReadyDesc:  statusReady,
		createdDesc:      created,
		deletionDesc:     deletion,
	}
}

// Describe sends the metric descriptors to the channel.
func (c *MRCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.readyDesc
	ch <- c.statusSyncedDesc
	ch <- c.statusReadyDesc
	ch <- c.createdDesc
	ch <- c.deletionDesc
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
		if c.includeXRParent {
			key.XRKind = mr.XRKind
			key.XRNamespace = mr.XRNamespace
			key.XRGroup = mr.XRGroup
		}
		if c.includeOwnerXR {
			key.OwnerXRName = mr.OwnerXRName
			key.OwnerXRKind = mr.OwnerXRKind
			key.OwnerXRNamespace = mr.OwnerXRNamespace
			key.OwnerXRGroup = mr.OwnerXRGroup
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
		labels := c.labelValues(key)

		m, err := prometheus.NewConstMetric(c.totalDesc, prometheus.GaugeValue, float64(val.Total), labels...)
		if err != nil {
			slog.Error("failed to create mr_total metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(c.readyDesc, prometheus.GaugeValue, float64(val.Ready), labels...)
		if err != nil {
			slog.Error("failed to create mr_ready metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(c.statusSyncedDesc, prometheus.GaugeValue, float64(val.SyncedCount), labels...)
		if err != nil {
			slog.Error("failed to create mr_status_synced metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(c.statusReadyDesc, prometheus.GaugeValue, float64(val.Ready), labels...)
		if err != nil {
			slog.Error("failed to create mr_status_ready metric", "error", err)
			continue
		}
		ch <- m

		if !val.CreatedAt.IsZero() {
			m, err = prometheus.NewConstMetric(c.createdDesc, prometheus.GaugeValue, float64(val.CreatedAt.Unix()), labels...)
			if err != nil {
				slog.Error("failed to create mr_created_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}

		if !val.DeletedAt.IsZero() {
			m, err = prometheus.NewConstMetric(c.deletionDesc, prometheus.GaugeValue, float64(val.DeletedAt.Unix()), labels...)
			if err != nil {
				slog.Error("failed to create mr_deletion_timestamp metric", "error", err)
				continue
			}
			ch <- m
		}
	}
}

func (c *MRCollector) labelValues(key mrAggKey) []string {
	labels := []string{key.Group, key.Kind, key.Version, key.Namespace, key.Name, key.XRName}
	if c.includeXRParent {
		labels = append(labels, key.XRKind, key.XRNamespace, key.XRGroup)
	}
	if c.includeOwnerXR {
		labels = append(labels, key.OwnerXRName, key.OwnerXRKind, key.OwnerXRNamespace, key.OwnerXRGroup)
	}
	labels = append(labels,
		key.ClaimName, key.ClaimNS,
		key.Provider, key.ProviderConfig, key.ExternalName, key.ManagementPolicies,
		key.Synced, key.Ready, key.Reason, key.Paused, key.Deleting,
	)
	return labels
}
