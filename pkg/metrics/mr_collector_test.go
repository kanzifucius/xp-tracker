package metrics

import (
	"testing"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestMRCollector_Empty(t *testing.T) {
	s := store.New()
	c := NewMRCollector(s)

	families := gatherCollector(t, c)
	totalFam := families["crossplane_mr_total"]
	if totalFam != nil && len(totalFam.GetMetric()) > 0 {
		t.Errorf("expected no crossplane_mr_total metrics, got %d", len(totalFam.GetMetric()))
	}
}

func TestMRCollector_WithData(t *testing.T) {
	s := store.New()
	s.ReplaceMRs("nop.crossplane.io/v1alpha1/nopresources", []store.MRInfo{
		{
			GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Kind: "NopResource",
			Namespace: "default", Name: "nop-1", XRName: "xr-1",
			ClaimName: "widget-a", ClaimNS: "team-alpha",
			Provider: "provider-nop", ProviderConfig: "default",
			Synced: true, Ready: true,
		},
		{
			GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Kind: "NopResource",
			Namespace: "default", Name: "nop-2", XRName: "xr-2",
			ClaimName: "widget-b", ClaimNS: "team-alpha",
			Provider: "provider-nop", ProviderConfig: "default",
			Synced: true, Ready: false,
		},
	})

	c := NewMRCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_mr_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_mr_total")
	}
	if len(totalFam.GetMetric()) != 2 {
		t.Fatalf("expected 2 total samples, got %d", len(totalFam.GetMetric()))
	}

	readyFam := families["crossplane_mr_ready"]
	if readyFam == nil {
		t.Fatal("missing crossplane_mr_ready")
	}
	var readySum float64
	for _, metric := range readyFam.GetMetric() {
		readySum += metric.GetGauge().GetValue()
	}
	if readySum != 1 {
		t.Errorf("crossplane_mr_ready sum: expected 1, got %v", readySum)
	}

	labels := findLabelsByLabelValue(t, totalFam.GetMetric(), "name", "nop-1")
	assertLabel(t, labels, "group", "nop.crossplane.io")
	assertLabel(t, labels, "kind", "NopResource")
	assertLabel(t, labels, "xr_name", "xr-1")
	assertLabel(t, labels, "claim_name", "widget-a")
	assertLabel(t, labels, "claim_namespace", "team-alpha")
	assertLabel(t, labels, "provider", "provider-nop")
	assertLabel(t, labels, "provider_config", "default")
	assertLabel(t, labels, "synced", "true")
	assertLabel(t, labels, "ready", "true")
}
