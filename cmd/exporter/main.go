// Package main is the entrypoint for the Crossplane metrics exporter.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

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

	slog.Info("configuration loaded",
		"claim_gvrs", formatGVRs(cfg.ClaimGVRs),
		"xr_gvrs", formatGVRs(cfg.XRGVRs),
		"namespaces", cfg.Namespaces,
		"creator_annotation", cfg.CreatorAnnotationKey,
		"team_annotation", cfg.TeamAnnotationKey,
		"composition_label", cfg.CompositionLabelKey,
		"poll_interval_seconds", cfg.PollIntervalSeconds,
		"metrics_addr", cfg.MetricsAddr,
		"store_backend", cfg.StoreBackend,
	)

	// Create Kubernetes dynamic client.
	client, err := kube.NewDynamicClient()
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}

	// Initialise the store based on STORE_BACKEND.
	mem := store.New()
	var s store.Store = mem

	// Set up context with signal-based cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

	// Mark the server as ready after the first poll cycle completes.
	// The poller runs an initial poll synchronously before entering the
	// ticker loop, so by the time Run returns control we can signal readiness.
	// We use a small goroutine to avoid blocking if the first poll is slow.
	go func() {
		// Wait briefly for the first poll to finish (Run does an immediate poll).
		time.Sleep(100 * time.Millisecond)
		srv.SetReady()
		slog.Info("server marked as ready")
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
