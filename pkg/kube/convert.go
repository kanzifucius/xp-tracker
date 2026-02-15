package kube

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// GVRString returns the "group/version/resource" string for a GVR.
func GVRString(gvr schema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}

// UnstructuredToClaim converts an unstructured Kubernetes object to a ClaimInfo.
func UnstructuredToClaim(obj unstructured.Unstructured, gvr schema.GroupVersionResource, cfg *config.Config) store.ClaimInfo {
	claim := store.ClaimInfo{
		GVR:       GVRString(gvr),
		Group:     gvr.Group,
		Kind:      obj.GetKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		CreatedAt: obj.GetCreationTimestamp().Time,
	}

	// If Kind is empty (common with dynamic client List results), derive from resource name.
	if claim.Kind == "" {
		claim.Kind = resourceToKind(gvr.Resource)
	}

	// Extract creator annotation.
	if cfg.CreatorAnnotationKey != "" {
		claim.Creator = obj.GetAnnotations()[cfg.CreatorAnnotationKey]
	}

	// Extract team annotation.
	if cfg.TeamAnnotationKey != "" {
		claim.Team = obj.GetAnnotations()[cfg.TeamAnnotationKey]
	}

	// Extract spec.resourceRef.name for composition enrichment.
	claim.XRRef = nestedString(obj.Object, "spec", "resourceRef", "name")

	// Extract composition from labels on the claim itself (some setups label claims directly).
	if cfg.CompositionLabelKey != "" {
		if comp := obj.GetLabels()[cfg.CompositionLabelKey]; comp != "" {
			claim.Composition = comp
		}
	}

	// Extract Ready condition.
	claim.Ready, claim.Reason = extractReadyCondition(obj.Object)

	return claim
}

// UnstructuredToXR converts an unstructured Kubernetes object to an XRInfo.
func UnstructuredToXR(obj unstructured.Unstructured, gvr schema.GroupVersionResource, cfg *config.Config) store.XRInfo {
	xr := store.XRInfo{
		GVR:       GVRString(gvr),
		Group:     gvr.Group,
		Kind:      obj.GetKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		CreatedAt: obj.GetCreationTimestamp().Time,
	}

	if xr.Kind == "" {
		xr.Kind = resourceToKind(gvr.Resource)
	}

	// Extract composition label.
	if cfg.CompositionLabelKey != "" {
		xr.Composition = obj.GetLabels()[cfg.CompositionLabelKey]
	}

	// Extract Ready condition.
	xr.Ready, xr.Reason = extractReadyCondition(obj.Object)

	return xr
}

// extractReadyCondition finds the "Ready" condition in status.conditions and returns
// (ready bool, reason string).
func extractReadyCondition(obj map[string]interface{}) (bool, string) {
	conditions, found, err := unstructured.NestedSlice(obj, "status", "conditions")
	if err != nil || !found {
		return false, ""
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		if condType != "Ready" {
			continue
		}
		status, _ := cond["status"].(string)
		reason, _ := cond["reason"].(string)
		return strings.EqualFold(status, "True"), reason
	}

	return false, ""
}

// nestedString safely extracts a nested string field from an unstructured object.
func nestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}
	return val
}

// resourceToKind converts a plural lowercase resource name to a PascalCase kind.
// e.g. "postgresqlinstances" -> "Postgresqlinstance".
// This is a rough heuristic; the real kind comes from the API server.
func resourceToKind(resource string) string {
	if resource == "" {
		return ""
	}
	// Remove trailing 's' if present (simple heuristic).
	kind := strings.TrimSuffix(resource, "s")

	// Capitalise first letter.
	return strings.ToUpper(kind[:1]) + kind[1:]
}
