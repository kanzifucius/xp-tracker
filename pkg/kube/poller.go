package kube

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/metrics"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// mrPollConcurrency is the maximum number of concurrent MR GVR list calls per
// poll cycle. 1055 GVRs polled sequentially takes minutes; a bounded worker
// pool reduces that to seconds.
//
// This must be kept in proportion to the client QPS/Burst configured in
// NewDynamicClient. At 20 workers and 100 QPS / 200 burst the initial wave
// is absorbed by the burst allowance and steady-state throughput stays well
// within the QPS ceiling.
const mrPollConcurrency = 20

// packageGVRs are the fixed, low-cardinality pkg.crossplane.io meta-types
// polled each cycle. Package revisions (ProviderRevision, etc.) are excluded
// on purpose. Function uses v1beta1 to match Crossplane 2.0.x samples; if a
// cluster only serves functions/v1 the NotFound path skips this GVR cleanly.
var packageGVRs = []schema.GroupVersionResource{
	{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"},
	{Group: "pkg.crossplane.io", Version: "v1", Resource: "configurations"},
	{Group: "pkg.crossplane.io", Version: "v1beta1", Resource: "functions"},
}

// Poller periodically lists Crossplane claims, XRs, managed resources, and
// package meta-types from the Kubernetes API and updates the in-memory store.
type Poller struct {
	client dynamic.Interface
	cfg    *config.Config
	store  store.Store

	firstPollDone chan struct{}
	firstPollOnce sync.Once
}

// NewPoller creates a new Poller.
func NewPoller(client dynamic.Interface, cfg *config.Config, s store.Store) *Poller {
	return &Poller{
		client:        client,
		cfg:           cfg,
		store:         s,
		firstPollDone: make(chan struct{}),
	}
}

// FirstPollDone returns a channel that is closed after the first poll cycle
// completes. Completion is signalled even when some GVR polls errored.
func (p *Poller) FirstPollDone() <-chan struct{} {
	return p.firstPollDone
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(p.cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Run an initial poll immediately.
	p.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("poller shutting down")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// poll executes a single polling cycle: list all configured GVRs and update the store.
func (p *Poller) poll(ctx context.Context) {
	defer p.firstPollOnce.Do(func() { close(p.firstPollDone) })

	slog.Debug("polling cycle started")
	start := time.Now()

	// Snapshot GVR lists so re-discovery can replace inventory mid-cycle
	// without races or holding the config lock across API calls.
	targets := p.cfg.SnapshotPollTargets()

	var hadErrors bool

	// Poll XRs first so composition and claim linkage are available for enrichment.
	for _, gvr := range targets.XRGVRs {
		if err := p.pollXRs(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	for _, gvr := range targets.ClaimGVRs {
		if err := p.pollClaims(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	// MR polling is fan-out: there can be 1000+ GVRs (one per provider resource
	// type). Sequential listing would take minutes; a bounded worker pool keeps
	// it to seconds without flooding the API server.
	if mrErrors := p.pollMRsConcurrent(ctx, targets.MRGVRs); mrErrors {
		hadErrors = true
	}

	// Package inventory is a small fixed set (Providers, Configurations,
	// Functions). Revisions are never listed.
	for _, gvr := range packageGVRs {
		if err := p.pollPackages(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	// Enrich legacy claims from XRs, legacy XRs from claims, and MRs from XRs.
	p.store.EnrichClaimCompositions()
	p.store.EnrichXRClaims()
	p.store.EnrichMRClaims()

	// Only persist if the entire cycle succeeded. Persisting a partial
	// snapshot could overwrite a valid one with incomplete data.
	if hadErrors {
		slog.Warn("skipping persistence due to polling errors")
	} else if ps, ok := p.store.(store.PersistentStore); ok {
		persistStart := time.Now()
		if err := ps.Persist(ctx); err != nil {
			slog.Error("failed to persist store snapshot", "error", err)
		} else {
			metrics.S3PersistDuration.Observe(time.Since(persistStart).Seconds())
		}
	}

	// Update self-monitoring gauges.
	claimCount := p.store.ClaimCount()
	xrCount := p.store.XRCount()
	mrCount := p.store.MRCount()
	pkgCount := p.store.PackageCount()
	metrics.StoreClaims.Set(float64(claimCount))
	metrics.StoreXRs.Set(float64(xrCount))
	metrics.StoreMRs.Set(float64(mrCount))
	metrics.StorePackages.Set(float64(pkgCount))
	metrics.PollDuration.Observe(time.Since(start).Seconds())

	// Freshness signal: only advance when every GVR list succeeded. Partial
	// cycles leave the previous timestamp so alerts fire on sustained failures.
	if !hadErrors {
		metrics.LastSuccessfulPollTimestamp.Set(float64(time.Now().Unix()))
	}

	slog.Info("polling cycle complete",
		"claims", claimCount,
		"xrs", xrCount,
		"mrs", mrCount,
		"packages", pkgCount,
		"had_errors", hadErrors,
	)
}

// pollClaims lists all claims for a given GVR and updates the store.
func (p *Poller) pollClaims(ctx context.Context, gvr schema.GroupVersionResource) error {
	gvrStr := GVRString(gvr)
	namespaces := p.cfg.Namespaces

	var allClaims []store.ClaimInfo

	if len(namespaces) == 0 {
		// List across all namespaces.
		claims, err := p.listClaims(ctx, gvr, "")
		if err != nil {
			slog.Error("failed to list claims", "gvr", gvrStr, "error", err)
			return err
		}
		allClaims = claims
	} else {
		var errs []error
		for _, ns := range namespaces {
			claims, err := p.listClaims(ctx, gvr, ns)
			if err != nil {
				slog.Error("failed to list claims", "gvr", gvrStr, "namespace", ns, "error", err)
				errs = append(errs, err)
				continue
			}
			allClaims = append(allClaims, claims...)
		}
		if len(errs) > 0 {
			// Still update the store with whatever we got, but report errors.
			p.store.ReplaceClaims(gvrStr, allClaims)
			slog.Debug("claims partially updated", "gvr", gvrStr, "count", len(allClaims))
			return errs[0]
		}
	}

	p.store.ReplaceClaims(gvrStr, allClaims)
	slog.Debug("claims updated", "gvr", gvrStr, "count", len(allClaims))
	return nil
}

// pollXRs lists all XRs for a given GVR and updates the store.
func (p *Poller) pollXRs(ctx context.Context, gvr schema.GroupVersionResource) error {
	gvrStr := GVRString(gvr)

	namespaces := p.cfg.Namespaces
	var allXRs []store.XRInfo

	if !p.cfg.IsNamespacedXR(gvr) || len(namespaces) == 0 {
		xrs, err := p.listXRs(ctx, gvr, "")
		if err != nil {
			slog.Error("failed to list XRs", "gvr", gvrStr, "error", err)
			return err
		}
		allXRs = xrs
	} else {
		var errs []error
		for _, ns := range namespaces {
			xrs, err := p.listXRs(ctx, gvr, ns)
			if err != nil {
				slog.Error("failed to list XRs", "gvr", gvrStr, "namespace", ns, "error", err)
				errs = append(errs, err)
				continue
			}
			allXRs = append(allXRs, xrs...)
		}
		if len(errs) > 0 {
			p.store.ReplaceXRs(gvrStr, allXRs)
			slog.Debug("XRs partially updated", "gvr", gvrStr, "count", len(allXRs))
			return errs[0]
		}
	}

	p.store.ReplaceXRs(gvrStr, allXRs)
	slog.Debug("XRs updated", "gvr", gvrStr, "count", len(allXRs))
	return nil
}

// pollMRsConcurrent fans out MR polling across the given GVRs using a
// bounded worker pool. It returns true if any GVR produced an error.
func (p *Poller) pollMRsConcurrent(ctx context.Context, mrGVRs []schema.GroupVersionResource) (hadErrors bool) {
	if len(mrGVRs) == 0 {
		return false
	}

	var errCount atomic.Int64

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(mrPollConcurrency)

	for _, gvr := range mrGVRs {
		gvr := gvr // capture loop variable (pre-Go 1.22)
		g.Go(func() error {
			if err := p.pollMRs(ctx, gvr); err != nil {
				errCount.Add(1)
				metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
			}
			// Always return nil so errgroup does not cancel the context on the
			// first error — we want all GVRs attempted every cycle.
			return nil
		})
	}

	// g.Wait() cannot error (we never return non-nil above).
	_ = g.Wait()
	return errCount.Load() > 0
}

// pollMRs lists claim-linked MRs for a given GVR and updates the store.
func (p *Poller) pollMRs(ctx context.Context, gvr schema.GroupVersionResource) error {
	gvrStr := GVRString(gvr)
	provider := p.cfg.MRProvider(gvrStr)

	namespaces := p.cfg.Namespaces
	var allMRs []store.MRInfo

	if !p.cfg.IsNamespacedMR(gvr) || len(namespaces) == 0 {
		mrs, err := p.listMRs(ctx, gvr, "", provider)
		if err != nil {
			slog.Error("failed to list MRs", "gvr", gvrStr, "error", err)
			return err
		}
		allMRs = mrs
	} else {
		var errs []error
		for _, ns := range namespaces {
			mrs, err := p.listMRs(ctx, gvr, ns, provider)
			if err != nil {
				slog.Error("failed to list MRs", "gvr", gvrStr, "namespace", ns, "error", err)
				errs = append(errs, err)
				continue
			}
			allMRs = append(allMRs, mrs...)
		}
		if len(errs) > 0 {
			p.store.ReplaceMRs(gvrStr, allMRs)
			slog.Debug("MRs partially updated", "gvr", gvrStr, "count", len(allMRs))
			return errs[0]
		}
	}

	p.store.ReplaceMRs(gvrStr, allMRs)
	slog.Debug("MRs updated", "gvr", gvrStr, "count", len(allMRs))
	return nil
}

// listMRs lists XR-linked MRs for a specific GVR and optional namespace.
func (p *Poller) listMRs(ctx context.Context, gvr schema.GroupVersionResource, namespace, provider string) ([]store.MRInfo, error) {
	var ri dynamic.ResourceInterface
	if namespace == "" {
		ri = p.client.Resource(gvr)
	} else {
		ri = p.client.Resource(gvr).Namespace(namespace)
	}

	labelSelector := p.cfg.CompositeLabelKey

	var mrs []store.MRInfo
	var continueToken string
	for {
		opts := metav1.ListOptions{
			Limit:         500,
			Continue:      continueToken,
			LabelSelector: labelSelector,
		}
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for _, item := range list.Items {
			mr := UnstructuredToMR(item, gvr, p.cfg, provider)
			if mr.XRName == "" {
				continue
			}
			mrs = append(mrs, mr)
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	return mrs, nil
}

// listClaims lists claims for a specific GVR and optional namespace.
// If namespace is empty, lists across all namespaces.
// Uses server-side pagination to avoid unbounded response sizes.
func (p *Poller) listClaims(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]store.ClaimInfo, error) {
	var ri dynamic.ResourceInterface
	if namespace == "" {
		ri = p.client.Resource(gvr)
	} else {
		ri = p.client.Resource(gvr).Namespace(namespace)
	}

	var claims []store.ClaimInfo
	var continueToken string
	for {
		opts := metav1.ListOptions{
			Limit:    500,
			Continue: continueToken,
		}
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for _, item := range list.Items {
			claims = append(claims, UnstructuredToClaim(item, gvr, p.cfg))
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	return claims, nil
}

// pollPackages lists Providers, Configurations, or Functions for a GVR and
// updates the store. A missing API (NotFound) is treated as an empty inventory
// so clusters without that package kind/version do not fail the poll cycle.
func (p *Poller) pollPackages(ctx context.Context, gvr schema.GroupVersionResource) error {
	gvrStr := GVRString(gvr)
	pkgs, err := p.listPackages(ctx, gvr)
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			p.store.ReplacePackages(gvrStr, nil)
			slog.Debug("package GVR not found; treating as empty", "gvr", gvrStr)
			return nil
		}
		slog.Error("failed to list packages", "gvr", gvrStr, "error", err)
		return err
	}
	p.store.ReplacePackages(gvrStr, pkgs)
	slog.Debug("packages updated", "gvr", gvrStr, "count", len(pkgs))
	return nil
}

// listPackages lists cluster-scoped package meta-types for a GVR.
func (p *Poller) listPackages(ctx context.Context, gvr schema.GroupVersionResource) ([]store.PackageInfo, error) {
	ri := p.client.Resource(gvr)

	var pkgs []store.PackageInfo
	var continueToken string
	for {
		opts := metav1.ListOptions{
			Limit:    500,
			Continue: continueToken,
		}
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for _, item := range list.Items {
			pkgs = append(pkgs, UnstructuredToPackage(item, gvr))
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	return pkgs, nil
}

// listXRs lists XRs for a specific GVR and optional namespace.
// Uses server-side pagination to avoid unbounded response sizes.
func (p *Poller) listXRs(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]store.XRInfo, error) {
	var ri dynamic.ResourceInterface
	if namespace == "" {
		ri = p.client.Resource(gvr)
	} else {
		ri = p.client.Resource(gvr).Namespace(namespace)
	}

	var xrs []store.XRInfo
	var continueToken string
	for {
		opts := metav1.ListOptions{
			Limit:    500,
			Continue: continueToken,
		}
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for _, item := range list.Items {
			xrs = append(xrs, UnstructuredToXR(item, gvr, p.cfg))
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	return xrs, nil
}
