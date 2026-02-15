package config

import (
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestParseGVR_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  schema.GroupVersionResource
	}{
		{
			input: "platform.example.org/v1alpha1/postgresqlinstances",
			want: schema.GroupVersionResource{
				Group: "platform.example.org", Version: "v1alpha1", Resource: "postgresqlinstances",
			},
		},
		{
			input: "apps/v1/deployments",
			want: schema.GroupVersionResource{
				Group: "apps", Version: "v1", Resource: "deployments",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseGVR(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseGVR_Invalid(t *testing.T) {
	tests := []string{
		"",
		"only-one-segment",
		"two/segments",
		"//",
		"group//resource",
		"/version/resource",
		"group/version/",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseGVR(input)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", input)
			}
		})
	}
}

func TestParseGVRs_Valid(t *testing.T) {
	input := "platform.example.org/v1alpha1/postgresqlinstances, platform.example.org/v1alpha1/kafkatopics"
	gvrs, err := ParseGVRs(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gvrs) != 2 {
		t.Fatalf("expected 2 GVRs, got %d", len(gvrs))
	}
	if gvrs[0].Resource != "postgresqlinstances" {
		t.Errorf("expected postgresqlinstances, got %s", gvrs[0].Resource)
	}
	if gvrs[1].Resource != "kafkatopics" {
		t.Errorf("expected kafkatopics, got %s", gvrs[1].Resource)
	}
}

func TestParseGVRs_Empty(t *testing.T) {
	_, err := ParseGVRs("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseGVRs_TrailingComma(t *testing.T) {
	input := "platform.example.org/v1alpha1/postgresqlinstances,"
	gvrs, err := ParseGVRs(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gvrs) != 1 {
		t.Fatalf("expected 1 GVR, got %d", len(gvrs))
	}
}

func TestLoad_Defaults(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS": "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":    "platform.example.org/v1alpha1/xpostgresqlinstances",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.CompositionLabelKey != "crossplane.io/composition-name" {
		t.Errorf("expected default composition label key, got %s", cfg.CompositionLabelKey)
	}
	if cfg.PollIntervalSeconds != 30 {
		t.Errorf("expected default poll interval 30, got %d", cfg.PollIntervalSeconds)
	}
	if cfg.MetricsAddr != ":8080" {
		t.Errorf("expected default metrics addr :8080, got %s", cfg.MetricsAddr)
	}
	if len(cfg.Namespaces) != 0 {
		t.Errorf("expected no namespaces, got %v", cfg.Namespaces)
	}
}

func TestLoad_AllOptions(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS":             "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":                "platform.example.org/v1alpha1/xpostgresqlinstances",
		"KUBE_NAMESPACE_SCOPE":   "team-a, team-b",
		"CREATOR_ANNOTATION_KEY": "platform.example.org/creator",
		"TEAM_ANNOTATION_KEY":    "platform.example.org/team",
		"COMPOSITION_LABEL_KEY":  "custom.io/comp",
		"POLL_INTERVAL_SECONDS":  "15",
		"METRICS_ADDR":           ":9090",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Namespaces) != 2 || cfg.Namespaces[0] != "team-a" || cfg.Namespaces[1] != "team-b" {
		t.Errorf("unexpected namespaces: %v", cfg.Namespaces)
	}
	if cfg.CreatorAnnotationKey != "platform.example.org/creator" {
		t.Errorf("unexpected creator key: %s", cfg.CreatorAnnotationKey)
	}
	if cfg.TeamAnnotationKey != "platform.example.org/team" {
		t.Errorf("unexpected team key: %s", cfg.TeamAnnotationKey)
	}
	if cfg.CompositionLabelKey != "custom.io/comp" {
		t.Errorf("unexpected composition label key: %s", cfg.CompositionLabelKey)
	}
	if cfg.PollIntervalSeconds != 15 {
		t.Errorf("unexpected poll interval: %d", cfg.PollIntervalSeconds)
	}
	if cfg.MetricsAddr != ":9090" {
		t.Errorf("unexpected metrics addr: %s", cfg.MetricsAddr)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	// Clear all env vars
	setEnvs(t, map[string]string{})

	_, err := Load()
	if err == nil {
		t.Error("expected error when CLAIM_GVRS is missing")
	}
}

func TestLoad_InvalidPollInterval(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS":            "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":               "platform.example.org/v1alpha1/xpostgresqlinstances",
		"POLL_INTERVAL_SECONDS": "abc",
	})

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid POLL_INTERVAL_SECONDS")
	}
}

func TestLoad_ZeroPollInterval(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS":            "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":               "platform.example.org/v1alpha1/xpostgresqlinstances",
		"POLL_INTERVAL_SECONDS": "0",
	})

	_, err := Load()
	if err == nil {
		t.Error("expected error for zero POLL_INTERVAL_SECONDS")
	}
}

func TestLoad_StoreBackendDefaults(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS": "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":    "platform.example.org/v1alpha1/xpostgresqlinstances",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StoreBackend != "memory" {
		t.Errorf("expected default store backend 'memory', got %q", cfg.StoreBackend)
	}
	if cfg.S3KeyPrefix != "xp-tracker" {
		t.Errorf("expected default S3 key prefix 'xp-tracker', got %q", cfg.S3KeyPrefix)
	}
	if cfg.S3Region != "us-east-1" {
		t.Errorf("expected default S3 region 'us-east-1', got %q", cfg.S3Region)
	}
	if cfg.S3Bucket != "" {
		t.Errorf("expected empty S3 bucket, got %q", cfg.S3Bucket)
	}
	if cfg.S3Endpoint != "" {
		t.Errorf("expected empty S3 endpoint, got %q", cfg.S3Endpoint)
	}
}

func TestLoad_StoreBackendS3(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS":    "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":       "platform.example.org/v1alpha1/xpostgresqlinstances",
		"STORE_BACKEND": "s3",
		"S3_BUCKET":     "my-snapshots",
		"S3_KEY_PREFIX": "prod/xp",
		"S3_REGION":     "eu-west-1",
		"S3_ENDPOINT":   "http://minio:9000",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StoreBackend != "s3" {
		t.Errorf("expected store backend 's3', got %q", cfg.StoreBackend)
	}
	if cfg.S3Bucket != "my-snapshots" {
		t.Errorf("expected S3 bucket 'my-snapshots', got %q", cfg.S3Bucket)
	}
	if cfg.S3KeyPrefix != "prod/xp" {
		t.Errorf("expected S3 key prefix 'prod/xp', got %q", cfg.S3KeyPrefix)
	}
	if cfg.S3Region != "eu-west-1" {
		t.Errorf("expected S3 region 'eu-west-1', got %q", cfg.S3Region)
	}
	if cfg.S3Endpoint != "http://minio:9000" {
		t.Errorf("expected S3 endpoint 'http://minio:9000', got %q", cfg.S3Endpoint)
	}
}

func TestLoad_StoreBackendS3MissingBucket(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS":    "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":       "platform.example.org/v1alpha1/xpostgresqlinstances",
		"STORE_BACKEND": "s3",
	})

	_, err := Load()
	if err == nil {
		t.Error("expected error when STORE_BACKEND=s3 but S3_BUCKET is missing")
	}
}

func TestLoad_StoreBackendInvalid(t *testing.T) {
	setEnvs(t, map[string]string{
		"CLAIM_GVRS":    "platform.example.org/v1alpha1/postgresqlinstances",
		"XR_GVRS":       "platform.example.org/v1alpha1/xpostgresqlinstances",
		"STORE_BACKEND": "redis",
	})

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid STORE_BACKEND")
	}
}

// setEnvs sets environment variables for the duration of the test and clears
// all config-related env vars first to ensure clean state.
func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	keys := []string{
		"CLAIM_GVRS", "XR_GVRS", "KUBE_NAMESPACE_SCOPE",
		"CREATOR_ANNOTATION_KEY", "TEAM_ANNOTATION_KEY",
		"COMPOSITION_LABEL_KEY", "POLL_INTERVAL_SECONDS", "METRICS_ADDR",
		"STORE_BACKEND", "S3_BUCKET", "S3_KEY_PREFIX", "S3_REGION", "S3_ENDPOINT",
	}
	for _, k := range keys {
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("failed to unset %s: %v", k, err)
		}
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}
}
