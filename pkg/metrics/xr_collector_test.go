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
	statusSyncedFam := families["crossplane_xr_status_synced"]
	statusReadyFam := families["crossplane_xr_status_ready"]

	if totalFam != nil && len(totalFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_xr_total metrics, got %d", len(totalFam.GetMetric()))
	}
	if readyFam != nil && len(readyFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_xr_ready metrics, got %d", len(readyFam.GetMetric()))
	}
	if statusSyncedFam != nil && len(statusSyncedFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_xr_status_synced metrics, got %d", len(statusSyncedFam.GetMetric()))
	}
	if statusReadyFam != nil && len(statusReadyFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_xr_status_ready metrics, got %d", len(statusReadyFam.GetMetric()))
	}
}

func TestXRCollector_SingleComposition(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", ClaimName: "claim-a", ClaimNS: "ns-a", Composition: "comp-prod", Synced: true, Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", ClaimName: "claim-b", ClaimNS: "ns-a", Composition: "comp-prod", Synced: true, Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr3", ClaimName: "claim-c", ClaimNS: "ns-b", Composition: "comp-prod", Synced: false, Ready: false},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_xr_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_xr_total")
	}
	if len(totalFam.GetMetric()) != 3 {
		t.Fatalf("expected 3 total samples, got %d", len(totalFam.GetMetric()))
	}
	for _, metric := range totalFam.GetMetric() {
		if got := metric.GetGauge().GetValue(); got != 1 {
			t.Errorf("crossplane_xr_total: expected 1 per XR, got %v", got)
		}
	}

	readyFam := families["crossplane_xr_ready"]
	if readyFam == nil {
		t.Fatal("missing crossplane_xr_ready")
	}
	if len(readyFam.GetMetric()) != 3 {
		t.Fatalf("expected 3 ready samples, got %d", len(readyFam.GetMetric()))
	}
	var readySum float64
	for _, metric := range readyFam.GetMetric() {
		readySum += metric.GetGauge().GetValue()
	}
	if readySum != 2 {
		t.Errorf("crossplane_xr_ready sum: expected 2, got %v", readySum)
	}

	labels := findLabelsByLabelValue(t, totalFam.GetMetric(), "name", "xr1")
	assertLabel(t, labels, "group", "g")
	assertLabel(t, labels, "kind", "XThing")
	assertLabel(t, labels, "namespace", "")
	assertLabel(t, labels, "name", "xr1")
	assertLabel(t, labels, "claim_name", "claim-a")
	assertLabel(t, labels, "claim_namespace", "ns-a")
	assertLabel(t, labels, "synced", "true")
	assertLabel(t, labels, "ready", "true")
}

func TestXRCollector_MultipleCompositions(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", ClaimName: "claim-a", ClaimNS: "ns-a", Composition: "comp-prod", Synced: true, Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", ClaimName: "claim-b", ClaimNS: "ns-b", Composition: "comp-dev", Synced: false, Ready: false},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_xr_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_xr_total")
	}
	if len(totalFam.GetMetric()) != 2 {
		t.Fatalf("expected 2 total samples (one per XR), got %d", len(totalFam.GetMetric()))
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
	if count != 4 {
		t.Fatalf("expected 4 descriptors, got %d", count)
	}
}

func TestXRCollector_AllReady(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", ClaimName: "claim-a", ClaimNS: "ns-a", Composition: "comp", Synced: true, Ready: true},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", ClaimName: "claim-b", ClaimNS: "ns-a", Composition: "comp", Synced: true, Ready: true},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	var total float64
	for _, metric := range families["crossplane_xr_total"].GetMetric() {
		total += metric.GetGauge().GetValue()
	}
	var ready float64
	for _, metric := range families["crossplane_xr_ready"].GetMetric() {
		ready += metric.GetGauge().GetValue()
	}

	if total != ready {
		t.Errorf("all XRs ready: total=%v, ready=%v should be equal", total, ready)
	}
}

func TestXRCollector_NoneReady(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", ClaimName: "claim-a", ClaimNS: "ns-a", Composition: "comp", Synced: false, Ready: false},
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr2", ClaimName: "claim-b", ClaimNS: "ns-a", Composition: "comp", Synced: false, Ready: false},
	})

	c := NewXRCollector(s)
	families := gatherCollector(t, c)

	var total float64
	for _, metric := range families["crossplane_xr_total"].GetMetric() {
		total += metric.GetGauge().GetValue()
	}
	var ready float64
	for _, metric := range families["crossplane_xr_ready"].GetMetric() {
		ready += metric.GetGauge().GetValue()
	}

	if total != 2 {
		t.Errorf("total: expected 2, got %v", total)
	}
	if ready != 0 {
		t.Errorf("ready: expected 0, got %v", ready)
	}
}
