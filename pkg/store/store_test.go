package store

import (
	"sort"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New()
	if s.ClaimCount() != 0 {
		t.Errorf("expected 0 claims, got %d", s.ClaimCount())
	}
	if s.XRCount() != 0 {
		t.Errorf("expected 0 XRs, got %d", s.XRCount())
	}
}

func TestReplaceClaims(t *testing.T) {
	s := New()

	claims := []ClaimInfo{
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "a", Ready: true},
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "b", Ready: false},
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns2", Name: "c", Ready: true},
	}
	s.ReplaceClaims("g1/v1/k1s", claims)

	if s.ClaimCount() != 3 {
		t.Fatalf("expected 3 claims, got %d", s.ClaimCount())
	}

	snap := s.SnapshotClaims()
	if len(snap) != 3 {
		t.Fatalf("expected 3 in snapshot, got %d", len(snap))
	}
}

func TestReplaceClaims_RemovesStale(t *testing.T) {
	s := New()

	s.ReplaceClaims("g1/v1/k1s", []ClaimInfo{
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "a"},
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "b"},
	})
	if s.ClaimCount() != 2 {
		t.Fatalf("expected 2, got %d", s.ClaimCount())
	}

	// Replace with smaller set — "b" should be removed.
	s.ReplaceClaims("g1/v1/k1s", []ClaimInfo{
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "a"},
	})
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 after replace, got %d", s.ClaimCount())
	}
}

func TestReplaceClaims_DifferentGVRs(t *testing.T) {
	s := New()

	s.ReplaceClaims("g1/v1/k1s", []ClaimInfo{
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "a"},
	})
	s.ReplaceClaims("g1/v1/k2s", []ClaimInfo{
		{GVR: "g1/v1/k2s", Group: "g1", Kind: "K2", Namespace: "ns1", Name: "x"},
	})

	if s.ClaimCount() != 2 {
		t.Fatalf("expected 2 claims across GVRs, got %d", s.ClaimCount())
	}

	// Replacing one GVR with empty should only remove that GVR's entries.
	s.ReplaceClaims("g1/v1/k1s", nil)
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim after removing g1/v1/k1s, got %d", s.ClaimCount())
	}

	snap := s.SnapshotClaims()
	if snap[0].Kind != "K2" {
		t.Errorf("expected K2, got %s", snap[0].Kind)
	}
}

func TestReplaceXRs(t *testing.T) {
	s := New()

	xrs := []XRInfo{
		{GVR: "g1/v1/xk1s", Group: "g1", Kind: "XK1", Name: "xr1", Composition: "comp-a", Ready: true},
		{GVR: "g1/v1/xk1s", Group: "g1", Kind: "XK1", Name: "xr2", Composition: "comp-b", Ready: false},
	}
	s.ReplaceXRs("g1/v1/xk1s", xrs)

	if s.XRCount() != 2 {
		t.Fatalf("expected 2 XRs, got %d", s.XRCount())
	}

	snap := s.SnapshotXRs()
	if len(snap) != 2 {
		t.Fatalf("expected 2 in snapshot, got %d", len(snap))
	}
}

func TestReplaceXRs_RemovesStale(t *testing.T) {
	s := New()

	s.ReplaceXRs("g1/v1/xk1s", []XRInfo{
		{GVR: "g1/v1/xk1s", Group: "g1", Kind: "XK1", Name: "xr1"},
		{GVR: "g1/v1/xk1s", Group: "g1", Kind: "XK1", Name: "xr2"},
	})
	s.ReplaceXRs("g1/v1/xk1s", []XRInfo{
		{GVR: "g1/v1/xk1s", Group: "g1", Kind: "XK1", Name: "xr1"},
	})

	if s.XRCount() != 1 {
		t.Fatalf("expected 1 XR after replace, got %d", s.XRCount())
	}
}

func TestEnrichClaimCompositions(t *testing.T) {
	s := New()

	// Add XRs first.
	s.ReplaceXRs("g1/v1/xpostgres", []XRInfo{
		{GVR: "g1/v1/xpostgres", Group: "g1", Kind: "XPostgreSQL", Name: "xr-abc", Composition: "comp-prod"},
		{GVR: "g1/v1/xpostgres", Group: "g1", Kind: "XPostgreSQL", Name: "xr-def", Composition: "comp-dev"},
	})

	// Add claims referencing XRs.
	s.ReplaceClaims("g1/v1/postgres", []ClaimInfo{
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db1", XRRef: "xr-abc"},
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db2", XRRef: "xr-def"},
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db3", XRRef: ""}, // no ref
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db4", XRRef: "xr-missing"},
	})

	s.EnrichClaimCompositions()

	snap := s.SnapshotClaims()
	byName := make(map[string]ClaimInfo)
	for _, c := range snap {
		byName[c.Name] = c
	}

	if byName["db1"].Composition != "comp-prod" {
		t.Errorf("db1: expected comp-prod, got %q", byName["db1"].Composition)
	}
	if byName["db2"].Composition != "comp-dev" {
		t.Errorf("db2: expected comp-dev, got %q", byName["db2"].Composition)
	}
	if byName["db3"].Composition != "" {
		t.Errorf("db3: expected empty composition, got %q", byName["db3"].Composition)
	}
	if byName["db4"].Composition != "" {
		t.Errorf("db4: expected empty composition (missing XR), got %q", byName["db4"].Composition)
	}
}

func TestEnrichXRClaims(t *testing.T) {
	s := New()

	s.ReplaceXRs("g1/v1/xpostgres", []XRInfo{
		{GVR: "g1/v1/xpostgres", Group: "g1", Kind: "XPostgreSQL", Name: "xr-abc"},
		{GVR: "g1/v1/xpostgres", Group: "g1", Kind: "XPostgreSQL", Name: "xr-def", ClaimName: "label-claim", ClaimNS: "label-ns"},
		{GVR: "g1/v1/xpostgres", Group: "g1", Kind: "XPostgreSQL", Name: "xr-orphan"},
	})

	s.ReplaceClaims("g1/v1/postgres", []ClaimInfo{
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db1", XRRef: "xr-abc"},
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns2", Name: "db2", XRRef: "xr-def"},
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db3", XRRef: ""},
		{GVR: "g1/v1/postgres", Group: "g1", Kind: "PostgreSQL", Namespace: "ns1", Name: "db4", XRRef: "xr-missing"},
	})

	s.EnrichXRClaims()

	snap := s.SnapshotXRs()
	byName := make(map[string]XRInfo)
	for _, x := range snap {
		byName[x.Name] = x
	}

	if byName["xr-abc"].ClaimName != "db1" {
		t.Errorf("xr-abc: expected claim name db1, got %q", byName["xr-abc"].ClaimName)
	}
	if byName["xr-abc"].ClaimNS != "ns1" {
		t.Errorf("xr-abc: expected claim namespace ns1, got %q", byName["xr-abc"].ClaimNS)
	}
	if byName["xr-def"].ClaimName != "label-claim" {
		t.Errorf("xr-def: expected label-derived claim name preserved, got %q", byName["xr-def"].ClaimName)
	}
	if byName["xr-def"].ClaimNS != "label-ns" {
		t.Errorf("xr-def: expected label-derived claim namespace preserved, got %q", byName["xr-def"].ClaimNS)
	}
	if byName["xr-orphan"].ClaimName != "" {
		t.Errorf("xr-orphan: expected empty claim name, got %q", byName["xr-orphan"].ClaimName)
	}
}

func TestReplaceMRs(t *testing.T) {
	s := New()
	s.ReplaceMRs("nop.crossplane.io/v1alpha1/nopresources", []MRInfo{
		{GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Kind: "NopResource", Namespace: "default", Name: "nop-1", XRName: "xr-1"},
	})
	if s.MRCount() != 1 {
		t.Fatalf("expected 1 MR, got %d", s.MRCount())
	}
}

func TestEnrichMRClaims(t *testing.T) {
	s := New()

	s.ReplaceXRs("g1/v1/xwidgets", []XRInfo{
		{GVR: "g1/v1/xwidgets", Group: "g1", Kind: "XWidget", Name: "xr-abc", ClaimName: "widget-a", ClaimNS: "team-alpha"},
		{GVR: "g1/v1/xwidgets", Group: "g1", Kind: "XWidget", Name: "xr-labeled", ClaimName: "direct-claim", ClaimNS: "direct-ns"},
	})

	s.ReplaceMRs("nop.crossplane.io/v1alpha1/nopresources", []MRInfo{
		{GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Kind: "NopResource", Namespace: "default", Name: "nop-1", XRName: "xr-abc"},
		{GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Kind: "NopResource", Namespace: "default", Name: "nop-2", XRName: "xr-labeled", ClaimName: "label-claim", ClaimNS: "label-ns"},
		{GVR: "nop.crossplane.io/v1alpha1/nopresources", Group: "nop.crossplane.io", Kind: "NopResource", Namespace: "default", Name: "nop-3", XRName: "xr-missing"},
	})

	s.EnrichMRClaims()

	snap := s.SnapshotMRs()
	byName := make(map[string]MRInfo)
	for _, m := range snap {
		byName[m.Name] = m
	}

	if byName["nop-1"].ClaimName != "widget-a" || byName["nop-1"].ClaimNS != "team-alpha" {
		t.Errorf("nop-1: expected claim widget-a/team-alpha, got %q/%q", byName["nop-1"].ClaimName, byName["nop-1"].ClaimNS)
	}
	if byName["nop-2"].ClaimName != "label-claim" || byName["nop-2"].ClaimNS != "label-ns" {
		t.Errorf("nop-2: expected label-derived claim preserved, got %q/%q", byName["nop-2"].ClaimName, byName["nop-2"].ClaimNS)
	}
	if byName["nop-3"].ClaimName != "" {
		t.Errorf("nop-3: expected empty claim name, got %q", byName["nop-3"].ClaimName)
	}
}

func TestSnapshotClaims_IsCopy(t *testing.T) {
	s := New()
	s.ReplaceClaims("g1/v1/k1s", []ClaimInfo{
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "a"},
	})

	snap := s.SnapshotClaims()
	snap[0].Name = "mutated"

	snap2 := s.SnapshotClaims()
	if snap2[0].Name != "a" {
		t.Errorf("snapshot mutation leaked: got %s", snap2[0].Name)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	now := time.Now()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claims := make([]ClaimInfo, 100)
			for j := range claims {
				claims[j] = ClaimInfo{
					GVR:       "g1/v1/k1s",
					Group:     "g1",
					Kind:      "K1",
					Namespace: "ns",
					Name:      "claim-" + time.Now().String(),
					Ready:     j%2 == 0,
					CreatedAt: now,
				}
			}
			s.ReplaceClaims("g1/v1/k1s", claims)
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.SnapshotClaims()
			_ = s.ClaimCount()
		}()
	}

	wg.Wait()
}

func TestSnapshotClaims_Deterministic(t *testing.T) {
	s := New()
	s.ReplaceClaims("g1/v1/k1s", []ClaimInfo{
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "c"},
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "a"},
		{GVR: "g1/v1/k1s", Group: "g1", Kind: "K1", Namespace: "ns1", Name: "b"},
	})

	snap := s.SnapshotClaims()
	sort.Slice(snap, func(i, j int) bool {
		return snap[i].Name < snap[j].Name
	})

	if snap[0].Name != "a" || snap[1].Name != "b" || snap[2].Name != "c" {
		t.Errorf("unexpected order: %v", snap)
	}
}
