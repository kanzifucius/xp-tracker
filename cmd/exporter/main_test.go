package main

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/kanzifucius/xp-tracker/pkg/config"
)

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
		map[schema.GroupVersionResource]string{
			{Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositeresourcedefinitions"}:      "CompositeResourceDefinitionList",
			{Group: "apiextensions.crossplane.io", Version: "v2", Resource: "compositeresourcedefinitions"}:      "CompositeResourceDefinitionList",
			{Group: "apiextensions.crossplane.io", Version: "v1alpha1", Resource: "managedresourcedefinitions"}: "ManagedResourceDefinitionList",
		},
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
