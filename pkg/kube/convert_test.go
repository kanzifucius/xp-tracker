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

	claim := UnstructuredToClaim(*obj, gvr, cfg)

	if claim.GVR != "platform.example.org/v1alpha1/postgresqlinstances" {
		t.Errorf("GVR: got %q", claim.GVR)
	}
	if claim.Group != "platform.example.org" {
		t.Errorf("Group: got %q", claim.Group)
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
	if claim.Reason != "Available" {
		t.Errorf("Reason: got %q", claim.Reason)
	}
	if !claim.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: got %v, want %v", claim.CreatedAt, now)
	}
	if claim.Source != "central" {
		t.Errorf("Source: got %q, want %q", claim.Source, "central")
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
	if claim.Source != "central" {
		t.Errorf("Source: got %q, want %q", claim.Source, "central")
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
	if xr.Kind != "XPostgreSQLInstance" {
		t.Errorf("Kind: got %q", xr.Kind)
	}
	if xr.Namespace != "" {
		t.Errorf("Namespace should be empty for cluster-scoped XR, got %q", xr.Namespace)
	}
	if xr.Composition != "production-postgres" {
		t.Errorf("Composition: got %q", xr.Composition)
	}
	if xr.Ready {
		t.Error("expected Ready=false")
	}
	if xr.Reason != "Unavailable" {
		t.Errorf("Reason: got %q", xr.Reason)
	}
	if xr.Source != "central" {
		t.Errorf("Source: got %q, want %q", xr.Source, "central")
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
	if xr.Ready {
		t.Error("expected Ready=false when no status")
	}
	if xr.Composition != "" {
		t.Errorf("Composition should be empty, got %q", xr.Composition)
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
