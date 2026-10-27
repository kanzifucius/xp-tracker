package kube

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		GVR:               GVRString(gvr),
		Group:             gvr.Group,
		Version:           gvr.Version,
		Kind:              obj.GetKind(),
		Namespace:         obj.GetNamespace(),
		Name:              obj.GetName(),
		UID:               string(obj.GetUID()),
		CreatedAt:         obj.GetCreationTimestamp().Time,
		ClaimsUnsupported: !xrSupportsClaims(cfg, gvr),
	}

	if xr.Kind == "" {
		xr.Kind = resourceToKind(gvr.Resource)
	}

	// Extract composition label.
	labels := obj.GetLabels()
	if cfg.CompositionLabelKey != "" {
		xr.Composition = labels[cfg.CompositionLabelKey]
	}
	if xr.Composition == "" {
		xr.Composition = nestedString(obj.Object, "spec", "crossplane", "compositionRef", "name")
	}
	xr.ClaimName = labels["crossplane.io/claim-name"]
	xr.ClaimNS = labels["crossplane.io/claim-namespace"]

	xr.Paused = isPaused(obj)
	xr.DeletedAt = deletionTimestamp(obj)

	// Immediate XR parent from ownerReferences (validated later against inventory).
	if parent, ok := immediateXROwner(obj.GetOwnerReferences()); ok {
		xr.ParentKind = parent.Kind
		xr.ParentName = parent.Name
		xr.ParentGroup = parent.Group
		xr.ParentUID = parent.UID
	}
	xr.ResourceRefs = extractResourceRefs(obj.Object)

	// Extract standard Crossplane status conditions.
	xr.Synced = extractConditionStatus(obj.Object, "Synced")
	xr.Ready, xr.Reason = extractReadyCondition(obj.Object)

	return xr
}

// xrOwnerRef is a candidate immediate XR parent from metadata.ownerReferences.
type xrOwnerRef struct {
	Group string
	Kind  string
	Name  string
	UID   string
}

// immediateXROwner selects the immediate XR-typed owner from ownerReferences.
// Preference: controller:true XR-like ref; else exactly one XR-like ref.
// Claims and core Kubernetes kinds are ignored. Ambiguous → ok=false.
// Owner UID is preserved so store enrichment can reject same-name replacements.
func immediateXROwner(refs []metav1.OwnerReference) (xrOwnerRef, bool) {
	var candidates []xrOwnerRef
	var controller *xrOwnerRef
	for _, ref := range refs {
		if !isXRLikeOwner(ref) {
			continue
		}
		cand := xrOwnerRef{
			Group: apiGroupFromAPIVersion(ref.APIVersion),
			Kind:  ref.Kind,
			Name:  ref.Name,
			UID:   string(ref.UID),
		}
		candidates = append(candidates, cand)
		if ref.Controller != nil && *ref.Controller {
			c := cand
			controller = &c
		}
	}
	if controller != nil {
		return *controller, true
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return xrOwnerRef{}, false
}

// isXRLikeOwner reports whether an ownerRef could be a Crossplane XR parent.
// Claims stay on claim_* labels and are never treated as parent_*.
func isXRLikeOwner(ref metav1.OwnerReference) bool {
	if ref.Name == "" || ref.Kind == "" {
		return false
	}
	if strings.HasSuffix(ref.Kind, "Claim") {
		return false
	}
	group := apiGroupFromAPIVersion(ref.APIVersion)
	return !isNonXROwnerGroup(group)
}

func apiGroupFromAPIVersion(apiVersion string) string {
	if apiVersion == "" {
		return ""
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		// Fall back to the substring before "/" when present.
		if i := strings.IndexByte(apiVersion, '/'); i >= 0 {
			return apiVersion[:i]
		}
		return ""
	}
	return gv.Group
}

// isNonXROwnerGroup lists API groups that are never XR composites.
func isNonXROwnerGroup(group string) bool {
	// Empty group covers core resources (apiVersion "v1").
	switch group {
	case "":
		return true
	case "apps", "batch", "autoscaling", "policy", "rbac.authorization.k8s.io",
		"networking.k8s.io", "coordination.k8s.io", "admissionregistration.k8s.io",
		"apiextensions.k8s.io", "storage.k8s.io", "scheduling.k8s.io",
		"certificates.k8s.io", "discovery.k8s.io", "events.k8s.io",
		"flowcontrol.apiserver.k8s.io", "node.k8s.io",
		"pkg.crossplane.io", "apiextensions.crossplane.io",
		"protection.crossplane.io", "ops.crossplane.io":
		return true
	default:
		return false
	}
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

	// Immediate composing XR from ownerReferences (validated / fallback in store).
	// Do not copy xr_name here — composite label and immediate owner may diverge.
	if owner, ok := immediateXROwner(obj.GetOwnerReferences()); ok {
		mr.OwnerXRKind = owner.Kind
		mr.OwnerXRName = owner.Name
		mr.OwnerXRGroup = owner.Group
	}

	mr.Synced = extractConditionStatus(obj.Object, "Synced")
	mr.Ready, mr.Reason = extractReadyCondition(obj.Object)

	return mr
}

// extractResourceRefs reads composed child refs from legacy spec.resourceRefs
// or v2 spec.crossplane.resourceRefs.
func extractResourceRefs(obj map[string]interface{}) []store.ComposedRef {
	refs := nestedObjectSlice(obj, "spec", "resourceRefs")
	if len(refs) == 0 {
		refs = nestedObjectSlice(obj, "spec", "crossplane", "resourceRefs")
	}
	if len(refs) == 0 {
		return nil
	}
	out := make([]store.ComposedRef, 0, len(refs))
	for _, ref := range refs {
		cr := store.ComposedRef{
			APIVersion: asString(ref["apiVersion"]),
			Kind:       asString(ref["kind"]),
			Name:       asString(ref["name"]),
		}
		if cr.Name == "" && cr.Kind == "" {
			continue
		}
		out = append(out, cr)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func nestedObjectSlice(obj map[string]interface{}, fields ...string) []map[string]interface{} {
	raw, found, err := unstructured.NestedSlice(obj, fields...)
	if err != nil || !found {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
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

// UnstructuredToPackage converts an unstructured Provider, Configuration, or
// Function object into a PackageInfo. Healthy and Installed come from the
// standard pkg.crossplane.io conditions.
func UnstructuredToPackage(obj unstructured.Unstructured, gvr schema.GroupVersionResource) store.PackageInfo {
	pkg := store.PackageInfo{
		GVR:     GVRString(gvr),
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    obj.GetKind(),
		Name:    obj.GetName(),
	}
	if pkg.Kind == "" {
		pkg.Kind = resourceToKind(gvr.Resource)
		// resourceToKind is a rough heuristic ("providers" -> "Provider");
		// normalise common package plurals explicitly.
		switch gvr.Resource {
		case "providers":
			pkg.Kind = "Provider"
		case "configurations":
			pkg.Kind = "Configuration"
		case "functions":
			pkg.Kind = "Function"
		}
	}
	pkg.Healthy = extractConditionStatus(obj.Object, "Healthy")
	pkg.Installed = extractConditionStatus(obj.Object, "Installed")
	return pkg
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

func xrSupportsClaims(cfg *config.Config, gvr schema.GroupVersionResource) bool {
	return cfg.XRSupportsClaims(gvr)
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
