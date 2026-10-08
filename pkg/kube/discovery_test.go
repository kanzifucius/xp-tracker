package kube

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

func TestDiscoverFromXRD(t *testing.T) {
	xrdWithClaim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1",
			"kind":       "CompositeResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "xpostgresqlinstances.platform.example.org",
			},
			"spec": map[string]interface{}{
				"group": "platform.example.org",
				"names": map[string]interface{}{
					"plural": "xpostgresqlinstances",
				},
				"claimNames": map[string]interface{}{
					"plural": "postgresqlinstances",
				},
				"versions": []interface{}{
					map[string]interface{}{"name": "v1alpha1", "served": true, "referenceable": false},
					map[string]interface{}{"name": "v1beta1", "served": true, "referenceable": true},
				},
			},
		},
	}
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
		map[schema.GroupVersionResource]string{
			xrdV1GVR: "CompositeResourceDefinitionList",
			xrdV2GVR: "CompositeResourceDefinitionList",
		},
		xrdWithClaim, xrdWithoutClaim,
	)

	claims, xrs, scopes, err := DiscoverFromXRD(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverFromXRD error: %v", err)
	}

	if len(claims) != 1 {
		t.Fatalf("expected 1 claim GVR, got %d", len(claims))
	}
	if claims[0].Version != "v1beta1" {
		t.Fatalf("expected referenceable version v1beta1, got %s", claims[0].Version)
	}
	if len(xrs) != 2 {
		t.Fatalf("expected 2 XR GVRs, got %d", len(xrs))
	}
	if scopes[gvrKey(xrs[0])] != config.ResourceScopeLegacyCluster {
		t.Fatalf("expected v1 XRD scope LegacyCluster, got %q", scopes[gvrKey(xrs[0])])
	}
}

func TestDiscoverFromXRD_ErrorsOnInvalidXRD(t *testing.T) {
	invalid := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1",
			"kind":       "CompositeResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "broken.example.org",
			},
			"spec": map[string]interface{}{
				"group": "example.org",
				"names": map[string]interface{}{
					"plural": "xbrokens",
				},
				"versions": []interface{}{
					map[string]interface{}{"name": "v1", "served": false},
				},
			},
		},
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			xrdV1GVR: "CompositeResourceDefinitionList",
			xrdV2GVR: "CompositeResourceDefinitionList",
		},
		invalid,
	)

	_, _, _, err := DiscoverFromXRD(context.Background(), client)
	if err == nil {
		t.Fatal("expected discovery error for XRD without referenceable/served versions")
	}
}

func activeMRD(name, group, plural, providerLabel string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1alpha1",
			"kind":       "ManagedResourceDefinition",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"group": group,
				"names": map[string]interface{}{
					"plural": plural,
				},
				"state": "Active",
				"versions": []interface{}{
					map[string]interface{}{"name": "v1alpha1", "served": true, "storage": true},
				},
			},
		},
	}
	if providerLabel != "" {
		obj.SetLabels(map[string]string{
			packageLabelKey: providerLabel,
		})
	}
	return obj
}

func TestDiscoverMRGVRsFromMRDs_ActiveWithPackageLabel(t *testing.T) {
	active := activeMRD("nopresources.nop.crossplane.io", "nop.crossplane.io", "nopresources", "provider-nop")
	inactive := activeMRD("buckets.s3.aws.m.crossplane.io", "s3.aws.m.crossplane.io", "buckets", "provider-aws")
	inactive.Object["spec"].(map[string]interface{})["state"] = "Inactive"

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			mrdGVR: "ManagedResourceDefinitionList",
		},
		active, inactive,
	)

	gvrs, providers, scopes, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverMRGVRsFromMRDs error: %v", err)
	}
	if len(gvrs) != 1 {
		t.Fatalf("expected 1 MR GVR, got %d", len(gvrs))
	}
	if gvrs[0].Group != "nop.crossplane.io" || gvrs[0].Version != "v1alpha1" || gvrs[0].Resource != "nopresources" {
		t.Fatalf("unexpected GVR: %+v", gvrs[0])
	}
	key := gvrKey(gvrs[0])
	if providers[key] != "provider-nop" {
		t.Fatalf("expected provider-nop, got %q", providers[key])
	}
	if scopes[key] != config.ResourceScopeLegacyCluster {
		t.Fatalf("expected LegacyCluster scope, got %q", scopes[key])
	}
}

func TestDiscoverMRGVRsFromMRDs_ProviderFromOwnerRef(t *testing.T) {
	mrd := activeMRD("nopresources.nop.crossplane.io", "nop.crossplane.io", "nopresources", "")
	mrd.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "pkg.crossplane.io/v1",
			Kind:       "Provider",
			Name:       "provider-nop",
		},
	})

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			mrdGVR: "ManagedResourceDefinitionList",
		},
		mrd,
	)

	_, providers, _, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverMRGVRsFromMRDs error: %v", err)
	}
	key := "nop.crossplane.io/v1alpha1/nopresources"
	if providers[key] != "provider-nop" {
		t.Fatalf("expected provider-nop from ownerRef, got %q", providers[key])
	}
}

func TestDiscoverMRGVRsFromMRDs_EmptyWhenNoActiveMRDs(t *testing.T) {
	inactive := activeMRD("nopresources.nop.crossplane.io", "nop.crossplane.io", "nopresources", "provider-nop")
	inactive.Object["spec"].(map[string]interface{})["state"] = "Inactive"

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			mrdGVR: "ManagedResourceDefinitionList",
		},
		inactive,
	)

	gvrs, providers, _, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverMRGVRsFromMRDs error: %v", err)
	}
	if len(gvrs) != 0 {
		t.Fatalf("expected 0 MR GVRs, got %d", len(gvrs))
	}
	if len(providers) != 0 {
		t.Fatalf("expected 0 provider mappings, got %d", len(providers))
	}
}

func TestDiscoverMRGVRsFromMRDs_SkipsMalformedMRD(t *testing.T) {
	healthy := activeMRD("nopresources.nop.crossplane.io", "nop.crossplane.io", "nopresources", "provider-nop")

	// Modelled on truncated MRDs from crossplane/crossplane#7817: Active,
	// versions present, but every version has served=false and storage=false.
	truncated := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1alpha1",
			"kind":       "ManagedResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "accountlocalusers.storage.azure.upbound.io",
			},
			"spec": map[string]interface{}{
				"group": "storage.azure.upbound.io",
				"names": map[string]interface{}{
					"plural": "accountlocalusers",
				},
				"state": "Active",
				"versions": []interface{}{
					map[string]interface{}{"name": "v1beta1", "served": false, "storage": false},
					map[string]interface{}{"name": "v1beta2", "served": false, "storage": false},
				},
			},
		},
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			mrdGVR: "ManagedResourceDefinitionList",
		},
		healthy, truncated,
	)

	reason := metrics.MRDSkipReasonNoStorageOrServedVersion
	before := testutil.ToFloat64(metrics.MRDDiscoverySkipped.WithLabelValues(reason))

	gvrs, providers, _, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverMRGVRsFromMRDs error: %v", err)
	}
	if len(gvrs) != 1 {
		t.Fatalf("expected 1 MR GVR (healthy only), got %d", len(gvrs))
	}
	if gvrs[0].Resource != "nopresources" {
		t.Fatalf("expected healthy nopresources GVR, got %+v", gvrs[0])
	}
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider mapping, got %d", len(providers))
	}

	after := testutil.ToFloat64(metrics.MRDDiscoverySkipped.WithLabelValues(reason))
	if got := after - before; got != 1 {
		t.Fatalf("skipped counter delta = %v, want 1 for reason %q", got, reason)
	}
}

func TestDiscoverMRGVRsFromMRDs_SkipsMissingVersions(t *testing.T) {
	healthy := activeMRD("nopresources.nop.crossplane.io", "nop.crossplane.io", "nopresources", "provider-nop")
	broken := activeMRD("broken.example.org", "example.org", "brokens", "provider-broken")
	delete(broken.Object["spec"].(map[string]interface{}), "versions")

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			mrdGVR: "ManagedResourceDefinitionList",
		},
		healthy, broken,
	)

	reason := metrics.MRDSkipReasonMissingVersions
	before := testutil.ToFloat64(metrics.MRDDiscoverySkipped.WithLabelValues(reason))

	gvrs, _, _, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverMRGVRsFromMRDs error: %v", err)
	}
	if len(gvrs) != 1 {
		t.Fatalf("expected 1 MR GVR, got %d", len(gvrs))
	}

	after := testutil.ToFloat64(metrics.MRDDiscoverySkipped.WithLabelValues(reason))
	if got := after - before; got != 1 {
		t.Fatalf("skipped counter delta = %v, want 1 for reason %q", got, reason)
	}
}

func TestDiscoverMRGVRsFromMRDs_SkipsMissingPlural(t *testing.T) {
	healthy := activeMRD("nopresources.nop.crossplane.io", "nop.crossplane.io", "nopresources", "provider-nop")
	broken := activeMRD("broken.example.org", "example.org", "brokens", "provider-broken")
	broken.Object["spec"].(map[string]interface{})["names"] = map[string]interface{}{}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			mrdGVR: "ManagedResourceDefinitionList",
		},
		healthy, broken,
	)

	reason := metrics.MRDSkipReasonMissingPlural
	before := testutil.ToFloat64(metrics.MRDDiscoverySkipped.WithLabelValues(reason))

	gvrs, _, _, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverMRGVRsFromMRDs error: %v", err)
	}
	if len(gvrs) != 1 {
		t.Fatalf("expected 1 MR GVR, got %d", len(gvrs))
	}

	after := testutil.ToFloat64(metrics.MRDDiscoverySkipped.WithLabelValues(reason))
	if got := after - before; got != 1 {
		t.Fatalf("skipped counter delta = %v, want 1 for reason %q", got, reason)
	}
}

func TestDiscoverFromXRD_V2Namespaced(t *testing.T) {
	xrd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.crossplane.io/v2",
		"kind":       "CompositeResourceDefinition",
		"metadata":   map[string]interface{}{"name": "apps.example.org"},
		"spec": map[string]interface{}{
			"group": "example.org",
			"names": map[string]interface{}{"plural": "apps"},
			"scope": "Namespaced",
			"versions": []interface{}{
				map[string]interface{}{"name": "v1", "served": true, "referenceable": true},
			},
		},
	}}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			xrdV1GVR: "CompositeResourceDefinitionList",
			xrdV2GVR: "CompositeResourceDefinitionList",
		},
		xrd,
	)

	claims, xrs, scopes, err := DiscoverFromXRD(context.Background(), client)
	if err != nil {
		t.Fatalf("DiscoverFromXRD error: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("expected no claim GVRs, got %d", len(claims))
	}
	if len(xrs) != 1 {
		t.Fatalf("expected one XR GVR, got %d", len(xrs))
	}
	if scopes[gvrKey(xrs[0])] != config.ResourceScopeNamespaced {
		t.Fatalf("expected Namespaced scope, got %q", scopes[gvrKey(xrs[0])])
	}
}
