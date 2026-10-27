// Package config loads and validates exporter configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceScope describes whether a Kubernetes resource is namespace-scoped or
// cluster-scoped.
type ResourceScope string

const (
	// ResourceScopeNamespaced identifies resources that exist in Kubernetes namespaces.
	ResourceScopeNamespaced ResourceScope = "Namespaced"
	// ResourceScopeCluster identifies resources that exist at cluster scope.
	ResourceScopeCluster ResourceScope = "Cluster"
	// ResourceScopeLegacyCluster identifies v1-compatible cluster-scoped resources.
	ResourceScopeLegacyCluster ResourceScope = "LegacyCluster"
)

// Config holds all runtime configuration for the exporter.
//
// GVR inventory fields (ClaimGVRs, XRGVRs, MRGVRs, and related scope/provider
// maps) are guarded by mu so periodic XRD/MRD re-discovery can replace them
// while the poller is running. Use SnapshotPollTargets / ReplaceInventory or
// the locked accessors rather than reading those fields from another goroutine.
type Config struct {
	mu sync.RWMutex

	// ClaimGVRs is the list of claim GroupVersionResources to poll.
	ClaimGVRs []schema.GroupVersionResource

	// XRGVRs is the list of composite resource GroupVersionResources to poll.
	XRGVRs []schema.GroupVersionResource

	// XRGVRSScopes maps XR GVR keys to their Crossplane resource scope.
	XRGVRSScopes map[string]ResourceScope

	// MRGVRs is the list of provider Managed Resource GroupVersionResources to poll.
	MRGVRs []schema.GroupVersionResource

	// MRGVRSScopes maps MR GVR keys to their Kubernetes resource scope.
	MRGVRSScopes map[string]ResourceScope

	// MRProviderNames maps GVR key (group/version/resource) to provider package name.
	MRProviderNames map[string]string

	// EnvClaimGVRs are the CLAIM_GVRS values from the environment. Preserved
	// across re-discovery so additive merge semantics stay stable.
	EnvClaimGVRs []schema.GroupVersionResource

	// EnvXRGVRs are the XR_GVRS values from the environment. Preserved across
	// re-discovery so additive merge semantics stay stable.
	EnvXRGVRs []schema.GroupVersionResource

	// EnvMRGVRs are the MR_GVRS values from the environment. Preserved across
	// re-discovery so additive merge semantics stay stable.
	EnvMRGVRs []schema.GroupVersionResource

	// Namespaces restricts watches to these namespaces. Empty means all.
	Namespaces []string

	// CreatorAnnotationKey is the annotation key used to identify the claim creator.
	CreatorAnnotationKey string

	// TeamAnnotationKey is the annotation key used to identify the team.
	TeamAnnotationKey string

	// CompositionLabelKey is the label key on XRs identifying the Composition.
	CompositionLabelKey string

	// CompositeLabelKey is the label key on MRs linking them to a composite (XR).
	CompositeLabelKey string

	// MRXRParentLabels controls whether crossplane_mr_* series include typed
	// XR parent labels (xr_kind, xr_namespace, xr_group). Default true.
	MRXRParentLabels bool

	// XRParentLabels controls whether crossplane_xr_* series include
	// parent_kind / parent_name / parent_namespace from XR ownerReferences.
	// Default true.
	XRParentLabels bool

	// MROwnerXRLabels controls whether crossplane_mr_* series include
	// owner_xr_name / owner_xr_kind / owner_xr_namespace / owner_xr_group for
	// the immediate composing XR. Default true.
	MROwnerXRLabels bool

	// XRChildInfoMetric controls emission of crossplane_xr_child_info edge
	// series (XR→XR nesting). Default false (opt-in; watch cardinality).
	XRChildInfoMetric bool

	// MROwnerInfoMetric controls emission of crossplane_mr_owner_info edge
	// series (MR→XR). Default false; prefer owner_xr_* labels when MR count
	// is large (~1200+).
	MROwnerInfoMetric bool

	// ConditionTransitionTimestamps emits Ready lastTransitionTime gauges.
	// Default false.
	ConditionTransitionTimestamps bool

	// ConditionMessageInfo emits Ready condition info metrics (includes
	// truncated message). Default false; watch cardinality/PII.
	ConditionMessageInfo bool

	// ResourceIdentityLabels adds uid and composition_revision labels on
	// helper metrics when those helpers are enabled. Default false.
	ResourceIdentityLabels bool

	// PollIntervalSeconds is the number of seconds between polling cycles.
	PollIntervalSeconds int

	// DiscoveryIntervalSeconds is the number of seconds between XRD/MRD
	// re-discovery cycles. Zero disables periodic re-discovery (startup
	// discovery still runs once).
	DiscoveryIntervalSeconds int

	// MetricsAddr is the listen address for the HTTP metrics server.
	MetricsAddr string

	// LogLevel is the slog level for structured logging.
	// Valid values: debug, info, warn, error. Default: info.
	LogLevel slog.Level

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

// PollTargets is a point-in-time copy of the GVR lists used for one poll cycle.
type PollTargets struct {
	ClaimGVRs []schema.GroupVersionResource
	XRGVRs    []schema.GroupVersionResource
	MRGVRs    []schema.GroupVersionResource
}

// DiscoveredInventory is a complete GVR inventory ready to install under lock.
type DiscoveredInventory struct {
	ClaimGVRs       []schema.GroupVersionResource
	XRGVRs          []schema.GroupVersionResource
	MRGVRs          []schema.GroupVersionResource
	XRGVRSScopes    map[string]ResourceScope
	MRGVRSScopes    map[string]ResourceScope
	MRProviderNames map[string]string
}

const (
	defaultCompositionLabelKey = "crossplane.io/composition-name"
	defaultCompositeLabelKey   = "crossplane.io/composite"
	defaultPollInterval        = 30
	defaultDiscoveryInterval   = 300
	defaultMetricsAddr         = ":8080"
	defaultLogLevel            = "info"
	defaultStoreBackend        = "memory"
	defaultS3KeyPrefix         = "xp-tracker"
	defaultS3Region            = "us-east-1"
)

// Load reads configuration from environment variables and returns a validated Config.
func Load() (*Config, error) {
	cfg := &Config{
		CompositionLabelKey:           defaultCompositionLabelKey,
		CompositeLabelKey:             defaultCompositeLabelKey,
		MRXRParentLabels:              true,
		XRParentLabels:                true,
		MROwnerXRLabels:               true,
		XRChildInfoMetric:             false,
		MROwnerInfoMetric:             false,
		ConditionTransitionTimestamps: false,
		ConditionMessageInfo:          false,
		ResourceIdentityLabels:        false,
		PollIntervalSeconds:           defaultPollInterval,
		DiscoveryIntervalSeconds:      defaultDiscoveryInterval,
		MetricsAddr:                   defaultMetricsAddr,
		LogLevel:                      slog.LevelInfo,
		MRProviderNames:               make(map[string]string),
		XRGVRSScopes:                  make(map[string]ResourceScope),
		MRGVRSScopes:                  make(map[string]ResourceScope),
	}

	// Optional: CLAIM_GVRS (deprecated in favour of XRD discovery)
	claimRaw := os.Getenv("CLAIM_GVRS")
	if claimRaw != "" {
		claimGVRs, err := ParseGVRs(claimRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid CLAIM_GVRS: %w", err)
		}
		cfg.EnvClaimGVRs = claimGVRs
		cfg.ClaimGVRs = append([]schema.GroupVersionResource(nil), claimGVRs...)
	}

	// Optional: XR_GVRS (deprecated in favour of XRD discovery)
	xrRaw := os.Getenv("XR_GVRS")
	if xrRaw != "" {
		xrGVRs, err := ParseGVRs(xrRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid XR_GVRS: %w", err)
		}
		cfg.EnvXRGVRs = xrGVRs
		cfg.XRGVRs = append([]schema.GroupVersionResource(nil), xrGVRs...)
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

	// Optional: COMPOSITE_LABEL_KEY
	if v := os.Getenv("COMPOSITE_LABEL_KEY"); v != "" {
		cfg.CompositeLabelKey = v
	}

	// Optional: MR_XR_PARENT_LABELS (default true). Set false/0/no to omit
	// xr_kind / xr_namespace / xr_group on crossplane_mr_* series.
	if v := os.Getenv("MR_XR_PARENT_LABELS"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MR_XR_PARENT_LABELS: %w", err)
		}
		cfg.MRXRParentLabels = enabled
	}

	// Optional: XR_PARENT_LABELS (default true). Set false/0/no to omit
	// parent_kind / parent_name / parent_namespace on crossplane_xr_* series.
	if v := os.Getenv("XR_PARENT_LABELS"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid XR_PARENT_LABELS: %w", err)
		}
		cfg.XRParentLabels = enabled
	}

	// Optional: MR_OWNER_XR_LABELS (default true). Set false/0/no to omit
	// owner_xr_* on crossplane_mr_* series.
	if v := os.Getenv("MR_OWNER_XR_LABELS"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MR_OWNER_XR_LABELS: %w", err)
		}
		cfg.MROwnerXRLabels = enabled
	}

	// Optional: XR_CHILD_INFO_METRIC (default false). Set true/1/yes to emit
	// crossplane_xr_child_info XR→XR edge series.
	if v := os.Getenv("XR_CHILD_INFO_METRIC"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid XR_CHILD_INFO_METRIC: %w", err)
		}
		cfg.XRChildInfoMetric = enabled
	}

	// Optional: MR_OWNER_INFO_METRIC (default false). Set true/1/yes to emit
	// crossplane_mr_owner_info MR→XR edge series (discouraged at large MR counts).
	if v := os.Getenv("MR_OWNER_INFO_METRIC"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MR_OWNER_INFO_METRIC: %w", err)
		}
		cfg.MROwnerInfoMetric = enabled
	}

	// Optional: CONDITION_TRANSITION_TIMESTAMPS (default false).
	if v := os.Getenv("CONDITION_TRANSITION_TIMESTAMPS"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CONDITION_TRANSITION_TIMESTAMPS: %w", err)
		}
		cfg.ConditionTransitionTimestamps = enabled
	}

	// Optional: CONDITION_MESSAGE_INFO (default false).
	if v := os.Getenv("CONDITION_MESSAGE_INFO"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CONDITION_MESSAGE_INFO: %w", err)
		}
		cfg.ConditionMessageInfo = enabled
	}

	// Optional: RESOURCE_IDENTITY_LABELS (default false) — uid / composition_revision
	// on helper metrics when those helpers are enabled.
	if v := os.Getenv("RESOURCE_IDENTITY_LABELS"); v != "" {
		enabled, err := parseBoolEnv(v)
		if err != nil {
			return nil, fmt.Errorf("invalid RESOURCE_IDENTITY_LABELS: %w", err)
		}
		cfg.ResourceIdentityLabels = enabled
	}

	// Optional: MR_GVRS (merged with MRD discovery at startup and on refresh)
	if mrRaw := os.Getenv("MR_GVRS"); mrRaw != "" {
		mrGVRs, err := ParseGVRs(mrRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid MR_GVRS: %w", err)
		}
		cfg.EnvMRGVRs = mrGVRs
		cfg.MRGVRs = append([]schema.GroupVersionResource(nil), mrGVRs...)
	}

	// Optional: POLL_INTERVAL_SECONDS
	if v := os.Getenv("POLL_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("POLL_INTERVAL_SECONDS must be a positive integer, got %q", v)
		}
		cfg.PollIntervalSeconds = n
	}

	// Optional: DISCOVERY_INTERVAL_SECONDS (0 disables periodic re-discovery)
	if v := os.Getenv("DISCOVERY_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("DISCOVERY_INTERVAL_SECONDS must be a non-negative integer, got %q", v)
		}
		cfg.DiscoveryIntervalSeconds = n
	}

	// Optional: METRICS_ADDR
	if v := os.Getenv("METRICS_ADDR"); v != "" {
		cfg.MetricsAddr = v
	}

	// Optional: LOG_LEVEL (debug|info|warn|error)
	logLevelRaw := os.Getenv("LOG_LEVEL")
	if logLevelRaw == "" {
		logLevelRaw = defaultLogLevel
	}
	level, err := ParseLogLevel(logLevelRaw)
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = level

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

// SnapshotPollTargets returns a copy of the current claim/XR/MR GVR lists for
// one poll cycle. The copy is safe to iterate without holding mu.
func (c *Config) SnapshotPollTargets() PollTargets {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return PollTargets{
		ClaimGVRs: append([]schema.GroupVersionResource(nil), c.ClaimGVRs...),
		XRGVRs:    append([]schema.GroupVersionResource(nil), c.XRGVRs...),
		MRGVRs:    append([]schema.GroupVersionResource(nil), c.MRGVRs...),
	}
}

// ReplaceInventory atomically replaces the discovered GVR inventory.
func (c *Config) ReplaceInventory(inv DiscoveredInventory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ClaimGVRs = inv.ClaimGVRs
	c.XRGVRs = inv.XRGVRs
	c.MRGVRs = inv.MRGVRs
	c.XRGVRSScopes = inv.XRGVRSScopes
	c.MRGVRSScopes = inv.MRGVRSScopes
	c.MRProviderNames = inv.MRProviderNames
}

// IsNamespacedXR reports whether the XR GVR is namespace-scoped.
func (c *Config) IsNamespacedXR(gvr schema.GroupVersionResource) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.XRGVRSScopes[gvrKey(gvr)] == ResourceScopeNamespaced
}

// IsNamespacedMR reports whether the MR GVR is namespace-scoped.
func (c *Config) IsNamespacedMR(gvr schema.GroupVersionResource) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MRGVRSScopes[gvrKey(gvr)] == ResourceScopeNamespaced
}

// MRProvider returns the provider package name for an MR GVR key, if known.
func (c *Config) MRProvider(gvrKey string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MRProviderNames[gvrKey]
}

// XRSupportsClaims reports whether an XR GVR is expected to have claims
// (LegacyCluster scope, or unknown scope for backwards compatibility).
func (c *Config) XRSupportsClaims(gvr schema.GroupVersionResource) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	scope, known := c.XRGVRSScopes[gvrKey(gvr)]
	return !known || scope == ResourceScopeLegacyCluster
}

func gvrKey(gvr schema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}

// parseBoolEnv parses a boolean environment value.
// Accepted true: true, 1, yes. Accepted false: false, 0, no. Case-insensitive.
func parseBoolEnv(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("want true/false/1/0/yes/no, got %q", v)
	}
}

// ParseLogLevel maps a case-insensitive level name to slog.Level.
// Accepted values: debug, info, warn (or warning), error.
func ParseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error; got %q", raw)
	}
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
			slog.Warn("duplicate GVR ignored", "gvr", key)
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
