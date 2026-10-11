package metrics

import (
	"testing"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestPackageCollector_Empty(t *testing.T) {
	s := store.New()
	c := NewPackageCollector(s)

	families := gatherCollector(t, c)
	for _, name := range []string{
		"crossplane_packages_total",
		"crossplane_packages_healthy",
		"crossplane_packages_installed",
	} {
		fam := families[name]
		if fam != nil && len(fam.GetMetric()) > 0 {
			t.Errorf("expected no %s metrics, got %d", name, len(fam.GetMetric()))
		}
	}
}

func TestPackageCollector_ProvidersAndFunctions(t *testing.T) {
	s := store.New()
	s.ReplacePackages("pkg.crossplane.io/v1/providers", []store.PackageInfo{
		{GVR: "pkg.crossplane.io/v1/providers", Group: "pkg.crossplane.io", Version: "v1", Kind: "Provider", Name: "provider-nop", Healthy: true, Installed: true},
		{GVR: "pkg.crossplane.io/v1/providers", Group: "pkg.crossplane.io", Version: "v1", Kind: "Provider", Name: "provider-aws", Healthy: false, Installed: true},
	})
	s.ReplacePackages("pkg.crossplane.io/v1beta1/functions", []store.PackageInfo{
		{GVR: "pkg.crossplane.io/v1beta1/functions", Group: "pkg.crossplane.io", Version: "v1beta1", Kind: "Function", Name: "function-patch-and-transform", Healthy: true, Installed: true},
	})

	c := NewPackageCollector(s)
	families := gatherCollector(t, c)

	totalFam := families["crossplane_packages_total"]
	if totalFam == nil {
		t.Fatal("missing crossplane_packages_total")
	}
	if len(totalFam.GetMetric()) != 3 {
		t.Fatalf("expected 3 total samples, got %d", len(totalFam.GetMetric()))
	}

	var totalSum, healthySum, installedSum float64
	for _, metric := range totalFam.GetMetric() {
		totalSum += metric.GetGauge().GetValue()
	}
	for _, metric := range families["crossplane_packages_healthy"].GetMetric() {
		healthySum += metric.GetGauge().GetValue()
	}
	for _, metric := range families["crossplane_packages_installed"].GetMetric() {
		installedSum += metric.GetGauge().GetValue()
	}
	if totalSum != 3 {
		t.Errorf("packages_total sum: got %v, want 3", totalSum)
	}
	if healthySum != 2 {
		t.Errorf("packages_healthy sum: got %v, want 2", healthySum)
	}
	if installedSum != 3 {
		t.Errorf("packages_installed sum: got %v, want 3", installedSum)
	}

	nopLabels := findLabelsByLabelValue(t, totalFam.GetMetric(), "name", "provider-nop")
	assertLabel(t, nopLabels, "kind", "Provider")
	assertLabel(t, nopLabels, "healthy", "true")
	assertLabel(t, nopLabels, "installed", "true")

	awsLabels := findLabelsByLabelValue(t, totalFam.GetMetric(), "name", "provider-aws")
	assertLabel(t, awsLabels, "kind", "Provider")
	assertLabel(t, awsLabels, "healthy", "false")
	assertLabel(t, awsLabels, "installed", "true")

	fnLabels := findLabelsByLabelValue(t, totalFam.GetMetric(), "name", "function-patch-and-transform")
	assertLabel(t, fnLabels, "kind", "Function")
	assertLabel(t, fnLabels, "healthy", "true")
	assertLabel(t, fnLabels, "installed", "true")
}

func TestPackageCollector_LabelSetIsSmall(t *testing.T) {
	s := store.New()
	s.ReplacePackages("pkg.crossplane.io/v1/providers", []store.PackageInfo{
		{GVR: "pkg.crossplane.io/v1/providers", Kind: "Provider", Name: "p1", Healthy: true, Installed: true},
	})

	families := gatherCollector(t, NewPackageCollector(s))
	totalFam := families["crossplane_packages_total"]
	if totalFam == nil || len(totalFam.GetMetric()) == 0 {
		t.Fatal("missing crossplane_packages_total sample")
	}
	labels := labelMap(totalFam.GetMetric()[0])
	if len(labels) != 4 {
		t.Fatalf("expected exactly 4 labels, got %d (%v)", len(labels), labels)
	}
	for _, key := range []string{"kind", "name", "healthy", "installed"} {
		if _, ok := labels[key]; !ok {
			t.Errorf("missing expected label %q", key)
		}
	}
}
