package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestClaimCollector_Performance1000(t *testing.T) {
	s := store.New()

	// Generate 1000 claims spread across 10 namespaces, 5 compositions, 10 creators, 5 teams.
	claims := make([]store.ClaimInfo, 1000)
	for i := range claims {
		claims[i] = store.ClaimInfo{
			GVR:         "example.org/v1alpha1/things",
			Group:       "example.org",
			Kind:        "Thing",
			Namespace:   fmt.Sprintf("ns-%d", i%10),
			Name:        fmt.Sprintf("claim-%d", i),
			Creator:     fmt.Sprintf("user-%d", i%10),
			Team:        fmt.Sprintf("team-%d", i%5),
			Composition: fmt.Sprintf("comp-%d", i%5),
			Ready:       i%3 != 0, // ~66% ready
			Reason:      "Available",
			CreatedAt:   time.Now(),
		}
	}
	s.ReplaceClaims("example.org/v1alpha1/things", claims)

	c := NewClaimCollector(s)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	start := time.Now()
	families, err := reg.Gather()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	if elapsed > time.Second {
		t.Errorf("Collect took %v, expected < 1s", elapsed)
	}

	// With 10 namespaces x 5 compositions x 10 creators x 5 teams = up to 2500 label tuples,
	// but actual count depends on modular distribution. Verify we got results.
	totalFam := findFamily(families, "crossplane_claims_total")
	if totalFam == nil {
		t.Fatal("missing crossplane_claims_total")
	}
	if len(totalFam.GetMetric()) == 0 {
		t.Error("expected at least 1 metric sample")
	}

	t.Logf("Claim Collect: 1000 objects -> %d label tuples in %v", len(totalFam.GetMetric()), elapsed)
}

func TestXRCollector_Performance1000(t *testing.T) {
	s := store.New()

	// Generate 1000 XRs spread across 5 compositions and 10 kinds.
	xrs := make([]store.XRInfo, 1000)
	for i := range xrs {
		xrs[i] = store.XRInfo{
			GVR:         "example.org/v1alpha1/xthings",
			Group:       "example.org",
			Kind:        fmt.Sprintf("XThing%d", i%10),
			Namespace:   "", // cluster-scoped
			Name:        fmt.Sprintf("xr-%d", i),
			Composition: fmt.Sprintf("comp-%d", i%5),
			Ready:       i%4 != 0, // 75% ready
			Reason:      "Available",
			CreatedAt:   time.Now(),
		}
	}
	s.ReplaceXRs("example.org/v1alpha1/xthings", xrs)

	c := NewXRCollector(s)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	start := time.Now()
	families, err := reg.Gather()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	if elapsed > time.Second {
		t.Errorf("Collect took %v, expected < 1s", elapsed)
	}

	totalFam := findFamily(families, "crossplane_xr_total")
	if totalFam == nil {
		t.Fatal("missing crossplane_xr_total")
	}
	if len(totalFam.GetMetric()) == 0 {
		t.Error("expected at least 1 metric sample")
	}

	t.Logf("XR Collect: 1000 objects -> %d label tuples in %v", len(totalFam.GetMetric()), elapsed)
}

func findFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}
