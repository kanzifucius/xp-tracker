package kube

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func newFakeClient(gvrs map[schema.GroupVersionResource]string, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrs, objects...)
}

func TestPoller_PollClaims(t *testing.T) {
	claimGVR := schema.GroupVersionResource{
		Group: "platform.example.org", Version: "v1alpha1", Resource: "postgresqlinstances",
	}

	xrGVR := schema.GroupVersionResource{
		Group: "platform.example.org", Version: "v1alpha1", Resource: "xpostgresqlinstances",
	}

	// Create fake objects.
	claim1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "platform.example.org/v1alpha1",
			"kind":       "PostgreSQLInstance",
			"metadata": map[string]interface{}{
				"name":      "db-1",
				"namespace": "team-a",
				"annotations": map[string]interface{}{
					"platform.example.org/creator": "alice",
				},
			},
			"spec": map[string]interface{}{
				"resourceRef": map[string]interface{}{
					"name": "xr-db-1",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
						"reason": "Available",
					},
				},
			},
		},
	}

	claim2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "platform.example.org/v1alpha1",
			"kind":       "PostgreSQLInstance",
			"metadata": map[string]interface{}{
				"name":      "db-2",
				"namespace": "team-b",
				"annotations": map[string]interface{}{
					"platform.example.org/creator": "bob",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "False",
						"reason": "Creating",
					},
				},
			},
		},
	}

	xr1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "platform.example.org/v1alpha1",
			"kind":       "XPostgreSQLInstance",
			"metadata": map[string]interface{}{
				"name": "xr-db-1",
				"labels": map[string]interface{}{
					"crossplane.io/composition-name": "prod-postgres",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
						"reason": "Available",
					},
				},
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			claimGVR: "PostgreSQLInstanceList",
			xrGVR:    "XPostgreSQLInstanceList",
		},
		claim1, claim2, xr1,
	)

	cfg := &config.Config{
		ClaimGVRs:            []schema.GroupVersionResource{claimGVR},
		XRGVRs:               []schema.GroupVersionResource{xrGVR},
		CreatorAnnotationKey: "platform.example.org/creator",
		CompositionLabelKey:  "crossplane.io/composition-name",
		PollIntervalSeconds:  30,
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)

	// Run a single poll cycle.
	ctx := context.Background()
	poller.poll(ctx)

	// Verify claims.
	if s.ClaimCount() != 2 {
		t.Fatalf("expected 2 claims, got %d", s.ClaimCount())
	}

	claims := s.SnapshotClaims()
	byName := make(map[string]store.ClaimInfo)
	for _, c := range claims {
		byName[c.Name] = c
	}

	db1 := byName["db-1"]
	if db1.Creator != "alice" {
		t.Errorf("db-1 creator: got %q", db1.Creator)
	}
	if !db1.Ready {
		t.Error("db-1 should be ready")
	}
	if db1.Composition != "prod-postgres" {
		t.Errorf("db-1 composition: got %q (expected enrichment from XR)", db1.Composition)
	}

	db2 := byName["db-2"]
	if db2.Creator != "bob" {
		t.Errorf("db-2 creator: got %q", db2.Creator)
	}
	if db2.Ready {
		t.Error("db-2 should not be ready")
	}
	if db2.Reason != "Creating" {
		t.Errorf("db-2 reason: got %q", db2.Reason)
	}

	// Verify XRs.
	if s.XRCount() != 1 {
		t.Fatalf("expected 1 XR, got %d", s.XRCount())
	}

	xrs := s.SnapshotXRs()
	if xrs[0].Composition != "prod-postgres" {
		t.Errorf("XR composition: got %q", xrs[0].Composition)
	}
	if !xrs[0].Ready {
		t.Error("XR should be ready")
	}
}

func TestPoller_NamespaceScoped(t *testing.T) {
	claimGVR := schema.GroupVersionResource{
		Group: "g", Version: "v1", Resource: "things",
	}
	xrGVR := schema.GroupVersionResource{
		Group: "g", Version: "v1", Resource: "xthings",
	}

	claimA := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"kind":       "Thing",
			"metadata": map[string]interface{}{
				"name":      "t1",
				"namespace": "ns-a",
			},
		},
	}
	claimB := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"kind":       "Thing",
			"metadata": map[string]interface{}{
				"name":      "t2",
				"namespace": "ns-b",
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			claimGVR: "ThingList",
			xrGVR:    "XThingList",
		},
		claimA, claimB,
	)

	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{claimGVR},
		XRGVRs:              []schema.GroupVersionResource{xrGVR},
		Namespaces:          []string{"ns-a"}, // only watch ns-a
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.poll(context.Background())

	// Should only see the claim in ns-a.
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim (ns-a only), got %d", s.ClaimCount())
	}

	claims := s.SnapshotClaims()
	if claims[0].Namespace != "ns-a" {
		t.Errorf("expected namespace ns-a, got %q", claims[0].Namespace)
	}
}

func TestPoller_RunStopsOnCancel(t *testing.T) {
	claimGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "things"}
	xrGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "xthings"}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			claimGVR: "ThingList",
			xrGVR:    "XThingList",
		},
	)

	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{claimGVR},
		XRGVRs:              []schema.GroupVersionResource{xrGVR},
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 1, // short interval for test
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		poller.Run(ctx)
		close(done)
	}()

	// Give it time to run initial poll + one tick.
	time.Sleep(1500 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK, poller exited.
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not stop after context cancellation")
	}
}

func TestPoller_StaleRemoval(t *testing.T) {
	claimGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "things"}
	xrGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "xthings"}

	claim1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"kind":       "Thing",
			"metadata":   map[string]interface{}{"name": "t1", "namespace": "ns"},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			claimGVR: "ThingList",
			xrGVR:    "XThingList",
		},
		claim1,
	)

	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{claimGVR},
		XRGVRs:              []schema.GroupVersionResource{xrGVR},
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)

	// First poll — should find 1 claim.
	poller.poll(context.Background())
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim, got %d", s.ClaimCount())
	}

	// Remove the object from the fake client.
	err := client.Resource(claimGVR).Namespace("ns").Delete(
		context.Background(), "t1", metav1.DeleteOptions{},
	)
	if err != nil {
		t.Fatalf("failed to delete fake object: %v", err)
	}

	// Second poll — stale claim should be removed.
	poller.poll(context.Background())
	if s.ClaimCount() != 0 {
		t.Fatalf("expected 0 claims after deletion, got %d", s.ClaimCount())
	}
}

func TestPoller_PollMRsConcurrent(t *testing.T) {
	// Build N MR GVRs to exercise the concurrent fan-out path.
	const n = 30 // more than mrPollConcurrency (20) to test queuing

	claimGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "things"}
	xrGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "xthings"}

	gvrMap := map[schema.GroupVersionResource]string{
		claimGVR: "ThingList",
		xrGVR:    "XThingList",
	}
	mrGVRs := make([]schema.GroupVersionResource, n)
	providerNames := make(map[string]string, n)
	var objects []runtime.Object

	for i := range n {
		gvr := schema.GroupVersionResource{
			Group:    fmt.Sprintf("provider-%d.crossplane.io", i),
			Version:  "v1alpha1",
			Resource: "widgets",
		}
		mrGVRs[i] = gvr
		listKind := fmt.Sprintf("Widget%dList", i)
		gvrMap[gvr] = listKind
		key := GVRString(gvr)
		providerNames[key] = fmt.Sprintf("provider-%d", i)

		// Add one MR with the composite label so it is stored.
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": gvr.Group + "/" + gvr.Version,
				"kind":       "Widget",
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("widget-%d", i),
					"labels": map[string]interface{}{
						"crossplane.io/composite": fmt.Sprintf("xr-%d", i),
					},
				},
			},
		}
		objects = append(objects, obj)
	}

	client := newFakeClient(gvrMap, objects...)

	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{claimGVR},
		XRGVRs:              []schema.GroupVersionResource{xrGVR},
		MRGVRs:              mrGVRs,
		MRProviderNames:     providerNames,
		CompositeLabelKey:   "crossplane.io/composite",
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.poll(context.Background())

	if got := s.MRCount(); got != n {
		t.Fatalf("expected %d MRs, got %d", n, got)
	}
}

func TestPoller_PollMRs(t *testing.T) {
	claimGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "things"}
	xrGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "xthings"}
	mrGVR := schema.GroupVersionResource{Group: "nop.crossplane.io", Version: "v1alpha1", Resource: "nopresources"}

	mrLinked := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nop.crossplane.io/v1alpha1",
			"kind":       "NopResource",
			"metadata": map[string]interface{}{
				"name":      "nop-1",
				"namespace": "default",
				"labels": map[string]interface{}{
					"crossplane.io/composite": "xr-1",
				},
			},
			"spec": map[string]interface{}{
				"providerConfigRef": map[string]interface{}{"name": "default"},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True", "reason": "Available"},
				},
			},
		},
	}

	xr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"kind":       "XThing",
			"metadata": map[string]interface{}{
				"name": "xr-1",
				"labels": map[string]interface{}{
					"crossplane.io/claim-name":      "thing-1",
					"crossplane.io/claim-namespace": "default",
				},
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			claimGVR: "ThingList",
			xrGVR:    "XThingList",
			mrGVR:    "NopResourceList",
		},
		mrLinked, xr,
	)

	gvrKey := "nop.crossplane.io/v1alpha1/nopresources"
	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{claimGVR},
		XRGVRs:              []schema.GroupVersionResource{xrGVR},
		MRGVRs:              []schema.GroupVersionResource{mrGVR},
		MRProviderNames:     map[string]string{gvrKey: "provider-nop"},
		CompositionLabelKey: "crossplane.io/composition-name",
		CompositeLabelKey:   "crossplane.io/composite",
		PollIntervalSeconds: 30,
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.poll(context.Background())

	if s.MRCount() != 1 {
		t.Fatalf("expected 1 MR, got %d", s.MRCount())
	}

	mrs := s.SnapshotMRs()
	if mrs[0].XRName != "xr-1" {
		t.Errorf("XRName: got %q", mrs[0].XRName)
	}
	if mrs[0].ClaimName != "thing-1" {
		t.Errorf("ClaimName: got %q, want thing-1", mrs[0].ClaimName)
	}
	if mrs[0].Provider != "provider-nop" {
		t.Errorf("Provider: got %q", mrs[0].Provider)
	}
}
