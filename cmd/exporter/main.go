// Package main is the entrypoint for the Crossplane metrics exporter.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/kube"
	"github.com/kanzifucius/xp-tracker/pkg/metrics"
	"github.com/kanzifucius/xp-tracker/pkg/server"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// Build-time variables set via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// warnDeprecatedGVRsOnce logs the CLAIM_GVRS/XR_GVRS deprecation warning at most once.
var warnDeprecatedGVRsOnce sync.Once

func main() {
	// Resolve LOG_LEVEL before constructing the handler so early startup
	// messages honour the configured verbosity.
	logLevelRaw := os.Getenv("LOG_LEVEL")
	if logLevelRaw == "" {
		logLevelRaw = "info"
	}
	level, err := config.ParseLogLevel(logLevelRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid LOG_LEVEL: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	slog.Info("xp-tracker starting",
		"version", version,
		"commit", commit,
		"date", date,
		"log_level", logLevelRaw,
	)

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Set up context with signal-based cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create Kubernetes dynamic client.
	client, err := kube.NewDynamicClient()
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	if err := discoverAndApplyGVRs(ctx, client, cfg); err != nil {
		return err
	}

	targets := cfg.SnapshotPollTargets()
	slog.Info("configuration loaded",
		"claim_gvrs", formatGVRs(targets.ClaimGVRs),
		"xr_gvrs", formatGVRs(targets.XRGVRs),
		"mr_gvrs", formatGVRs(targets.MRGVRs),
		"namespaces", cfg.Namespaces,
		"creator_annotation", cfg.CreatorAnnotationKey,
		"team_annotation", cfg.TeamAnnotationKey,
		"composition_label", cfg.CompositionLabelKey,
		"composite_label", cfg.CompositeLabelKey,
		"poll_interval_seconds", cfg.PollIntervalSeconds,
		"discovery_interval_seconds", cfg.DiscoveryIntervalSeconds,
		"metrics_addr", cfg.MetricsAddr,
		"log_level", cfg.LogLevel.String(),
		"store_backend", cfg.StoreBackend,
	)

	// Initialise the store based on STORE_BACKEND.
	mem := store.New()
	var s store.Store = mem

	if cfg.StoreBackend == "s3" {
		s3Client, err := store.NewS3Client(ctx, cfg.S3Region, cfg.S3Endpoint)
		if err != nil {
			return fmt.Errorf("create S3 client: %w", err)
		}
		s3s := store.NewS3Store(mem, s3Client, cfg.S3Bucket, cfg.S3KeyPrefix)

		slog.Info("restoring store snapshot from S3",
			"bucket", cfg.S3Bucket,
			"key_prefix", cfg.S3KeyPrefix,
		)
		if err := s3s.Restore(ctx); err != nil {
			slog.Warn("failed to restore S3 snapshot, starting with empty store", "error", err)
		}
		s = s3s
	}

	// Start the HTTP metrics server.
	mrXRParent := cfg.MRXRParentLabels
	xrParent := cfg.XRParentLabels
	mrOwnerXR := cfg.MROwnerXRLabels
	xrChildInfo := cfg.XRChildInfoMetric
	mrOwnerInfo := cfg.MROwnerInfoMetric
	srv := server.New(cfg.MetricsAddr, s, server.Options{
		MRXRParentLabels:  &mrXRParent,
		XRParentLabels:    &xrParent,
		MROwnerXRLabels:   &mrOwnerXR,
		XRChildInfoMetric: &xrChildInfo,
		MROwnerInfoMetric: &mrOwnerInfo,
	})
	go func() {
		if err := srv.Run(ctx); err != nil {
			slog.Error("metrics server error", "error", err)
			cancel()
		}
	}()

	// Start the polling loop.
	poller := kube.NewPoller(client, cfg, s)
	go func() {
		slog.Info("starting poller")
		poller.Run(ctx)
	}()

	// Periodic XRD/MRD re-discovery so new types appear without a restart.
	if cfg.DiscoveryIntervalSeconds > 0 {
		go runDiscoveryRefresh(ctx, client, cfg)
	} else {
		slog.Info("periodic GVR re-discovery disabled", "discovery_interval_seconds", 0)
	}

	// Mark the server as ready only after the first poll cycle completes
	// (even if some GVR polls errored). This avoids the previous fixed sleep
	// that could race ahead of a slow first cycle.
	go func() {
		select {
		case <-poller.FirstPollDone():
			srv.SetReady()
			slog.Info("server marked as ready")
		case <-ctx.Done():
		}
	}()

	slog.Info("exporter running", "metrics_addr", cfg.MetricsAddr)

	// Block until context is cancelled.
	<-ctx.Done()
	slog.Info("shutdown complete")
	return nil
}

// formatGVRs converts a slice of GVRs to human-readable strings for logging.
func formatGVRs(gvrs []schema.GroupVersionResource) []string {
	out := make([]string, len(gvrs))
	for i, gvr := range gvrs {
		out[i] = gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
	}
	return out
}

// runDiscoveryRefresh re-runs XRD/MRD discovery on a fixed interval and swaps
// the poller's GVR inventory under the config lock. Failures leave the previous
// inventory in place.
func runDiscoveryRefresh(ctx context.Context, client dynamic.Interface, cfg *config.Config) {
	interval := time.Duration(cfg.DiscoveryIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("periodic GVR re-discovery enabled", "discovery_interval_seconds", cfg.DiscoveryIntervalSeconds)

	for {
		select {
		case <-ctx.Done():
			slog.Info("discovery refresh shutting down")
			return
		case <-ticker.C:
			if err := discoverAndApplyGVRs(ctx, client, cfg); err != nil {
				slog.Warn("periodic GVR re-discovery failed; keeping previous inventory", "error", err)
				continue
			}
			targets := cfg.SnapshotPollTargets()
			slog.Info("periodic GVR re-discovery complete",
				"claim_gvrs", formatGVRs(targets.ClaimGVRs),
				"xr_gvrs", formatGVRs(targets.XRGVRs),
				"mr_gvrs", formatGVRs(targets.MRGVRs),
			)
		}
	}
}

func discoverAndApplyGVRs(ctx context.Context, client dynamic.Interface, cfg *config.Config) error {
	start := time.Now()
	err := applyDiscoveredGVRs(ctx, client, cfg)
	metrics.DiscoveryDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.DiscoveryRuns.WithLabelValues(metrics.DiscoveryResultError).Inc()
		return err
	}
	metrics.DiscoveryRuns.WithLabelValues(metrics.DiscoveryResultSuccess).Inc()
	return nil
}

func applyDiscoveredGVRs(ctx context.Context, client dynamic.Interface, cfg *config.Config) error {
	claimGVRs, xrGVRs, xrScopes, err := kube.DiscoverFromXRD(ctx, client)
	if err != nil {
		return fmt.Errorf("discover claim and XR GVRs from XRDs: %w", err)
	}
	if len(xrGVRs) == 0 {
		return fmt.Errorf("no XR GVRs discovered from XRDs; ensure Crossplane XRDs exist in the cluster")
	}

	if len(cfg.EnvClaimGVRs) > 0 || len(cfg.EnvXRGVRs) > 0 {
		warnDeprecatedGVRsOnce.Do(func() {
			slog.Warn("CLAIM_GVRS and/or XR_GVRS are deprecated; values are merged with XRD discovery (deduplicated)",
				"claim_gvrs", formatGVRs(cfg.EnvClaimGVRs),
				"xr_gvrs", formatGVRs(cfg.EnvXRGVRs),
			)
		})
	}

	mrGVRs, providerNames, mrScopes, err := kube.DiscoverMRGVRsFromMRDs(ctx, client)
	if err != nil {
		return fmt.Errorf("discover MR GVRs from managed resource definitions: %w", err)
	}
	if providerNames == nil {
		providerNames = make(map[string]string)
	}
	if mrScopes == nil {
		mrScopes = make(map[string]config.ResourceScope)
	}
	if xrScopes == nil {
		xrScopes = make(map[string]config.ResourceScope)
	}

	// Merge env overrides with freshly discovered sets (additive, deduplicated).
	// Env slices are immutable after Load so re-discovery does not accumulate
	// previously discovered GVRs that have since been removed from the cluster.
	cfg.ReplaceInventory(config.DiscoveredInventory{
		ClaimGVRs:       mergeGVRs(claimGVRs, cfg.EnvClaimGVRs),
		XRGVRs:          mergeGVRs(xrGVRs, cfg.EnvXRGVRs),
		MRGVRs:          mergeGVRs(mrGVRs, cfg.EnvMRGVRs),
		XRGVRSScopes:    xrScopes,
		MRGVRSScopes:    mrScopes,
		MRProviderNames: providerNames,
	})

	return nil
}

// mergeGVRs returns discovered GVRs plus any extras not already present
// (deduplicated by group/version/resource).
func mergeGVRs(discovered, extra []schema.GroupVersionResource) []schema.GroupVersionResource {
	seen := make(map[string]struct{}, len(discovered)+len(extra))
	merged := make([]schema.GroupVersionResource, 0, len(discovered)+len(extra))
	for _, gvr := range discovered {
		key := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, gvr)
	}
	for _, gvr := range extra {
		key := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, gvr)
	}
	return merged
}
