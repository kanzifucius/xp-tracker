package kube

import (
	"context"
	"testing"

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
