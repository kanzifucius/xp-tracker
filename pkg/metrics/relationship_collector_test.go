package metrics

import (
	"testing"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestRelationshipCollector_DisabledEmitsNothing(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xnets", []store.XRInfo{
		{GVR: "g/v1/xnets", Group: "g", Kind: "XNetwork", Name: "net-1", ParentName: "app-1", ParentKind: "XApp", ParentGroup: "g"},
	})
	c := NewRelationshipCollector(s, false, false)
	families := gatherCollector(t, c)
	if families["crossplane_xr_child_info"] != nil {
		t.Fatal("expected no xr_child_info when disabled")
	}
	if families["crossplane_mr_owner_info"] != nil {
		t.Fatal("expected no mr_owner_info when disabled")
	}
}

func TestRelationshipCollector_XRChildInfo(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/xapps", []store.XRInfo{
		{GVR: "g/v1/xapps", Group: "g", Kind: "XApp", Namespace: "ns", Name: "app-1"},
	})
	s.ReplaceXRs("g/v1/xnets", []store.XRInfo{
		{
			GVR: "g/v1/xnets", Group: "g", Kind: "XNetwork", Namespace: "ns", Name: "net-1",
			ParentKind: "XApp", ParentName: "app-1", ParentNamespace: "ns", ParentGroup: "g",
		},
	})

	c := NewRelationshipCollector(s, true, false)
	families := gatherCollector(t, c)
	fam := families["crossplane_xr_child_info"]
	if fam == nil || len(fam.GetMetric()) != 1 {
		t.Fatalf("expected 1 xr_child_info sample, got %v", fam)
	}
	labels := findLabelsByLabelValue(t, fam.GetMetric(), "child_name", "net-1")
	assertLabel(t, labels, "parent_kind", "XApp")
	assertLabel(t, labels, "parent_name", "app-1")
	assertLabel(t, labels, "parent_namespace", "ns")
	assertLabel(t, labels, "parent_group", "g")
	assertLabel(t, labels, "child_kind", "XNetwork")
	assertLabel(t, labels, "child_namespace", "ns")
}

func TestRelationshipCollector_MROwnerInfo(t *testing.T) {
	s := store.New()
	s.ReplaceMRs("nop/v1/nops", []store.MRInfo{
		{
			GVR: "nop/v1/nops", Group: "nop", Kind: "NopResource", Namespace: "ns", Name: "nop-1",
			OwnerXRName: "net-1", OwnerXRKind: "XNetwork", OwnerXRNamespace: "ns", OwnerXRGroup: "g",
		},
		{GVR: "nop/v1/nops", Group: "nop", Kind: "NopResource", Namespace: "ns", Name: "nop-orphan"},
	})

	c := NewRelationshipCollector(s, false, true)
	families := gatherCollector(t, c)
	fam := families["crossplane_mr_owner_info"]
	if fam == nil || len(fam.GetMetric()) != 1 {
		t.Fatalf("expected 1 mr_owner_info sample, got %v", fam)
	}
	labels := findLabelsByLabelValue(t, fam.GetMetric(), "mr_name", "nop-1")
	assertLabel(t, labels, "owner_name", "net-1")
	assertLabel(t, labels, "owner_kind", "XNetwork")
}
