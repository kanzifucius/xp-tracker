package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegisterSelfMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Should not panic.
	RegisterSelfMetrics(reg)

	// Initialise the counter vec so it appears in Gather output.
	PollErrors.WithLabelValues("test-register").Add(0)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("unexpected gather error: %v", err)
	}

	want := map[string]bool{
		"xp_tracker_poll_duration_seconds":       false,
		"xp_tracker_poll_errors_total":           false,
		"xp_tracker_store_claims":                false,
		"xp_tracker_store_xrs":                   false,
		"xp_tracker_s3_persist_duration_seconds": false,
	}

	for _, fam := range families {
		if _, ok := want[fam.GetName()]; ok {
			want[fam.GetName()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("expected metric family %q not found in gathered output", name)
		}
	}
}

func TestSelfMetricsUpdate(t *testing.T) {
	// Verify that updating the global metrics doesn't panic and produces
	// non-zero values when gathered.
	reg := prometheus.NewRegistry()
	RegisterSelfMetrics(reg)

	StoreClaims.Set(42)
	StoreXRs.Set(7)
	PollDuration.Observe(1.5)
	PollErrors.WithLabelValues("test.example.com/v1/widgets").Inc()
	S3PersistDuration.Observe(0.25)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("unexpected gather error: %v", err)
	}

	// Build a lookup for quick assertions.
	gauges := make(map[string]float64)
	for _, fam := range families {
		switch fam.GetName() {
		case "xp_tracker_store_claims":
			gauges["claims"] = fam.GetMetric()[0].GetGauge().GetValue()
		case "xp_tracker_store_xrs":
			gauges["xrs"] = fam.GetMetric()[0].GetGauge().GetValue()
		}
	}

	if got := gauges["claims"]; got != 42 {
		t.Errorf("store_claims = %v, want 42", got)
	}
	if got := gauges["xrs"]; got != 7 {
		t.Errorf("store_xrs = %v, want 7", got)
	}
}
