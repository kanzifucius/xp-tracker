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
