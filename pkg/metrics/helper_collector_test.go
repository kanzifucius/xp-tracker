package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestHelperCollector_Disabled(t *testing.T) {
	s := store.New()
	s.ReplaceXRs("g/v1/x", []store.XRInfo{{
		GVR: "g/v1/x", Group: "g", Kind: "X", Name: "a",
		ReadyLastTransition: time.Unix(1700000000, 0), Reason: "Available", ReadyMessage: "ok",
	}})
	c := NewHelperCollector(s, false, false, false)
	families := gatherCollector(t, c)
	if families["crossplane_xr_ready_last_transition_timestamp_seconds"] != nil {
		t.Fatal("expected no transition metrics when disabled")
	}
	if families["crossplane_xr_condition_info"] != nil {
		t.Fatal("expected no condition info when disabled")
	}
}

func TestHelperCollector_TransitionsAndIdentity(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	s := store.New()
	s.ReplaceXRs("g/v1/xapps", []store.XRInfo{{
		GVR: "g/v1/xapps", Group: "g", Version: "v1", Kind: "XApp", Namespace: "ns", Name: "app-1",
		UID: "uid-1", CompositionRevision: "rev-9", ReadyLastTransition: ts, Reason: "Available",
	}})
	c := NewHelperCollector(s, true, false, true)
	families := gatherCollector(t, c)
	fam := families["crossplane_xr_ready_last_transition_timestamp_seconds"]
	if fam == nil || len(fam.GetMetric()) != 1 {
		t.Fatalf("expected 1 transition sample, got %v", fam)
	}
	labels := findLabelsByLabelValue(t, fam.GetMetric(), "name", "app-1")
	assertLabel(t, labels, "uid", "uid-1")
	assertLabel(t, labels, "composition_revision", "rev-9")
	if got := fam.GetMetric()[0].GetGauge().GetValue(); got != float64(ts.Unix()) {
		t.Fatalf("timestamp: got %v want %v", got, ts.Unix())
	}
}

func TestHelperCollector_ConditionInfoTruncates(t *testing.T) {
	s := store.New()
	long := strings.Repeat("x", 200)
	s.ReplaceMRs("nop/v1/nops", []store.MRInfo{{
		GVR: "nop/v1/nops", Group: "nop", Version: "v1", Kind: "NopResource", Namespace: "ns", Name: "nop-1",
		Reason: "Unavailable", ReadyMessage: long,
	}})
	c := NewHelperCollector(s, false, true, false)
	families := gatherCollector(t, c)
	fam := families["crossplane_mr_condition_info"]
	if fam == nil || len(fam.GetMetric()) != 1 {
		t.Fatalf("expected 1 condition info sample, got %v", fam)
	}
	labels := findLabelsByLabelValue(t, fam.GetMetric(), "name", "nop-1")
	assertLabel(t, labels, "type", "Ready")
	assertLabel(t, labels, "reason", "Unavailable")
	msg := labels["message"]
	if len([]rune(msg)) != conditionMessageMaxRunes {
		t.Fatalf("expected message truncated to %d runes, got %d", conditionMessageMaxRunes, len([]rune(msg)))
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := truncateRunes("abcdefghij", 5); got != "abcde" {
		t.Fatalf("got %q", got)
	}
}
