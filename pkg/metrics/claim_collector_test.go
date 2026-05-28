package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestClaimCollector_Empty(t *testing.T) {
	s := store.New()
	c := NewClaimCollector(s)

	families := gatherCollector(t, c)
	// With an empty store, no metrics should be emitted.
	totalFam := families["crossplane_claims_total"]
	readyFam := families["crossplane_claims_ready"]
	statusSyncedFam := families["crossplane_claims_status_synced"]
	statusReadyFam := families["crossplane_claims_status_ready"]

	if totalFam != nil && len(totalFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_claims_total metrics, got %d", len(totalFam.GetMetric()))
	}
	if readyFam != nil && len(readyFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_claims_ready metrics, got %d", len(readyFam.GetMetric()))
	}
	if statusSyncedFam != nil && len(statusSyncedFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_claims_status_synced metrics, got %d", len(statusSyncedFam.GetMetric()))
	}
	if statusReadyFam != nil && len(statusReadyFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_claims_status_ready metrics, got %d", len(statusReadyFam.GetMetric()))
	}
}

func TestClaimCollector_SingleGroup(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Creator: "alice", Team: "backend", Composition: "comp-a", Synced: true, Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "b", Creator: "alice", Team: "backend", Composition: "comp-a", Synced: false, Ready: false},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "c", Creator: "alice", Team: "backend", Composition: "comp-a", Synced: true, Ready: true},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	// claim_name and status labels create one sample per claim.
	totalFam := families["crossplane_claims_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_claims_total")
	}
	if len(totalFam.GetMetric()) != 3 {
		t.Fatalf("expected 3 total samples, got %d", len(totalFam.GetMetric()))
	}
	for _, metric := range totalFam.GetMetric() {
		if got := metric.GetGauge().GetValue(); got != 1 {
			t.Errorf("crossplane_claims_total: expected 1 per claim, got %v", got)
		}
	}

	readyFam := families["crossplane_claims_ready"]
	if readyFam == nil {
		t.Fatal("missing crossplane_claims_ready")
	}
	if len(readyFam.GetMetric()) != 3 {
		t.Fatalf("expected 3 ready samples, got %d", len(readyFam.GetMetric()))
	}
	var readySum float64
	for _, metric := range readyFam.GetMetric() {
		readySum += metric.GetGauge().GetValue()
	}
	if readySum != 2 {
		t.Errorf("crossplane_claims_ready sum: expected 2, got %v", readySum)
	}

	// Verify labels for claim "a".
	labels := findLabelsByLabelValue(t, totalFam.GetMetric(), "claim_name", "a")
	assertLabel(t, labels, "group", "g")
	assertLabel(t, labels, "kind", "Thing")
	assertLabel(t, labels, "namespace", "ns1")
	assertLabel(t, labels, "creator", "alice")
	assertLabel(t, labels, "team", "backend")
	assertLabel(t, labels, "claim_name", "a")
	assertLabel(t, labels, "synced", "true")
	assertLabel(t, labels, "ready", "true")

	statusSyncedFam := families["crossplane_claims_status_synced"]
	if statusSyncedFam == nil {
		t.Fatal("missing crossplane_claims_status_synced")
	}
	var syncedSum float64
	for _, metric := range statusSyncedFam.GetMetric() {
		syncedSum += metric.GetGauge().GetValue()
	}
	if syncedSum != 2 {
		t.Errorf("crossplane_claims_status_synced sum: expected 2, got %v", syncedSum)
	}
}

func TestClaimCollector_MultipleGroups(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Synced: true, Ready: true},
	})
	s.ReplaceClaims("g/v1/widgets", []store.ClaimInfo{
		{GVR: "g/v1/widgets", Group: "g", Kind: "Widget", Namespace: "ns2", Name: "b", Synced: false, Ready: false},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	// 2 claims -> 2 samples per metric family.
	totalFam := families["crossplane_claims_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_claims_total")
	}
	if len(totalFam.GetMetric()) != 2 {
		t.Fatalf("expected 2 total samples, got %d", len(totalFam.GetMetric()))
	}
}

func TestClaimCollector_EmptyLabels(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a"},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_claims_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_claims_total")
	}

	labels := labelMap(totalFam.GetMetric()[0])
	assertLabel(t, labels, "creator", "")
	assertLabel(t, labels, "team", "")
	assertLabel(t, labels, "claim_name", "a")
	assertLabel(t, labels, "synced", "false")
	assertLabel(t, labels, "ready", "false")
}

func TestClaimCollector_Describe(t *testing.T) {
	s := store.New()
	c := NewClaimCollector(s)

	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)

	var count int
	for range ch {
		count++
	}
	if count != 4 {
		t.Fatalf("expected 4 descriptors, got %d", count)
	}
}

func TestClaimCollector_ReadySubsetOfTotal(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Synced: true, Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "b", Synced: true, Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "c", Synced: false, Ready: false},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "d", Synced: false, Ready: false},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "e", Synced: true, Ready: false},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	var total float64
	for _, metric := range families["crossplane_claims_total"].GetMetric() {
		total += metric.GetGauge().GetValue()
	}
	var ready float64
	for _, metric := range families["crossplane_claims_ready"].GetMetric() {
		ready += metric.GetGauge().GetValue()
	}

	if total != 5 {
		t.Errorf("total: expected 5, got %v", total)
	}
	if ready != 2 {
		t.Errorf("ready: expected 2, got %v", ready)
	}
	if ready > total {
		t.Error("ready should never exceed total")
	}
}

func findLabelsByLabelValue(t *testing.T, metrics []*dto.Metric, name, value string) map[string]string {
	t.Helper()
	for _, metric := range metrics {
		labels := labelMap(metric)
		if labels[name] == value {
			return labels
		}
	}
	t.Fatalf("could not find metric with label %q=%q", name, value)
	return nil
}

// --- helpers ---

// gatherCollector registers a collector in a fresh registry and gathers all metric families.
func gatherCollector(t *testing.T, c prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	out := make(map[string]*dto.MetricFamily)
	for _, f := range families {
		out[f.GetName()] = f
	}
	return out
}

// labelMap extracts label name->value pairs from a dto.Metric.
func labelMap(m *dto.Metric) map[string]string {
	out := make(map[string]string)
	for _, lp := range m.GetLabel() {
		out[lp.GetName()] = lp.GetValue()
	}
	return out
}

// assertLabel checks that a label has the expected value.
func assertLabel(t *testing.T, labels map[string]string, name, want string) {
	t.Helper()
	if got := labels[name]; got != want {
		t.Errorf("label %q: got %q, want %q", name, got, want)
	}
}
