package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

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
	createdAt := time.Unix(1700000000, 0).UTC()
	s := store.New()
	s.ReplaceMRs("nop.crossplane.io/v1alpha1/nopresources", []store.MRInfo{
		{
			GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Version: "v1alpha1", Kind: "NopResource",
			Namespace: "default", Name: "nop-1", XRName: "xr-1",
			ClaimName: "widget-a", ClaimNS: "team-alpha",
			Provider: "provider-nop", ProviderConfig: "default", ExternalName: "cloud-nop-1",
			ManagementPolicies: "*", Reason: "Available", CreatedAt: createdAt,
			Synced: true, Ready: true,
		},
		{
			GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Version: "v1alpha1", Kind: "NopResource",
			Namespace: "default", Name: "nop-2", XRName: "xr-2",
			ClaimName: "widget-b", ClaimNS: "team-alpha",
			Provider: "provider-nop", ProviderConfig: "default",
			ManagementPolicies: "Observe", Reason: "Creating", CreatedAt: createdAt,
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
	assertLabel(t, labels, "version", "v1alpha1")
	assertLabel(t, labels, "xr_name", "xr-1")
	assertLabel(t, labels, "claim_name", "widget-a")
	assertLabel(t, labels, "claim_namespace", "team-alpha")
	assertLabel(t, labels, "provider", "provider-nop")
	assertLabel(t, labels, "provider_config", "default")
	assertLabel(t, labels, "external_name", "cloud-nop-1")
	assertLabel(t, labels, "management_policies", "*")
	assertLabel(t, labels, "synced", "true")
	assertLabel(t, labels, "ready", "true")
	assertLabel(t, labels, "reason", "Available")
	assertLabel(t, labels, "paused", "false")
	assertLabel(t, labels, "deleting", "false")

	createdFam := families["crossplane_mr_created_timestamp_seconds"]
	if createdFam == nil || len(createdFam.GetMetric()) != 2 {
		t.Fatalf("expected 2 created timestamp samples, got %v", createdFam)
	}
	if families["crossplane_mr_deletion_timestamp_seconds"] != nil {
		t.Error("expected no deletion timestamp samples when nothing is deleting")
	}
}

func TestMRCollector_Describe(t *testing.T) {
	s := store.New()
	c := NewMRCollector(s)

	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)

	var count int
	for range ch {
		count++
	}
	if count != 6 {
		t.Fatalf("expected 6 descriptors, got %d", count)
	}
}

func TestMRCollector_Deleting(t *testing.T) {
	createdAt := time.Unix(1700000000, 0).UTC()
	deletedAt := time.Unix(1700001800, 0).UTC()

	s := store.New()
	s.ReplaceMRs("nop.crossplane.io/v1alpha1/nopresources", []store.MRInfo{
		{
			GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Version: "v1alpha1", Kind: "NopResource",
			Namespace: "default", Name: "nop-dying", XRName: "xr-1",
			Provider: "provider-nop", ManagementPolicies: "Observe",
			Paused: true, Synced: false, Ready: false, Reason: "Deleting",
			CreatedAt: createdAt, DeletedAt: deletedAt,
		},
	})

	c := NewMRCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_mr_total"]
	labels := findLabelsByLabelValue(t, totalFam.GetMetric(), "name", "nop-dying")
	assertLabel(t, labels, "deleting", "true")
	assertLabel(t, labels, "paused", "true")
	assertLabel(t, labels, "management_policies", "Observe")

	deletionFam := families["crossplane_mr_deletion_timestamp_seconds"]
	if deletionFam == nil || len(deletionFam.GetMetric()) != 1 {
		t.Fatalf("expected 1 deletion timestamp sample, got %v", deletionFam)
	}
	if got := deletionFam.GetMetric()[0].GetGauge().GetValue(); got != float64(deletedAt.Unix()) {
		t.Errorf("deletion timestamp: got %v, want %v", got, deletedAt.Unix())
	}
}
