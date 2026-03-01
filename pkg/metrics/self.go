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

	// S3PersistDuration tracks the duration of S3 persist operations.
	S3PersistDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "xp_tracker_s3_persist_duration_seconds",
		Help:    "Duration of S3 snapshot persist operations in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// NamespaceConfigs reports the number of active per-namespace ConfigMaps.
	NamespaceConfigs = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "xp_tracker_namespace_configs",
		Help: "Current number of active per-namespace ConfigMap configurations.",
	})
)

// RegisterSelfMetrics registers all self-monitoring metrics with the given
// Prometheus registry.
func RegisterSelfMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		PollDuration,
		PollErrors,
		StoreClaims,
		StoreXRs,
		S3PersistDuration,
		NamespaceConfigs,
	)
}
