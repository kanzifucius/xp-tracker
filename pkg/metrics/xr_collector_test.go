package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestXRCollector_Empty(t *testing.T) {
	s := store.New()
	c := NewXRCollector(s)

	families := gatherCollector(t, c)
	totalFam := families["crossplane_xr_total"]
	readyFam := families["crossplane_xr_ready"]

	if totalFam != nil && len(totalFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_xr_total metrics, got %d", len(totalFam.GetMetric()))
	}
	if readyFam != nil && len(readyFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_xr_ready metrics, got %d", len(readyFam.GetMetric()))
	}
}

func TestXRCollector_SingleComposition(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", Composition: "comp-prod", Source: "central", Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", Composition: "comp-prod", Source: "central", Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr3", Composition: "comp-prod", Source: "central", Ready: false},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_xr_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_xr_total")
	}
	if len(totalFam.GetMetric()) != 1 {
		t.Fatalf("expected 1 total sample, got %d", len(totalFam.GetMetric()))
	}
	if got := totalFam.GetMetric()[0].GetGauge().GetValue(); got != 3 {
		t.Errorf("crossplane_xr_total: expected 3, got %v", got)
	}

	readyFam := families["crossplane_xr_ready"]
	if readyFam == nil {
		t.Fatal("missing crossplane_xr_ready")
	}
	if got := readyFam.GetMetric()[0].GetGauge().GetValue(); got != 2 {
		t.Errorf("crossplane_xr_ready: expected 2, got %v", got)
	}

	labels := labelMap(totalFam.GetMetric()[0])
	assertLabel(t, labels, "group", "g")
	assertLabel(t, labels, "kind", "XThing")
	assertLabel(t, labels, "namespace", "")
	assertLabel(t, labels, "composition", "comp-prod")
	assertLabel(t, labels, "source", "central")
}

func TestXRCollector_MultipleCompositions(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", Composition: "comp-prod", Source: "central", Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", Composition: "comp-dev", Source: "central", Ready: false},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_xr_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_xr_total")
	}
	if len(totalFam.GetMetric()) != 2 {
		t.Fatalf("expected 2 total samples (one per composition), got %d", len(totalFam.GetMetric()))
	}
}

func TestXRCollector_Describe(t *testing.T) {
	s := store.New()
	c := NewXRCollector(s)

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

func TestXRCollector_AllReady(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", Composition: "comp", Source: "central", Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", Composition: "comp", Source: "central", Ready: true},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	total := families["crossplane_xr_total"].GetMetric()[0].GetGauge().GetValue()
	ready := families["crossplane_xr_ready"].GetMetric()[0].GetGauge().GetValue()

	if total != ready {
		t.Errorf("all XRs ready: total=%v, ready=%v should be equal", total, ready)
	}
}

func TestXRCollector_NoneReady(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", Composition: "comp", Source: "central", Ready: false},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", Composition: "comp", Source: "central", Ready: false},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	total := families["crossplane_xr_total"].GetMetric()[0].GetGauge().GetValue()
	ready := families["crossplane_xr_ready"].GetMetric()[0].GetGauge().GetValue()

	if total != 2 {
		t.Errorf("total: expected 2, got %v", total)
	}
	if ready != 0 {
		t.Errorf("ready: expected 0, got %v", ready)
	}
}
