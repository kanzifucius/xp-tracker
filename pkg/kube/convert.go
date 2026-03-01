package kube

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// ConvertKeys holds the annotation/label keys needed during object conversion.
// This allows the poller to supply per-namespace overrides without passing the
// entire Config.
type ConvertKeys struct {
	CreatorAnnotationKey string
	TeamAnnotationKey    string
	CompositionLabelKey  string
	Source               string // "central" or "namespace" — propagated to ClaimInfo/XRInfo
}

// KeysFromConfig builds a ConvertKeys from a central Config.
func KeysFromConfig(cfg *config.Config) ConvertKeys {
	return ConvertKeys{
		CreatorAnnotationKey: cfg.CreatorAnnotationKey,
		TeamAnnotationKey:    cfg.TeamAnnotationKey,
		CompositionLabelKey:  cfg.CompositionLabelKey,
		Source:               "central",
	}
}

// KeysFromNamespaceConfig builds a ConvertKeys from a NamespaceConfig,
// falling back to the central Config for CompositionLabelKey (which is
// not overridable per-namespace).
func KeysFromNamespaceConfig(nsCfg *config.NamespaceConfig, centralCfg *config.Config) ConvertKeys {
	keys := ConvertKeys{
		CreatorAnnotationKey: nsCfg.CreatorAnnotationKey,
		TeamAnnotationKey:    nsCfg.TeamAnnotationKey,
		Source:               "namespace",
	}
	if centralCfg != nil {
		keys.CompositionLabelKey = centralCfg.CompositionLabelKey
	}
	return keys
}

// GVRString returns the "group/version/resource" string for a GVR.
func GVRString(gvr schema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}

// UnstructuredToClaim converts an unstructured Kubernetes object to a ClaimInfo.
func UnstructuredToClaim(obj unstructured.Unstructured, gvr schema.GroupVersionResource, cfg *config.Config) store.ClaimInfo {
	return UnstructuredToClaimWithKeys(obj, gvr, KeysFromConfig(cfg))
}

// UnstructuredToClaimWithKeys converts an unstructured Kubernetes object to a ClaimInfo
// using the specified annotation/label keys.
func UnstructuredToClaimWithKeys(obj unstructured.Unstructured, gvr schema.GroupVersionResource, keys ConvertKeys) store.ClaimInfo {
	claim := store.ClaimInfo{
		GVR:       GVRString(gvr),
		Group:     gvr.Group,
		Kind:      obj.GetKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Source:    keys.Source,
		CreatedAt: obj.GetCreationTimestamp().Time,
	}

	// If Kind is empty (common with dynamic client List results), derive from resource name.
	if claim.Kind == "" {
		claim.Kind = resourceToKind(gvr.Resource)
	}

	// Extract creator annotation.
	if keys.CreatorAnnotationKey != "" {
		claim.Creator = obj.GetAnnotations()[keys.CreatorAnnotationKey]
	}

	// Extract team annotation.
	if keys.TeamAnnotationKey != "" {
		claim.Team = obj.GetAnnotations()[keys.TeamAnnotationKey]
	}

	// Extract spec.resourceRef.name for composition enrichment.
	claim.XRRef = nestedString(obj.Object, "spec", "resourceRef", "name")

	// Extract composition from labels on the claim itself (some setups label claims directly).
	if keys.CompositionLabelKey != "" {
		if comp := obj.GetLabels()[keys.CompositionLabelKey]; comp != "" {
			claim.Composition = comp
		}
	}

	// Extract Ready condition.
	claim.Ready, claim.Reason = extractReadyCondition(obj.Object)

	return claim
}

// UnstructuredToXR converts an unstructured Kubernetes object to an XRInfo.
func UnstructuredToXR(obj unstructured.Unstructured, gvr schema.GroupVersionResource, cfg *config.Config) store.XRInfo {
	return UnstructuredToXRWithKeys(obj, gvr, KeysFromConfig(cfg))
}

// UnstructuredToXRWithKeys converts an unstructured Kubernetes object to an XRInfo
// using the specified annotation/label keys.
func UnstructuredToXRWithKeys(obj unstructured.Unstructured, gvr schema.GroupVersionResource, keys ConvertKeys) store.XRInfo {
	xr := store.XRInfo{
		GVR:       GVRString(gvr),
		Group:     gvr.Group,
		Kind:      obj.GetKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Source:    keys.Source,
		CreatedAt: obj.GetCreationTimestamp().Time,
	}

	if xr.Kind == "" {
		xr.Kind = resourceToKind(gvr.Resource)
	}

	// Extract composition label.
	if keys.CompositionLabelKey != "" {
		xr.Composition = obj.GetLabels()[keys.CompositionLabelKey]
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
