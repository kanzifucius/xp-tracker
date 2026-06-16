package kube

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
			xrdGVR: "CompositeResourceDefinitionList",
		},
		xrdWithClaim, xrdWithoutClaim,
	)

	claims, xrs, err := DiscoverFromXRD(context.Background(), client)
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
			xrdGVR: "CompositeResourceDefinitionList",
		},
		invalid,
	)

	_, _, err := DiscoverFromXRD(context.Background(), client)
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

	gvrs, providers, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
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

	_, providers, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
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

	gvrs, providers, err := DiscoverMRGVRsFromMRDs(context.Background(), client)
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
