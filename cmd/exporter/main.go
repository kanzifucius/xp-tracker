// Package main is the entrypoint for the Crossplane metrics exporter.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/kube"
	"github.com/kanzifucius/xp-tracker/pkg/server"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// Build-time variables set via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("xp-tracker starting",
		"version", version,
		"commit", commit,
		"date", date,
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

	slog.Info("configuration loaded",
		"claim_gvrs", formatGVRs(cfg.ClaimGVRs),
		"xr_gvrs", formatGVRs(cfg.XRGVRs),
		"mr_gvrs", formatGVRs(cfg.MRGVRs),
		"namespaces", cfg.Namespaces,
		"creator_annotation", cfg.CreatorAnnotationKey,
		"team_annotation", cfg.TeamAnnotationKey,
		"composition_label", cfg.CompositionLabelKey,
		"composite_label", cfg.CompositeLabelKey,
		"poll_interval_seconds", cfg.PollIntervalSeconds,
		"metrics_addr", cfg.MetricsAddr,
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
	srv := server.New(cfg.MetricsAddr, s)
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

func discoverAndApplyGVRs(ctx context.Context, client dynamic.Interface, cfg *config.Config) error {
	claimGVRs, xrGVRs, xrScopes, err := kube.DiscoverFromXRD(ctx, client)
	if err != nil {
		return fmt.Errorf("discover claim and XR GVRs from XRDs: %w", err)
	}
	if len(xrGVRs) == 0 {
		return fmt.Errorf("no XR GVRs discovered from XRDs; ensure Crossplane XRDs exist in the cluster")
	}

	// Preserve env-configured CLAIM_GVRS / XR_GVRS before applying discovery.
	// These are deprecated additive overrides merged with XRD discovery.
	envClaimGVRs := cfg.ClaimGVRs
	envXRGVRs := cfg.XRGVRs
	if len(envClaimGVRs) > 0 || len(envXRGVRs) > 0 {
		slog.Warn("CLAIM_GVRS and/or XR_GVRS are deprecated; values are merged with XRD discovery (deduplicated)",
			"claim_gvrs", formatGVRs(envClaimGVRs),
			"xr_gvrs", formatGVRs(envXRGVRs),
		)
	}

	cfg.ClaimGVRs = mergeGVRs(claimGVRs, envClaimGVRs)
	cfg.XRGVRs = mergeGVRs(xrGVRs, envXRGVRs)
	cfg.XRGVRSScopes = xrScopes

	mrGVRs, providerNames, mrScopes, err := kube.DiscoverMRGVRsFromMRDs(ctx, client)
	if err != nil {
		return fmt.Errorf("discover MR GVRs from managed resource definitions: %w", err)
	}

	if cfg.MRProviderNames == nil {
		cfg.MRProviderNames = make(map[string]string)
	}
	if cfg.MRGVRSScopes == nil {
		cfg.MRGVRSScopes = make(map[string]config.ResourceScope)
	}
	for key, name := range providerNames {
		cfg.MRProviderNames[key] = name
	}
	for key, scope := range mrScopes {
		cfg.MRGVRSScopes[key] = scope
	}

	// Merge env-configured MR_GVRS (additive, deduplicated).
	cfg.MRGVRs = mergeGVRs(mrGVRs, cfg.MRGVRs)

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
