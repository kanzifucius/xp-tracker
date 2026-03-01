package kube

import (
	"context"
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
	if db1.Source != "central" {
		t.Errorf("db-1 source: got %q, want %q", db1.Source, "central")
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
	if db2.Source != "central" {
		t.Errorf("db-2 source: got %q, want %q", db2.Source, "central")
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
	if xrs[0].Source != "central" {
		t.Errorf("XR source: got %q, want %q", xrs[0].Source, "central")
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

// staticNSConfigs is a simple NamespaceConfigProvider for testing.
type staticNSConfigs struct {
	configs []config.NamespaceConfig
}

func (s *staticNSConfigs) NamespaceConfigs() []config.NamespaceConfig {
	return s.configs
}

func TestPoller_NamespaceConfigClaims(t *testing.T) {
	// A GVR only defined in the namespace config — not in central config.
	nsClaimGVR := schema.GroupVersionResource{
		Group: "team.example.org", Version: "v1alpha1", Resource: "databases",
	}

	claim1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1alpha1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      "db-1",
				"namespace": "team-a",
				"annotations": map[string]interface{}{
					"custom.io/creator": "alice",
					"custom.io/team":    "platform",
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

	// A claim in a different namespace — should NOT be picked up.
	claim2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1alpha1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      "db-2",
				"namespace": "team-b",
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			nsClaimGVR: "DatabaseList",
		},
		claim1, claim2,
	)

	cfg := &config.Config{
		CreatorAnnotationKey: "default.io/creator",
		CompositionLabelKey:  "crossplane.io/composition-name",
		PollIntervalSeconds:  30,
	}

	nsProvider := &staticNSConfigs{
		configs: []config.NamespaceConfig{
			{
				Namespace:            "team-a",
				ConfigMapName:        "team-a-config",
				ClaimGVRs:            []schema.GroupVersionResource{nsClaimGVR},
				CreatorAnnotationKey: "custom.io/creator",
				TeamAnnotationKey:    "custom.io/team",
			},
		},
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.SetNamespaceConfigProvider(nsProvider)

	poller.poll(context.Background())

	// Should only see claim from team-a (scoped to that namespace).
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim (team-a only), got %d", s.ClaimCount())
	}

	claims := s.SnapshotClaims()
	c := claims[0]
	if c.Name != "db-1" {
		t.Errorf("expected claim name db-1, got %q", c.Name)
	}
	if c.Namespace != "team-a" {
		t.Errorf("expected namespace team-a, got %q", c.Namespace)
	}
	// Custom creator annotation key should be used.
	if c.Creator != "alice" {
		t.Errorf("expected creator alice (from custom key), got %q", c.Creator)
	}
	if c.Team != "platform" {
		t.Errorf("expected team platform (from custom key), got %q", c.Team)
	}
	if c.Source != "namespace" {
		t.Errorf("expected source namespace, got %q", c.Source)
	}
}

func TestPoller_NamespaceConfigXRsClusterWide(t *testing.T) {
	// Namespace config specifies XR GVR — should be polled cluster-wide.
	nsXRGVR := schema.GroupVersionResource{
		Group: "team.example.org", Version: "v1alpha1", Resource: "xdatabases",
	}

	xr1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1alpha1",
			"kind":       "XDatabase",
			"metadata": map[string]interface{}{
				"name": "xdb-1",
				"labels": map[string]interface{}{
					"crossplane.io/composition-name": "db-composition",
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

	xr2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1alpha1",
			"kind":       "XDatabase",
			"metadata": map[string]interface{}{
				"name": "xdb-2",
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			nsXRGVR: "XDatabaseList",
		},
		xr1, xr2,
	)

	cfg := &config.Config{
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	nsProvider := &staticNSConfigs{
		configs: []config.NamespaceConfig{
			{
				Namespace:     "team-a",
				ConfigMapName: "team-a-config",
				XRGVRs:        []schema.GroupVersionResource{nsXRGVR},
			},
		},
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.SetNamespaceConfigProvider(nsProvider)

	poller.poll(context.Background())

	// Both XRs should be found (cluster-wide).
	if s.XRCount() != 2 {
		t.Fatalf("expected 2 XRs (cluster-wide), got %d", s.XRCount())
	}

	xrs := s.SnapshotXRs()
	byName := make(map[string]store.XRInfo)
	for _, x := range xrs {
		byName[x.Name] = x
	}

	if byName["xdb-1"].Composition != "db-composition" {
		t.Errorf("xdb-1 composition: got %q, want %q", byName["xdb-1"].Composition, "db-composition")
	}
	if byName["xdb-1"].Source != "namespace" {
		t.Errorf("xdb-1 source: got %q, want %q", byName["xdb-1"].Source, "namespace")
	}
}

func TestPoller_NamespaceConfigDeduplication(t *testing.T) {
	// The same GVR appears in both central config and namespace config.
	// Central should win — namespace config GVR should be skipped.
	sharedGVR := schema.GroupVersionResource{
		Group: "shared.example.org", Version: "v1", Resource: "widgets",
	}

	widget := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "shared.example.org/v1",
			"kind":       "Widget",
			"metadata": map[string]interface{}{
				"name":      "w1",
				"namespace": "team-a",
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			sharedGVR: "WidgetList",
		},
		widget,
	)

	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{sharedGVR},
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	nsProvider := &staticNSConfigs{
		configs: []config.NamespaceConfig{
			{
				Namespace:     "team-a",
				ConfigMapName: "team-a-config",
				ClaimGVRs:     []schema.GroupVersionResource{sharedGVR}, // same GVR
			},
		},
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.SetNamespaceConfigProvider(nsProvider)

	poller.poll(context.Background())

	// Should have exactly 1 claim — no duplication.
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim (dedup), got %d", s.ClaimCount())
	}
}

func TestPoller_MultipleNamespaceConfigs(t *testing.T) {
	gvrA := schema.GroupVersionResource{
		Group: "a.example.org", Version: "v1", Resource: "alphas",
	}
	gvrB := schema.GroupVersionResource{
		Group: "b.example.org", Version: "v1", Resource: "betas",
	}

	claimA := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "a.example.org/v1",
			"kind":       "Alpha",
			"metadata": map[string]interface{}{
				"name":      "a1",
				"namespace": "ns-a",
			},
		},
	}
	claimB := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "b.example.org/v1",
			"kind":       "Beta",
			"metadata": map[string]interface{}{
				"name":      "b1",
				"namespace": "ns-b",
			},
		},
	}
	// This claim is in ns-a but for gvrB — should NOT be picked up
	// because ns-a config only has gvrA.
	claimBinA := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "b.example.org/v1",
			"kind":       "Beta",
			"metadata": map[string]interface{}{
				"name":      "b-in-a",
				"namespace": "ns-a",
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			gvrA: "AlphaList",
			gvrB: "BetaList",
		},
		claimA, claimB, claimBinA,
	)

	cfg := &config.Config{
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	nsProvider := &staticNSConfigs{
		configs: []config.NamespaceConfig{
			{
				Namespace:     "ns-a",
				ConfigMapName: "ns-a-config",
				ClaimGVRs:     []schema.GroupVersionResource{gvrA},
			},
			{
				Namespace:     "ns-b",
				ConfigMapName: "ns-b-config",
				ClaimGVRs:     []schema.GroupVersionResource{gvrB},
			},
		},
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.SetNamespaceConfigProvider(nsProvider)

	poller.poll(context.Background())

	// Should have 2 claims: a1 from ns-a and b1 from ns-b.
	if s.ClaimCount() != 2 {
		t.Fatalf("expected 2 claims from 2 namespace configs, got %d", s.ClaimCount())
	}

	claims := s.SnapshotClaims()
	byName := make(map[string]store.ClaimInfo)
	for _, c := range claims {
		byName[c.Name] = c
	}

	if _, ok := byName["a1"]; !ok {
		t.Error("expected claim a1 from ns-a")
	}
	if _, ok := byName["b1"]; !ok {
		t.Error("expected claim b1 from ns-b")
	}
	if _, ok := byName["b-in-a"]; ok {
		t.Error("claim b-in-a should NOT be present (wrong GVR for ns-a config)")
	}
}

func TestPoller_NamespaceConfigStaleRemoval(t *testing.T) {
	nsGVR := schema.GroupVersionResource{
		Group: "team.example.org", Version: "v1", Resource: "databases",
	}

	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      "db-1",
				"namespace": "team-a",
			},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			nsGVR: "DatabaseList",
		},
		claim,
	)

	cfg := &config.Config{
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	nsProvider := &staticNSConfigs{
		configs: []config.NamespaceConfig{
			{
				Namespace:     "team-a",
				ConfigMapName: "team-a-config",
				ClaimGVRs:     []schema.GroupVersionResource{nsGVR},
			},
		},
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.SetNamespaceConfigProvider(nsProvider)

	// First poll — should find 1 claim.
	poller.poll(context.Background())
	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim, got %d", s.ClaimCount())
	}

	// Delete the object.
	err := client.Resource(nsGVR).Namespace("team-a").Delete(
		context.Background(), "db-1", metav1.DeleteOptions{},
	)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Second poll — stale claim should be removed.
	poller.poll(context.Background())
	if s.ClaimCount() != 0 {
		t.Fatalf("expected 0 claims after deletion, got %d", s.ClaimCount())
	}
}

func TestPoller_NamespaceConfigWithCompositionEnrichment(t *testing.T) {
	nsClaimGVR := schema.GroupVersionResource{
		Group: "team.example.org", Version: "v1", Resource: "databases",
	}
	nsXRGVR := schema.GroupVersionResource{
		Group: "team.example.org", Version: "v1", Resource: "xdatabases",
	}

	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      "db-1",
				"namespace": "team-a",
			},
			"spec": map[string]interface{}{
				"resourceRef": map[string]interface{}{
					"name": "xdb-1",
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

	xr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "team.example.org/v1",
			"kind":       "XDatabase",
			"metadata": map[string]interface{}{
				"name": "xdb-1",
				"labels": map[string]interface{}{
					"crossplane.io/composition-name": "ns-composition",
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
			nsClaimGVR: "DatabaseList",
			nsXRGVR:    "XDatabaseList",
		},
		claim, xr,
	)

	cfg := &config.Config{
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	nsProvider := &staticNSConfigs{
		configs: []config.NamespaceConfig{
			{
				Namespace:     "team-a",
				ConfigMapName: "team-a-config",
				ClaimGVRs:     []schema.GroupVersionResource{nsClaimGVR},
				XRGVRs:        []schema.GroupVersionResource{nsXRGVR},
			},
		},
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	poller.SetNamespaceConfigProvider(nsProvider)

	poller.poll(context.Background())

	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim, got %d", s.ClaimCount())
	}

	claims := s.SnapshotClaims()
	if claims[0].Composition != "ns-composition" {
		t.Errorf("expected composition %q (enriched from namespace XR), got %q", "ns-composition", claims[0].Composition)
	}
	if claims[0].Source != "namespace" {
		t.Errorf("expected source %q, got %q", "namespace", claims[0].Source)
	}
}

func TestPoller_NilNamespaceConfigProvider(t *testing.T) {
	// Ensure the poller works fine without a namespace config provider.
	claimGVR := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "things"}

	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"kind":       "Thing",
			"metadata":   map[string]interface{}{"name": "t1", "namespace": "ns"},
		},
	}

	client := newFakeClient(
		map[schema.GroupVersionResource]string{
			claimGVR: "ThingList",
		},
		claim,
	)

	cfg := &config.Config{
		ClaimGVRs:           []schema.GroupVersionResource{claimGVR},
		CompositionLabelKey: "crossplane.io/composition-name",
		PollIntervalSeconds: 30,
	}

	s := store.New()
	poller := NewPoller(client, cfg, s)
	// Intentionally NOT setting namespace config provider.

	poller.poll(context.Background())

	if s.ClaimCount() != 1 {
		t.Fatalf("expected 1 claim, got %d", s.ClaimCount())
	}
}
