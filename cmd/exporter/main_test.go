package main

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/metrics"
)

func xrdListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositeresourcedefinitions"}:     "CompositeResourceDefinitionList",
		{Group: "apiextensions.crossplane.io", Version: "v2", Resource: "compositeresourcedefinitions"}:     "CompositeResourceDefinitionList",
		{Group: "apiextensions.crossplane.io", Version: "v1alpha1", Resource: "managedresourcedefinitions"}: "ManagedResourceDefinitionList",
	}
}

func sampleXRDWithClaim() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1",
			"kind":       "CompositeResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "xqueues.platform.example.org",
			},
			"spec": map[string]interface{}{
				"group": "platform.example.org",
				"names": map[string]interface{}{
					"plural": "xqueues",
				},
				"claimNames": map[string]interface{}{
					"plural": "queues",
				},
				"versions": []interface{}{
					map[string]interface{}{"name": "v1", "served": true, "referenceable": true},
				},
			},
		},
	}
}

func TestDiscoverAndApplyGVRs_NoClaims(t *testing.T) {
	xrdWithoutClaim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1",
			"kind":       "CompositeResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "xqueues.platform.example.org",
			},
			"spec": map[string]interface{}{
				"group": "platform.example.org",
				"names": map[string]interface{}{
					"plural": "xqueues",
				},
				"versions": []interface{}{
					map[string]interface{}{"name": "v1", "served": true},
				},
			},
		},
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		xrdListKinds(),
		xrdWithoutClaim,
	)

	cfg := &config.Config{}
	err := discoverAndApplyGVRs(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("discoverAndApplyGVRs error: %v", err)
	}
	if len(cfg.ClaimGVRs) != 0 {
		t.Fatalf("expected no claim GVRs, got %d", len(cfg.ClaimGVRs))
	}
	if len(cfg.XRGVRs) != 1 {
		t.Fatalf("expected one XR GVR, got %d", len(cfg.XRGVRs))
	}
}

func TestDiscoverAndApplyGVRs_DiscoveryOnly(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		xrdListKinds(),
		sampleXRDWithClaim(),
	)

	cfg := &config.Config{}
	if err := discoverAndApplyGVRs(context.Background(), client, cfg); err != nil {
		t.Fatalf("discoverAndApplyGVRs error: %v", err)
	}

	if len(cfg.ClaimGVRs) != 1 {
		t.Fatalf("expected 1 claim GVR, got %d", len(cfg.ClaimGVRs))
	}
	if got := gvrKey(cfg.ClaimGVRs[0]); got != "platform.example.org/v1/queues" {
		t.Fatalf("unexpected claim GVR: %s", got)
	}
	if len(cfg.XRGVRs) != 1 {
		t.Fatalf("expected 1 XR GVR, got %d", len(cfg.XRGVRs))
	}
	if got := gvrKey(cfg.XRGVRs[0]); got != "platform.example.org/v1/xqueues" {
		t.Fatalf("unexpected XR GVR: %s", got)
	}
}

func TestDiscoverAndApplyGVRs_EnvAdditiveMerge(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		xrdListKinds(),
		sampleXRDWithClaim(),
	)

	extraClaim := schema.GroupVersionResource{
		Group: "platform.example.org", Version: "v1", Resource: "topics",
	}
	extraXR := schema.GroupVersionResource{
		Group: "platform.example.org", Version: "v1", Resource: "xtopics",
	}
	cfg := &config.Config{
		EnvClaimGVRs: []schema.GroupVersionResource{extraClaim},
		EnvXRGVRs:    []schema.GroupVersionResource{extraXR},
	}
	if err := discoverAndApplyGVRs(context.Background(), client, cfg); err != nil {
		t.Fatalf("discoverAndApplyGVRs error: %v", err)
	}

	targets := cfg.SnapshotPollTargets()
	if len(targets.ClaimGVRs) != 2 {
		t.Fatalf("expected 2 claim GVRs after merge, got %d", len(targets.ClaimGVRs))
	}
	if len(targets.XRGVRs) != 2 {
		t.Fatalf("expected 2 XR GVRs after merge, got %d", len(targets.XRGVRs))
	}
	assertContainsGVR(t, targets.ClaimGVRs, "platform.example.org/v1/queues")
	assertContainsGVR(t, targets.ClaimGVRs, "platform.example.org/v1/topics")
	assertContainsGVR(t, targets.XRGVRs, "platform.example.org/v1/xqueues")
	assertContainsGVR(t, targets.XRGVRs, "platform.example.org/v1/xtopics")
}

func TestDiscoverAndApplyGVRs_EnvDedupe(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		xrdListKinds(),
		sampleXRDWithClaim(),
	)

	// Env lists the same GVRs discovery will find, plus one unique claim.
	cfg := &config.Config{
		EnvClaimGVRs: []schema.GroupVersionResource{
			{Group: "platform.example.org", Version: "v1", Resource: "queues"},
			{Group: "platform.example.org", Version: "v1", Resource: "topics"},
		},
		EnvXRGVRs: []schema.GroupVersionResource{
			{Group: "platform.example.org", Version: "v1", Resource: "xqueues"},
		},
	}
	if err := discoverAndApplyGVRs(context.Background(), client, cfg); err != nil {
		t.Fatalf("discoverAndApplyGVRs error: %v", err)
	}

	targets := cfg.SnapshotPollTargets()
	if len(targets.ClaimGVRs) != 2 {
		t.Fatalf("expected 2 claim GVRs after dedupe, got %d: %v", len(targets.ClaimGVRs), formatGVRs(targets.ClaimGVRs))
	}
	if len(targets.XRGVRs) != 1 {
		t.Fatalf("expected 1 XR GVR after dedupe, got %d: %v", len(targets.XRGVRs), formatGVRs(targets.XRGVRs))
	}
	assertContainsGVR(t, targets.ClaimGVRs, "platform.example.org/v1/queues")
	assertContainsGVR(t, targets.ClaimGVRs, "platform.example.org/v1/topics")
	assertContainsGVR(t, targets.XRGVRs, "platform.example.org/v1/xqueues")
}

func TestDiscoverAndApplyGVRs_RediscoveryPicksUpNewXRD(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		xrdListKinds(),
		sampleXRDWithClaim(),
	)

	extraClaim := schema.GroupVersionResource{
		Group: "platform.example.org", Version: "v1", Resource: "topics",
	}
	cfg := &config.Config{
		EnvClaimGVRs: []schema.GroupVersionResource{extraClaim},
	}

	beforeSuccess := testutil.ToFloat64(metrics.DiscoveryRuns.WithLabelValues(metrics.DiscoveryResultSuccess))
	if err := discoverAndApplyGVRs(context.Background(), client, cfg); err != nil {
		t.Fatalf("initial discoverAndApplyGVRs error: %v", err)
	}
	initial := cfg.SnapshotPollTargets()
	if len(initial.XRGVRs) != 1 {
		t.Fatalf("expected 1 XR GVR after initial discovery, got %d", len(initial.XRGVRs))
	}
	assertContainsGVR(t, initial.ClaimGVRs, "platform.example.org/v1/topics")

	// Simulate a new XRD appearing in the cluster without restarting the process.
	newXRD := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1",
			"kind":       "CompositeResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "xbuckets.platform.example.org",
			},
			"spec": map[string]interface{}{
				"group": "platform.example.org",
				"names": map[string]interface{}{
					"plural": "xbuckets",
				},
				"claimNames": map[string]interface{}{
					"plural": "buckets",
				},
				"versions": []interface{}{
					map[string]interface{}{"name": "v1", "served": true, "referenceable": true},
				},
			},
		},
	}
	xrdGVR := schema.GroupVersionResource{
		Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositeresourcedefinitions",
	}
	if _, err := client.Resource(xrdGVR).Create(context.Background(), newXRD, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create new XRD: %v", err)
	}

	if err := discoverAndApplyGVRs(context.Background(), client, cfg); err != nil {
		t.Fatalf("rediscovery discoverAndApplyGVRs error: %v", err)
	}

	refreshed := cfg.SnapshotPollTargets()
	assertContainsGVR(t, refreshed.XRGVRs, "platform.example.org/v1/xqueues")
	assertContainsGVR(t, refreshed.XRGVRs, "platform.example.org/v1/xbuckets")
	assertContainsGVR(t, refreshed.ClaimGVRs, "platform.example.org/v1/queues")
	assertContainsGVR(t, refreshed.ClaimGVRs, "platform.example.org/v1/buckets")
	// Env override must still be present after rediscovery.
	assertContainsGVR(t, refreshed.ClaimGVRs, "platform.example.org/v1/topics")
	if len(refreshed.XRGVRs) != 2 {
		t.Fatalf("expected 2 XR GVRs after rediscovery, got %d: %v", len(refreshed.XRGVRs), formatGVRs(refreshed.XRGVRs))
	}
	if len(refreshed.ClaimGVRs) != 3 {
		t.Fatalf("expected 3 claim GVRs after rediscovery, got %d: %v", len(refreshed.ClaimGVRs), formatGVRs(refreshed.ClaimGVRs))
	}

	afterSuccess := testutil.ToFloat64(metrics.DiscoveryRuns.WithLabelValues(metrics.DiscoveryResultSuccess))
	if afterSuccess-beforeSuccess < 2 {
		t.Fatalf("expected discovery success counter to increase by at least 2, delta=%v", afterSuccess-beforeSuccess)
	}
}

func TestDiscoverAndApplyGVRs_RediscoveryKeepsInventoryOnFailure(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		xrdListKinds(),
		sampleXRDWithClaim(),
	)
	cfg := &config.Config{}
	if err := discoverAndApplyGVRs(context.Background(), client, cfg); err != nil {
		t.Fatalf("initial discoverAndApplyGVRs error: %v", err)
	}
	before := cfg.SnapshotPollTargets()

	// Empty tracker with no XRDs: rediscovery must fail and leave inventory alone.
	empty := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), xrdListKinds())
	beforeErrors := testutil.ToFloat64(metrics.DiscoveryRuns.WithLabelValues(metrics.DiscoveryResultError))
	if err := discoverAndApplyGVRs(context.Background(), empty, cfg); err == nil {
		t.Fatal("expected rediscovery error when no XRDs exist")
	}
	after := cfg.SnapshotPollTargets()
	if len(after.XRGVRs) != len(before.XRGVRs) || len(after.ClaimGVRs) != len(before.ClaimGVRs) {
		t.Fatalf("inventory changed after failed rediscovery: before=%v/%v after=%v/%v",
			formatGVRs(before.ClaimGVRs), formatGVRs(before.XRGVRs),
			formatGVRs(after.ClaimGVRs), formatGVRs(after.XRGVRs))
	}
	afterErrors := testutil.ToFloat64(metrics.DiscoveryRuns.WithLabelValues(metrics.DiscoveryResultError))
	if afterErrors-beforeErrors < 1 {
		t.Fatalf("expected discovery error counter to increase, delta=%v", afterErrors-beforeErrors)
	}
}

func TestMergeGVRs(t *testing.T) {
	a := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "as"}
	b := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "bs"}
	dup := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "as"}

	got := mergeGVRs([]schema.GroupVersionResource{a}, []schema.GroupVersionResource{dup, b})
	if len(got) != 2 {
		t.Fatalf("expected 2 GVRs, got %d", len(got))
	}
	if gvrKey(got[0]) != "g/v1/as" || gvrKey(got[1]) != "g/v1/bs" {
		t.Fatalf("unexpected merge order/content: %v", formatGVRs(got))
	}
}

func gvrKey(gvr schema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}

func assertContainsGVR(t *testing.T, gvrs []schema.GroupVersionResource, want string) {
	t.Helper()
	for _, gvr := range gvrs {
		if gvrKey(gvr) == want {
			return
		}
	}
	t.Fatalf("expected GVR %q in %v", want, formatGVRs(gvrs))
}
