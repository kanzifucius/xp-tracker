// Package config loads and validates exporter configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Config holds all runtime configuration for the exporter.
type Config struct {
	// ClaimGVRs is the list of claim GroupVersionResources to poll.
	ClaimGVRs []schema.GroupVersionResource

	// XRGVRs is the list of composite resource GroupVersionResources to poll.
	XRGVRs []schema.GroupVersionResource

	// Namespaces restricts watches to these namespaces. Empty means all.
	Namespaces []string

	// CreatorAnnotationKey is the annotation key used to identify the claim creator.
	CreatorAnnotationKey string

	// TeamAnnotationKey is the annotation key used to identify the team.
	TeamAnnotationKey string

	// CompositionLabelKey is the label key on XRs identifying the Composition.
	CompositionLabelKey string

	// PollIntervalSeconds is the number of seconds between polling cycles.
	PollIntervalSeconds int

	// MetricsAddr is the listen address for the HTTP metrics server.
	MetricsAddr string

	// StoreBackend selects the persistent store backend.
	// Valid values: "memory" (default, no persistence), "s3".
	StoreBackend string

	// S3Bucket is the S3 bucket for persistent snapshots. Required when StoreBackend is "s3".
	S3Bucket string

	// S3KeyPrefix is the key prefix for S3 snapshots. Default: "xp-tracker".
	S3KeyPrefix string

	// S3Region is the AWS region for the S3 client. Default: "us-east-1".
	S3Region string

	// S3Endpoint is an optional custom S3 endpoint URL (for MinIO, LocalStack, etc.).
	S3Endpoint string
}

const (
	defaultCompositionLabelKey = "crossplane.io/composition-name"
	defaultPollInterval        = 30
	defaultMetricsAddr         = ":8080"
	defaultStoreBackend        = "memory"
	defaultS3KeyPrefix         = "xp-tracker"
	defaultS3Region            = "us-east-1"
)

// Load reads configuration from environment variables and returns a validated Config.
func Load() (*Config, error) {
	cfg := &Config{
		CompositionLabelKey: defaultCompositionLabelKey,
		PollIntervalSeconds: defaultPollInterval,
		MetricsAddr:         defaultMetricsAddr,
	}

	// Optional: CLAIM_GVRS (was required; now optional to support namespace-scoped ConfigMap discovery).
	if claimRaw := os.Getenv("CLAIM_GVRS"); claimRaw != "" {
		claimGVRs, err := ParseGVRs(claimRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid CLAIM_GVRS: %w", err)
		}
		cfg.ClaimGVRs = claimGVRs
	}

	// Optional: XR_GVRS (was required; now optional to support namespace-scoped ConfigMap discovery).
	if xrRaw := os.Getenv("XR_GVRS"); xrRaw != "" {
		xrGVRs, err := ParseGVRs(xrRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid XR_GVRS: %w", err)
		}
		cfg.XRGVRs = xrGVRs
	}

	// Optional: KUBE_NAMESPACE_SCOPE
	if ns := os.Getenv("KUBE_NAMESPACE_SCOPE"); ns != "" {
		cfg.Namespaces = splitAndTrim(ns)
	}

	// Optional: CREATOR_ANNOTATION_KEY
	cfg.CreatorAnnotationKey = os.Getenv("CREATOR_ANNOTATION_KEY")

	// Optional: TEAM_ANNOTATION_KEY
	cfg.TeamAnnotationKey = os.Getenv("TEAM_ANNOTATION_KEY")

	// Optional: COMPOSITION_LABEL_KEY
	if v := os.Getenv("COMPOSITION_LABEL_KEY"); v != "" {
		cfg.CompositionLabelKey = v
	}

	// Optional: POLL_INTERVAL_SECONDS
	if v := os.Getenv("POLL_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("POLL_INTERVAL_SECONDS must be a positive integer, got %q", v)
		}
		cfg.PollIntervalSeconds = n
	}

	// Optional: METRICS_ADDR
	if v := os.Getenv("METRICS_ADDR"); v != "" {
		cfg.MetricsAddr = v
	}

	// Optional: STORE_BACKEND
	cfg.StoreBackend = defaultStoreBackend
	if v := os.Getenv("STORE_BACKEND"); v != "" {
		cfg.StoreBackend = v
	}
	switch cfg.StoreBackend {
	case "memory", "s3":
		// valid
	default:
		return nil, fmt.Errorf("STORE_BACKEND must be \"memory\" or \"s3\", got %q", cfg.StoreBackend)
	}

	// S3 configuration (required when STORE_BACKEND=s3, ignored otherwise).
	cfg.S3Bucket = os.Getenv("S3_BUCKET")
	cfg.S3KeyPrefix = defaultS3KeyPrefix
	if v := os.Getenv("S3_KEY_PREFIX"); v != "" {
		cfg.S3KeyPrefix = v
	}
	cfg.S3Region = defaultS3Region
	if v := os.Getenv("S3_REGION"); v != "" {
		cfg.S3Region = v
	}
	cfg.S3Endpoint = os.Getenv("S3_ENDPOINT")

	if cfg.StoreBackend == "s3" && cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required when STORE_BACKEND=s3")
	}

	// Validate S3 key prefix.
	if strings.Contains(cfg.S3KeyPrefix, "..") {
		return nil, fmt.Errorf("S3_KEY_PREFIX must not contain '..', got %q", cfg.S3KeyPrefix)
	}
	cfg.S3KeyPrefix = strings.Trim(cfg.S3KeyPrefix, "/")
	if cfg.S3KeyPrefix == "" {
		cfg.S3KeyPrefix = defaultS3KeyPrefix
	}

	return cfg, nil
}

// ParseGVRs parses a comma-separated list of GVR strings in the format "group/version/resource".
// Each segment must be non-empty. Duplicate GVRs are silently deduplicated with a warning log.
func ParseGVRs(raw string) ([]schema.GroupVersionResource, error) {
	parts := splitAndTrim(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty GVR list")
	}

	seen := make(map[string]struct{}, len(parts))
	gvrs := make([]schema.GroupVersionResource, 0, len(parts))
	for _, p := range parts {
		gvr, err := ParseGVR(p)
		if err != nil {
			return nil, err
		}
		key := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
		if _, dup := seen[key]; dup {
			// key is derived from parsed GVR (group/version/resource), not user-controlled input
			slog.Warn("duplicate GVR ignored", "gvr", key) // #nosec G706
			continue
		}
		seen[key] = struct{}{}
		gvrs = append(gvrs, gvr)
	}
	return gvrs, nil
}

// ParseGVR parses a single GVR string in the format "group/version/resource".
func ParseGVR(s string) (schema.GroupVersionResource, error) {
	segments := strings.SplitN(s, "/", 3)
	if len(segments) != 3 {
		return schema.GroupVersionResource{}, fmt.Errorf("invalid GVR %q: expected format group/version/resource", s)
	}
	for i, seg := range segments {
		segments[i] = strings.TrimSpace(seg)
		if segments[i] == "" {
			return schema.GroupVersionResource{}, fmt.Errorf("invalid GVR %q: segment %d is empty", s, i+1)
		}
	}
	return schema.GroupVersionResource{
		Group:    segments[0],
		Version:  segments[1],
		Resource: segments[2],
	}, nil
}

// splitAndTrim splits s by comma and trims whitespace from each part,
// discarding empty entries.
func splitAndTrim(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}
