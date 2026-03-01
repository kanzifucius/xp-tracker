package config

import (
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// ConfigMapLabelKey is the label key used to discover per-namespace ConfigMaps.
	ConfigMapLabelKey = "xp-tracker.kanzi.io/config"

	// ConfigMapLabelValue is the expected label value for per-namespace ConfigMaps.
	ConfigMapLabelValue = "gvrs"
)

// NamespaceConfig holds parsed configuration from a per-namespace ConfigMap.
// Claims are polled only within the specified Namespace; XRs are polled cluster-wide.
type NamespaceConfig struct {
	// Namespace is the namespace this configuration applies to (from the ConfigMap's metadata).
	Namespace string

	// ConfigMapName is the name of the source ConfigMap (for logging/debugging).
	ConfigMapName string

	// ClaimGVRs is the list of claim GroupVersionResources to poll in this namespace.
	ClaimGVRs []schema.GroupVersionResource

	// XRGVRs is the list of composite resource GroupVersionResources to poll cluster-wide.
	XRGVRs []schema.GroupVersionResource

	// CreatorAnnotationKey overrides the central config's creator annotation key.
	// Empty means inherit from the central config.
	CreatorAnnotationKey string

	// TeamAnnotationKey overrides the central config's team annotation key.
	// Empty means inherit from the central config.
	TeamAnnotationKey string
}

// ParseNamespaceConfigMap parses a per-namespace ConfigMap into a NamespaceConfig.
// The ConfigMap must contain at least one of CLAIM_GVRS or XR_GVRS.
// The fallback Config is used to inherit annotation keys when not specified in the ConfigMap.
func ParseNamespaceConfigMap(cm *corev1.ConfigMap, fallback *Config) (*NamespaceConfig, error) {
	if cm == nil {
		return nil, fmt.Errorf("ConfigMap is nil")
	}

	nsCfg := &NamespaceConfig{
		Namespace:     cm.Namespace,
		ConfigMapName: cm.Name,
	}

	data := cm.Data
	if data == nil {
		data = map[string]string{}
	}

	// Parse CLAIM_GVRS.
	if raw, ok := data["CLAIM_GVRS"]; ok && raw != "" {
		gvrs, err := ParseGVRs(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid CLAIM_GVRS in ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
		}
		nsCfg.ClaimGVRs = gvrs
	}

	// Parse XR_GVRS.
	if raw, ok := data["XR_GVRS"]; ok && raw != "" {
		gvrs, err := ParseGVRs(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid XR_GVRS in ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
		}
		nsCfg.XRGVRs = gvrs
	}

	// At least one GVR list must be non-empty.
	if len(nsCfg.ClaimGVRs) == 0 && len(nsCfg.XRGVRs) == 0 {
		return nil, fmt.Errorf("ConfigMap %s/%s must specify at least one of CLAIM_GVRS or XR_GVRS", cm.Namespace, cm.Name)
	}

	// Annotation key overrides — fall back to central config if not specified.
	nsCfg.CreatorAnnotationKey = data["CREATOR_ANNOTATION_KEY"]
	if nsCfg.CreatorAnnotationKey == "" && fallback != nil {
		nsCfg.CreatorAnnotationKey = fallback.CreatorAnnotationKey
	}

	nsCfg.TeamAnnotationKey = data["TEAM_ANNOTATION_KEY"]
	if nsCfg.TeamAnnotationKey == "" && fallback != nil {
		nsCfg.TeamAnnotationKey = fallback.TeamAnnotationKey
	}

	slog.Info("parsed namespace config",
		"namespace", nsCfg.Namespace,
		"configmap", nsCfg.ConfigMapName,
		"claim_gvrs", len(nsCfg.ClaimGVRs),
		"xr_gvrs", len(nsCfg.XRGVRs),
	)

	return nsCfg, nil
}
