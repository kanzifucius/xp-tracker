package kube

import (
	"strings"
	"time"

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
		Version:   gvr.Version,
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

	claim.Paused = isPaused(obj)
	claim.DeletedAt = deletionTimestamp(obj)

	// Extract spec.resourceRef.name for composition enrichment.
	claim.XRRef = nestedString(obj.Object, "spec", "resourceRef", "name")

	// Extract composition from labels on the claim itself (some setups label claims directly).
	if cfg.CompositionLabelKey != "" {
		if comp := obj.GetLabels()[cfg.CompositionLabelKey]; comp != "" {
			claim.Composition = comp
		}
	}

	// Extract standard Crossplane status conditions.
	claim.Synced = extractConditionStatus(obj.Object, "Synced")
	claim.Ready, claim.Reason = extractReadyCondition(obj.Object)

	return claim
}

// UnstructuredToXR converts an unstructured Kubernetes object to an XRInfo.
func UnstructuredToXR(obj unstructured.Unstructured, gvr schema.GroupVersionResource, cfg *config.Config) store.XRInfo {
	xr := store.XRInfo{
		GVR:       GVRString(gvr),
		Group:     gvr.Group,
		Version:   gvr.Version,
		Kind:      obj.GetKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		CreatedAt: obj.GetCreationTimestamp().Time,
	}

	if xr.Kind == "" {
		xr.Kind = resourceToKind(gvr.Resource)
	}

	// Extract composition label.
	labels := obj.GetLabels()
	if cfg.CompositionLabelKey != "" {
		xr.Composition = labels[cfg.CompositionLabelKey]
	}
	xr.ClaimName = labels["crossplane.io/claim-name"]
	xr.ClaimNS = labels["crossplane.io/claim-namespace"]

	xr.Paused = isPaused(obj)
	xr.DeletedAt = deletionTimestamp(obj)

	// Extract standard Crossplane status conditions.
	xr.Synced = extractConditionStatus(obj.Object, "Synced")
	xr.Ready, xr.Reason = extractReadyCondition(obj.Object)

	return xr
}

// UnstructuredToMR converts an unstructured Kubernetes object to an MRInfo.
func UnstructuredToMR(obj unstructured.Unstructured, gvr schema.GroupVersionResource, cfg *config.Config, provider string) store.MRInfo {
	mr := store.MRInfo{
		GVR:       GVRString(gvr),
		Group:     gvr.Group,
		Version:   gvr.Version,
		Kind:      obj.GetKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Provider:  provider,
		CreatedAt: obj.GetCreationTimestamp().Time,
	}

	if mr.Kind == "" {
		mr.Kind = resourceToKind(gvr.Resource)
	}

	labels := obj.GetLabels()
	if cfg.CompositeLabelKey != "" {
		mr.XRName = labels[cfg.CompositeLabelKey]
	}
	mr.ClaimName = labels["crossplane.io/claim-name"]
	mr.ClaimNS = labels["crossplane.io/claim-namespace"]

	annotations := obj.GetAnnotations()
	mr.ExternalName = annotations["crossplane.io/external-name"]
	mr.Paused = isPaused(obj)
	mr.DeletedAt = deletionTimestamp(obj)
	mr.ProviderConfig = nestedString(obj.Object, "spec", "providerConfigRef", "name")
	mr.ManagementPolicies = nestedStringSliceJoined(obj.Object, "spec", "managementPolicies")

	mr.Synced = extractConditionStatus(obj.Object, "Synced")
	mr.Ready, mr.Reason = extractReadyCondition(obj.Object)

	return mr
}

// isPaused reports whether the crossplane.io/paused annotation is set to true.
func isPaused(obj unstructured.Unstructured) bool {
	return strings.EqualFold(obj.GetAnnotations()["crossplane.io/paused"], "true")
}

// deletionTimestamp returns metadata.deletionTimestamp, or zero when not deleting.
func deletionTimestamp(obj unstructured.Unstructured) time.Time {
	ts := obj.GetDeletionTimestamp()
	if ts == nil {
		return time.Time{}
	}
	return ts.Time
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

// extractConditionStatus finds a condition type in status.conditions and returns whether its status is True.
func extractConditionStatus(obj map[string]interface{}, conditionType string) bool {
	conditions, found, err := unstructured.NestedSlice(obj, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		if condType != conditionType {
			continue
		}
		status, _ := cond["status"].(string)
		return strings.EqualFold(status, "True")
	}

	return false
}

// nestedString safely extracts a nested string field from an unstructured object.
func nestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}
	return val
}

// nestedStringSliceJoined extracts a nested string slice and joins it with commas.
func nestedStringSliceJoined(obj map[string]interface{}, fields ...string) string {
	vals, found, err := unstructured.NestedStringSlice(obj, fields...)
	if err != nil || !found || len(vals) == 0 {
		return ""
	}
	return strings.Join(vals, ",")
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
