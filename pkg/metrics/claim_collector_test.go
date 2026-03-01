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

	if totalFam != nil && len(totalFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_claims_total metrics, got %d", len(totalFam.GetMetric()))
	}
	if readyFam != nil && len(readyFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_claims_ready metrics, got %d", len(readyFam.GetMetric()))
	}
}

func TestClaimCollector_SingleGroup(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Creator: "alice", Team: "backend", Composition: "comp-a", Source: "central", Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "b", Creator: "alice", Team: "backend", Composition: "comp-a", Source: "central", Ready: false},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "c", Creator: "alice", Team: "backend", Composition: "comp-a", Source: "central", Ready: true},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	// All 3 claims share the same label tuple -> 1 sample per metric family.
	totalFam := families["crossplane_claims_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_claims_total")
	}
	if len(totalFam.GetMetric()) != 1 {
		t.Fatalf("expected 1 total sample, got %d", len(totalFam.GetMetric()))
	}
	if got := totalFam.GetMetric()[0].GetGauge().GetValue(); got != 3 {
		t.Errorf("crossplane_claims_total: expected 3, got %v", got)
	}

	readyFam := families["crossplane_claims_ready"]
	if readyFam == nil {
		t.Fatal("missing crossplane_claims_ready")
	}
	if got := readyFam.GetMetric()[0].GetGauge().GetValue(); got != 2 {
		t.Errorf("crossplane_claims_ready: expected 2, got %v", got)
	}

	// Verify labels.
	labels := labelMap(totalFam.GetMetric()[0])
	assertLabel(t, labels, "group", "g")
	assertLabel(t, labels, "kind", "Thing")
	assertLabel(t, labels, "namespace", "ns1")
	assertLabel(t, labels, "composition", "comp-a")
	assertLabel(t, labels, "creator", "alice")
	assertLabel(t, labels, "team", "backend")
	assertLabel(t, labels, "source", "central")
}

func TestClaimCollector_MultipleGroups(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Source: "central", Ready: true},
	})
	s.ReplaceClaims("g/v1/widgets", []store.ClaimInfo{
		{GVR: "g/v1/widgets", Group: "g", Kind: "Widget", Namespace: "ns2", Name: "b", Source: "central", Ready: false},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	// 2 distinct label tuples -> 2 samples per metric family.
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
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Source: "central"},
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
	assertLabel(t, labels, "composition", "")
	assertLabel(t, labels, "source", "central")
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
	if count != 2 {
		t.Fatalf("expected 2 descriptors, got %d", count)
	}
}

func TestClaimCollector_ReadySubsetOfTotal(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Source: "central", Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "b", Source: "central", Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "c", Source: "central", Ready: false},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "d", Source: "central", Ready: false},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "e", Source: "central", Ready: false},
	})

	c := NewClaimCollector(s)
	families := gatherCollector(t, c)

	total := families["crossplane_claims_total"].GetMetric()[0].GetGauge().GetValue()
	ready := families["crossplane_claims_ready"].GetMetric()[0].GetGauge().GetValue()

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
