package kube

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kanzifucius/xp-tracker/pkg/config"
)

func TestGVRString(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "platform.example.org", Version: "v1alpha1", Resource: "postgresqlinstances"}
	got := GVRString(gvr)
	want := "platform.example.org/v1alpha1/postgresqlinstances"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnstructuredToClaim_Full(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "platform.example.org", Version: "v1alpha1", Resource: "postgresqlinstances"}
	cfg := &config.Config{
		CreatorAnnotationKey: "platform.example.org/creator",
		TeamAnnotationKey:    "platform.example.org/team",
		CompositionLabelKey:  "crossplane.io/composition-name",
	}

	now := time.Now().Truncate(time.Second)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "platform.example.org/v1alpha1",
			"kind":       "PostgreSQLInstance",
			"metadata": map[string]interface{}{
				"name":              "my-db",
				"namespace":         "team-a",
				"creationTimestamp": now.Format(time.RFC3339),
				"annotations": map[string]interface{}{
					"platform.example.org/creator": "alice",
					"platform.example.org/team":    "backend",
					"crossplane.io/paused":         "true",
				},
				"labels": map[string]interface{}{
					"crossplane.io/composition-name": "comp-direct",
				},
			},
			"spec": map[string]interface{}{
				"resourceRef": map[string]interface{}{
					"name": "xr-abc-123",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Synced",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
						"reason": "Available",
					},
				},
			},
		},
	}

	// Set the creation timestamp properly via the typed accessor.
	obj.SetCreationTimestamp(metav1.NewTime(now))
	deletedAt := now.Add(5 * time.Minute)
	obj.SetDeletionTimestamp(&metav1.Time{Time: deletedAt})

	claim := UnstructuredToClaim(*obj, gvr, cfg)

	if claim.GVR != "platform.example.org/v1alpha1/postgresqlinstances" {
		t.Errorf("GVR: got %q", claim.GVR)
	}
	if claim.Group != "platform.example.org" {
		t.Errorf("Group: got %q", claim.Group)
	}
	if claim.Version != "v1alpha1" {
		t.Errorf("Version: got %q", claim.Version)
	}
	if claim.Kind != "PostgreSQLInstance" {
		t.Errorf("Kind: got %q", claim.Kind)
	}
	if claim.Namespace != "team-a" {
		t.Errorf("Namespace: got %q", claim.Namespace)
	}
	if claim.Name != "my-db" {
		t.Errorf("Name: got %q", claim.Name)
	}
	if claim.Creator != "alice" {
		t.Errorf("Creator: got %q", claim.Creator)
	}
	if claim.Team != "backend" {
		t.Errorf("Team: got %q", claim.Team)
	}
	if claim.Composition != "comp-direct" {
		t.Errorf("Composition: got %q", claim.Composition)
	}
	if claim.XRRef != "xr-abc-123" {
		t.Errorf("XRRef: got %q", claim.XRRef)
	}
	if !claim.Ready {
		t.Error("expected Ready=true")
	}
	if !claim.Synced {
		t.Error("expected Synced=true")
	}
	if claim.Reason != "Available" {
		t.Errorf("Reason: got %q", claim.Reason)
	}
	if !claim.Paused {
		t.Error("expected Paused=true")
	}
	if !claim.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: got %v, want %v", claim.CreatedAt, now)
	}
	if !claim.DeletedAt.Equal(deletedAt) {
		t.Errorf("DeletedAt: got %v, want %v", claim.DeletedAt, deletedAt)
	}
}

func TestUnstructuredToClaim_Minimal(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "things"}
	cfg := &config.Config{}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"metadata": map[string]interface{}{
				"name":      "thing-1",
				"namespace": "default",
			},
		},
	}

	claim := UnstructuredToClaim(*obj, gvr, cfg)

	if claim.Name != "thing-1" {
		t.Errorf("Name: got %q", claim.Name)
	}
	if claim.Creator != "" {
		t.Errorf("Creator should be empty, got %q", claim.Creator)
	}
	if claim.Ready {
		t.Error("expected Ready=false when no conditions")
	}
	if claim.XRRef != "" {
		t.Errorf("XRRef should be empty, got %q", claim.XRRef)
	}
}

func TestUnstructuredToClaim_NoKind_FallsBackToResource(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "postgresqlinstances"}
	cfg := &config.Config{}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			// no "kind" field — common in dynamic List results
			"metadata": map[string]interface{}{
				"name":      "db-1",
				"namespace": "ns",
			},
		},
	}

	claim := UnstructuredToClaim(*obj, gvr, cfg)
	if claim.Kind != "Postgresqlinstance" {
		t.Errorf("Kind fallback: got %q", claim.Kind)
	}
}

func TestUnstructuredToXR_Full(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "platform.example.org", Version: "v1alpha1", Resource: "xpostgresqlinstances"}
	cfg := &config.Config{
		CompositionLabelKey: "crossplane.io/composition-name",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "platform.example.org/v1alpha1",
			"kind":       "XPostgreSQLInstance",
			"metadata": map[string]interface{}{
				"name": "xr-abc-123",
				"labels": map[string]interface{}{
					"crossplane.io/composition-name": "production-postgres",
					"crossplane.io/claim-name":       "dbx-ws-aaip-1-1",
					"crossplane.io/claim-namespace":  "data-and-ai",
				},
				"annotations": map[string]interface{}{
					"crossplane.io/paused": "TRUE",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Synced",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Ready",
						"status": "False",
						"reason": "Unavailable",
					},
				},
			},
		},
	}

	xr := UnstructuredToXR(*obj, gvr, cfg)

	if xr.GVR != "platform.example.org/v1alpha1/xpostgresqlinstances" {
		t.Errorf("GVR: got %q", xr.GVR)
	}
	if xr.Version != "v1alpha1" {
		t.Errorf("Version: got %q", xr.Version)
	}
	if xr.Kind != "XPostgreSQLInstance" {
		t.Errorf("Kind: got %q", xr.Kind)
	}
	if xr.Namespace != "" {
		t.Errorf("Namespace should be empty for cluster-scoped XR, got %q", xr.Namespace)
	}
	if xr.Composition != "production-postgres" {
		t.Errorf("Composition: got %q", xr.Composition)
	}
	if xr.ClaimName != "dbx-ws-aaip-1-1" {
		t.Errorf("ClaimName: got %q", xr.ClaimName)
	}
	if xr.ClaimNS != "data-and-ai" {
		t.Errorf("ClaimNS: got %q", xr.ClaimNS)
	}
	if xr.Ready {
		t.Error("expected Ready=false")
	}
	if !xr.Synced {
		t.Error("expected Synced=true")
	}
	if xr.Reason != "Unavailable" {
		t.Errorf("Reason: got %q", xr.Reason)
	}
	if !xr.Paused {
		t.Error("expected Paused=true")
	}
	if !xr.DeletedAt.IsZero() {
		t.Errorf("DeletedAt should be zero, got %v", xr.DeletedAt)
	}
}

func TestUnstructuredToXR_NoConditions(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "g", Version: "v1", Resource: "xthings"}
	cfg := &config.Config{CompositionLabelKey: "crossplane.io/composition-name"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "g/v1",
			"kind":       "XThing",
			"metadata": map[string]interface{}{
				"name": "xr-1",
			},
		},
	}

	xr := UnstructuredToXR(*obj, gvr, cfg)
	if xr.Synced {
		t.Error("expected Synced=false when no status")
	}
	if xr.Ready {
		t.Error("expected Ready=false when no status")
	}
	if xr.Composition != "" {
		t.Errorf("Composition should be empty, got %q", xr.Composition)
	}
}

func TestUnstructuredToXR_V2NamespacedCompositionRef(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "platform.example.org", Version: "v1", Resource: "apps"}
	cfg := &config.Config{
		CompositionLabelKey: "crossplane.io/composition-name",
		XRGVRSScopes: map[string]config.ResourceScope{
			GVRString(gvr): config.ResourceScopeNamespaced,
		},
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "platform.example.org/v1",
		"kind":       "App",
		"metadata": map[string]interface{}{
			"name":      "payments",
			"namespace": "team-a",
		},
		"spec": map[string]interface{}{
			"crossplane": map[string]interface{}{
				"compositionRef": map[string]interface{}{"name": "app-v2"},
			},
		},
	}}

	xr := UnstructuredToXR(*obj, gvr, cfg)
	if xr.Namespace != "team-a" {
		t.Fatalf("expected namespace team-a, got %q", xr.Namespace)
	}
	if xr.Composition != "app-v2" {
		t.Fatalf("expected composition from spec.crossplane.compositionRef, got %q", xr.Composition)
	}
	if !xr.ClaimsUnsupported {
		t.Fatal("v2 namespaced XR must not support claims")
	}
}

func TestUnstructuredToXR_ParentFromOwnerReferences(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "platform.example.org", Version: "v1alpha1", Resource: "xnetworks"}
	cfg := &config.Config{}
	ctrl := true
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "platform.example.org/v1alpha1",
		"kind":       "XNetwork",
		"metadata": map[string]interface{}{
			"name":      "net-1",
			"namespace": "team-a",
		},
	}}
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "platform.example.org/v1alpha1",
		Kind:       "XApp",
		Name:       "app-1",
		Controller: &ctrl,
		UID:        "uid-app-1",
	}})

	xr := UnstructuredToXR(*obj, gvr, cfg)
	if xr.ParentKind != "XApp" || xr.ParentName != "app-1" || xr.ParentGroup != "platform.example.org" || xr.ParentUID != "uid-app-1" {
		t.Fatalf("parent: got kind=%q name=%q group=%q uid=%q", xr.ParentKind, xr.ParentName, xr.ParentGroup, xr.ParentUID)
	}
	if xr.ParentNamespace != "" {
		t.Fatalf("ParentNamespace should stay empty until store enrichment, got %q", xr.ParentNamespace)
	}
}

func TestUnstructuredToXR_IgnoresClaimOwner(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "platform.example.org", Version: "v1alpha1", Resource: "xapps"}
	cfg := &config.Config{}
	ctrl := true
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "platform.example.org/v1alpha1",
		"kind":       "XApp",
		"metadata":   map[string]interface{}{"name": "app-1", "namespace": "team-a"},
	}}
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "platform.example.org/v1alpha1",
		Kind:       "AppClaim",
		Name:       "claim-1",
		Controller: &ctrl,
		UID:        "uid-claim",
	}})

	xr := UnstructuredToXR(*obj, gvr, cfg)
	if xr.ParentName != "" || xr.ParentKind != "" {
		t.Fatalf("claim owners must not become parent_*, got kind=%q name=%q", xr.ParentKind, xr.ParentName)
	}
}

func TestImmediateXROwner_Ambiguous(t *testing.T) {
	refs := []metav1.OwnerReference{
		{APIVersion: "platform.example.org/v1alpha1", Kind: "XApp", Name: "a", UID: "1"},
		{APIVersion: "platform.example.org/v1alpha1", Kind: "XApp", Name: "b", UID: "2"},
	}
	if _, ok := immediateXROwner(refs); ok {
		t.Fatal("expected empty parent when multiple non-controller XR owners")
	}
}

func TestUnstructuredToMR_Full(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "nop.crossplane.io", Version: "v1alpha1", Resource: "nopresources"}
	cfg := &config.Config{
		CompositeLabelKey: "crossplane.io/composite",
	}

	now := time.Now().Truncate(time.Second)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nop.crossplane.io/v1alpha1",
			"kind":       "NopResource",
			"metadata": map[string]interface{}{
				"name":              "nop-abc",
				"namespace":         "default",
				"creationTimestamp": now.Format(time.RFC3339),
				"labels": map[string]interface{}{
					"crossplane.io/composite":       "xr-widget-1",
					"crossplane.io/claim-name":      "widget-a",
					"crossplane.io/claim-namespace": "team-alpha",
				},
				"annotations": map[string]interface{}{
					"crossplane.io/external-name": "cloud-nop-abc",
					"crossplane.io/paused":        "true",
				},
			},
			"spec": map[string]interface{}{
				"providerConfigRef": map[string]interface{}{
					"name": "default",
				},
				"managementPolicies": []interface{}{"Observe", "LateInitialize"},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Synced", "status": "True"},
					map[string]interface{}{"type": "Ready", "status": "True", "reason": "Available"},
				},
			},
		},
	}
	obj.SetCreationTimestamp(metav1.NewTime(now))
	deletedAt := now.Add(2 * time.Minute)
	obj.SetDeletionTimestamp(&metav1.Time{Time: deletedAt})

	mr := UnstructuredToMR(*obj, gvr, cfg, "provider-nop")

	if mr.GVR != "nop.crossplane.io/v1alpha1/nopresources" {
		t.Errorf("GVR: got %q", mr.GVR)
	}
	if mr.Version != "v1alpha1" {
		t.Errorf("Version: got %q", mr.Version)
	}
	if mr.Kind != "NopResource" {
		t.Errorf("Kind: got %q", mr.Kind)
	}
	if mr.XRName != "xr-widget-1" {
		t.Errorf("XRName: got %q", mr.XRName)
	}
	if mr.ClaimName != "widget-a" {
		t.Errorf("ClaimName: got %q", mr.ClaimName)
	}
	if mr.ClaimNS != "team-alpha" {
		t.Errorf("ClaimNS: got %q", mr.ClaimNS)
	}
	if mr.Provider != "provider-nop" {
		t.Errorf("Provider: got %q", mr.Provider)
	}
	if mr.ProviderConfig != "default" {
		t.Errorf("ProviderConfig: got %q", mr.ProviderConfig)
	}
	if mr.ExternalName != "cloud-nop-abc" {
		t.Errorf("ExternalName: got %q", mr.ExternalName)
	}
	if mr.ManagementPolicies != "Observe,LateInitialize" {
		t.Errorf("ManagementPolicies: got %q", mr.ManagementPolicies)
	}
	if !mr.Paused {
		t.Error("expected Paused=true")
	}
	if !mr.Ready || !mr.Synced {
		t.Error("expected Ready and Synced true")
	}
	if mr.Reason != "Available" {
		t.Errorf("Reason: got %q", mr.Reason)
	}
	if !mr.DeletedAt.Equal(deletedAt) {
		t.Errorf("DeletedAt: got %v, want %v", mr.DeletedAt, deletedAt)
	}
}

func TestUnstructuredToPackage_ProviderHealthyInstalled(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "pkg.crossplane.io/v1",
			"kind":       "Provider",
			"metadata": map[string]interface{}{
				"name": "provider-nop",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Installed", "status": "True", "reason": "ActivePackageRevision"},
					map[string]interface{}{"type": "Healthy", "status": "True", "reason": "HealthyPackageRevision"},
				},
			},
		},
	}

	pkg := UnstructuredToPackage(*obj, gvr)
	if pkg.GVR != "pkg.crossplane.io/v1/providers" {
		t.Errorf("GVR: got %q", pkg.GVR)
	}
	if pkg.Kind != "Provider" {
		t.Errorf("Kind: got %q", pkg.Kind)
	}
	if pkg.Name != "provider-nop" {
		t.Errorf("Name: got %q", pkg.Name)
	}
	if !pkg.Healthy {
		t.Error("expected Healthy=true")
	}
	if !pkg.Installed {
		t.Error("expected Installed=true")
	}
}

func TestUnstructuredToPackage_FunctionUnhealthy(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1beta1", Resource: "functions"}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "pkg.crossplane.io/v1beta1",
			"kind":       "Function",
			"metadata": map[string]interface{}{
				"name": "function-patch-and-transform",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Installed", "status": "True"},
					map[string]interface{}{"type": "Healthy", "status": "False", "reason": "UnhealthyPackageRevision"},
				},
			},
		},
	}

	pkg := UnstructuredToPackage(*obj, gvr)
	if pkg.Kind != "Function" {
		t.Errorf("Kind: got %q", pkg.Kind)
	}
	if pkg.Healthy {
		t.Error("expected Healthy=false")
	}
	if !pkg.Installed {
		t.Error("expected Installed=true")
	}
}

func TestUnstructuredToPackage_KindFromResource(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1", Resource: "configurations"}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "pkg.crossplane.io/v1",
			"metadata": map[string]interface{}{
				"name": "platform-config",
			},
		},
	}

	pkg := UnstructuredToPackage(*obj, gvr)
	if pkg.Kind != "Configuration" {
		t.Errorf("Kind: got %q, want Configuration", pkg.Kind)
	}
	if pkg.Healthy || pkg.Installed {
		t.Error("expected Healthy and Installed false when conditions missing")
	}
}

func TestExtractReadyCondition_MultipleConditions(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Synced", "status": "True"},
				map[string]interface{}{"type": "Ready", "status": "True", "reason": "Available"},
				map[string]interface{}{"type": "Healthy", "status": "False"},
			},
		},
	}

	ready, reason := extractReadyCondition(obj)
	if !ready {
		t.Error("expected Ready=true")
	}
	if reason != "Available" {
		t.Errorf("expected reason Available, got %q", reason)
	}
}

func TestExtractReadyCondition_CaseInsensitive(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "true"},
			},
		},
	}

	ready, _ := extractReadyCondition(obj)
	if !ready {
		t.Error("expected Ready=true for lowercase 'true'")
	}
}

func TestExtractReadyCondition_NoConditions(t *testing.T) {
	ready, reason := extractReadyCondition(map[string]interface{}{})
	if ready {
		t.Error("expected Ready=false")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestExtractConditionStatus(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Synced", "status": "True"},
				map[string]interface{}{"type": "Ready", "status": "False"},
			},
		},
	}

	if !extractConditionStatus(obj, "Synced") {
		t.Error("expected Synced=true")
	}
	if extractConditionStatus(obj, "Ready") {
		t.Error("expected Ready=false")
	}
	if extractConditionStatus(obj, "Healthy") {
		t.Error("expected Healthy=false for missing condition")
	}
}

func TestResourceToKind(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"postgresqlinstances", "Postgresqlinstance"},
		{"deployments", "Deployment"},
		{"thing", "Thing"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resourceToKind(tt.input)
			if got != tt.want {
				t.Errorf("resourceToKind(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
