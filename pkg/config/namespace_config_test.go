package config

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseNamespaceConfigMap_ValidClaimsOnly(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "team-a-tracker",
			Namespace: "team-a",
		},
		Data: map[string]string{
			"CLAIM_GVRS": "platform.example.org/v1alpha1/postgresqlinstances",
		},
	}

	nsCfg, err := ParseNamespaceConfigMap(cm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nsCfg.Namespace != "team-a" {
		t.Errorf("expected namespace team-a, got %q", nsCfg.Namespace)
	}
	if nsCfg.ConfigMapName != "team-a-tracker" {
		t.Errorf("expected configmap name team-a-tracker, got %q", nsCfg.ConfigMapName)
	}
	if len(nsCfg.ClaimGVRs) != 1 {
		t.Fatalf("expected 1 claim GVR, got %d", len(nsCfg.ClaimGVRs))
	}
	if nsCfg.ClaimGVRs[0].Resource != "postgresqlinstances" {
		t.Errorf("expected postgresqlinstances, got %q", nsCfg.ClaimGVRs[0].Resource)
	}
	if len(nsCfg.XRGVRs) != 0 {
		t.Errorf("expected 0 XR GVRs, got %d", len(nsCfg.XRGVRs))
	}
}

func TestParseNamespaceConfigMap_ValidXRsOnly(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "team-b-tracker",
			Namespace: "team-b",
		},
		Data: map[string]string{
			"XR_GVRS": "platform.example.org/v1alpha1/xpostgresqlinstances",
		},
	}

	nsCfg, err := ParseNamespaceConfigMap(cm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nsCfg.XRGVRs) != 1 {
		t.Fatalf("expected 1 XR GVR, got %d", len(nsCfg.XRGVRs))
	}
	if len(nsCfg.ClaimGVRs) != 0 {
		t.Errorf("expected 0 claim GVRs, got %d", len(nsCfg.ClaimGVRs))
	}
}

func TestParseNamespaceConfigMap_ValidBothGVRs(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "team-c-tracker",
			Namespace: "team-c",
		},
		Data: map[string]string{
			"CLAIM_GVRS": "platform.example.org/v1alpha1/postgresqlinstances, platform.example.org/v1alpha1/kafkatopics",
			"XR_GVRS":    "platform.example.org/v1alpha1/xpostgresqlinstances",
		},
	}

	nsCfg, err := ParseNamespaceConfigMap(cm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nsCfg.ClaimGVRs) != 2 {
		t.Errorf("expected 2 claim GVRs, got %d", len(nsCfg.ClaimGVRs))
	}
	if len(nsCfg.XRGVRs) != 1 {
		t.Errorf("expected 1 XR GVR, got %d", len(nsCfg.XRGVRs))
	}
}

func TestParseNamespaceConfigMap_AnnotationKeyOverrides(t *testing.T) {
	fallback := &Config{
		CreatorAnnotationKey: "default.org/creator",
		TeamAnnotationKey:    "default.org/team",
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "overrides",
			Namespace: "ns",
		},
		Data: map[string]string{
			"CLAIM_GVRS":             "g/v1/things",
			"CREATOR_ANNOTATION_KEY": "custom.org/creator",
			// TEAM_ANNOTATION_KEY not set — should inherit from fallback
		},
	}

	nsCfg, err := ParseNamespaceConfigMap(cm, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nsCfg.CreatorAnnotationKey != "custom.org/creator" {
		t.Errorf("expected custom creator key, got %q", nsCfg.CreatorAnnotationKey)
	}
	if nsCfg.TeamAnnotationKey != "default.org/team" {
		t.Errorf("expected fallback team key, got %q", nsCfg.TeamAnnotationKey)
	}
}

func TestParseNamespaceConfigMap_AnnotationKeyFallbackNoOverride(t *testing.T) {
	fallback := &Config{
		CreatorAnnotationKey: "default.org/creator",
		TeamAnnotationKey:    "default.org/team",
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-overrides",
			Namespace: "ns",
		},
		Data: map[string]string{
			"CLAIM_GVRS": "g/v1/things",
		},
	}

	nsCfg, err := ParseNamespaceConfigMap(cm, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nsCfg.CreatorAnnotationKey != "default.org/creator" {
		t.Errorf("expected fallback creator key, got %q", nsCfg.CreatorAnnotationKey)
	}
	if nsCfg.TeamAnnotationKey != "default.org/team" {
		t.Errorf("expected fallback team key, got %q", nsCfg.TeamAnnotationKey)
	}
}

func TestParseNamespaceConfigMap_NilFallback(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-fallback",
			Namespace: "ns",
		},
		Data: map[string]string{
			"CLAIM_GVRS": "g/v1/things",
		},
	}

	nsCfg, err := ParseNamespaceConfigMap(cm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nsCfg.CreatorAnnotationKey != "" {
		t.Errorf("expected empty creator key, got %q", nsCfg.CreatorAnnotationKey)
	}
	if nsCfg.TeamAnnotationKey != "" {
		t.Errorf("expected empty team key, got %q", nsCfg.TeamAnnotationKey)
	}
}

func TestParseNamespaceConfigMap_EmptyData(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty",
			Namespace: "ns",
		},
		Data: map[string]string{},
	}

	_, err := ParseNamespaceConfigMap(cm, nil)
	if err == nil {
		t.Error("expected error for ConfigMap with no GVRs")
	}
}

func TestParseNamespaceConfigMap_NilData(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nil-data",
			Namespace: "ns",
		},
	}

	_, err := ParseNamespaceConfigMap(cm, nil)
	if err == nil {
		t.Error("expected error for ConfigMap with nil data")
	}
}

func TestParseNamespaceConfigMap_NilConfigMap(t *testing.T) {
	_, err := ParseNamespaceConfigMap(nil, nil)
	if err == nil {
		t.Error("expected error for nil ConfigMap")
	}
}

func TestParseNamespaceConfigMap_InvalidClaimGVR(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-gvr",
			Namespace: "ns",
		},
		Data: map[string]string{
			"CLAIM_GVRS": "invalid-gvr-format",
		},
	}

	_, err := ParseNamespaceConfigMap(cm, nil)
	if err == nil {
		t.Error("expected error for invalid CLAIM_GVRS")
	}
}

func TestParseNamespaceConfigMap_InvalidXRGVR(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-xr-gvr",
			Namespace: "ns",
		},
		Data: map[string]string{
			"XR_GVRS": "only/two",
		},
	}

	_, err := ParseNamespaceConfigMap(cm, nil)
	if err == nil {
		t.Error("expected error for invalid XR_GVRS")
	}
}
