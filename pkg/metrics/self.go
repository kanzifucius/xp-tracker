package metrics

import "github.com/prometheus/client_golang/prometheus"

// Self-monitoring metrics for the exporter itself. These use the
// "xp_tracker_" prefix to distinguish them from the crossplane_* business
// metrics.
//
// All metrics are pre-registered via RegisterSelfMetrics and updated
// imperatively by the poller and S3 store code.
var (
	// PollDuration tracks the duration of each polling cycle.
	PollDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "xp_tracker_poll_duration_seconds",
		Help:    "Duration of a complete polling cycle in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// PollErrors counts polling errors per GVR.
	PollErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "xp_tracker_poll_errors_total",
		Help: "Total number of polling errors, partitioned by GVR.",
	}, []string{"gvr"})

	// StoreClaims reports the number of claims currently held in the store.
	StoreClaims = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "xp_tracker_store_claims",
		Help: "Current number of claims in the in-memory store.",
	})

	// StoreXRs reports the number of XRs currently held in the store.
	StoreXRs = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "xp_tracker_store_xrs",
		Help: "Current number of XRs in the in-memory store.",
	})

	// StoreMRs reports the number of MRs currently held in the store.
	StoreMRs = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "xp_tracker_store_mrs",
		Help: "Current number of provider MRs in the in-memory store.",
	})

	// S3PersistDuration tracks the duration of S3 persist operations.
	S3PersistDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "xp_tracker_s3_persist_duration_seconds",
		Help:    "Duration of S3 snapshot persist operations in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// MRDDiscoverySkipped counts Active ManagedResourceDefinitions skipped
	// during startup MR GVR discovery because their spec could not yield a
	// usable group/version/resource (for example, no storage or served
	// version). Labelled by short reason so operators can alert on cluster
	// data-quality problems without taking the exporter down.
	MRDDiscoverySkipped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "xp_tracker_mrd_discovery_skipped_total",
		Help: "Total number of Active ManagedResourceDefinitions skipped during MR GVR discovery because their spec was unusable.",
	}, []string{"reason"})
)

// Known reason label values for MRDDiscoverySkipped. Pre-initialised at
// registration so the series exists at 0 and alerts on > 0 work immediately.
const (
	MRDSkipReasonMissingGroup             = "missing_group"
	MRDSkipReasonMissingPlural            = "missing_plural"
	MRDSkipReasonMissingVersions          = "missing_versions"
	MRDSkipReasonNoStorageOrServedVersion = "no_storage_or_served_version"
	MRDSkipReasonOther                    = "other"
)

// RegisterSelfMetrics registers all self-monitoring metrics with the given
// Prometheus registry.
func RegisterSelfMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		PollDuration,
		PollErrors,
		StoreClaims,
		StoreXRs,
		StoreMRs,
		S3PersistDuration,
		MRDDiscoverySkipped,
	)

	// Pre-create the known reason series at 0.
	for _, reason := range []string{
		MRDSkipReasonMissingGroup,
		MRDSkipReasonMissingPlural,
		MRDSkipReasonMissingVersions,
		MRDSkipReasonNoStorageOrServedVersion,
		MRDSkipReasonOther,
	} {
		MRDDiscoverySkipped.WithLabelValues(reason)
	}
}
