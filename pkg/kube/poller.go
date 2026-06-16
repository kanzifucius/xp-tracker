package kube

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
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

// Poller periodically lists Crossplane claims and XRs from the Kubernetes API
// and updates the in-memory store.
type Poller struct {
	client dynamic.Interface
	cfg    *config.Config
	store  store.Store
}

// NewPoller creates a new Poller.
func NewPoller(client dynamic.Interface, cfg *config.Config, s store.Store) *Poller {
	return &Poller{
		client: client,
		cfg:    cfg,
		store:  s,
	}
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
	slog.Debug("polling cycle started")
	start := time.Now()

	var hadErrors bool

	// Poll XRs first so composition data is available for claim enrichment.
	for _, gvr := range p.cfg.XRGVRs {
		if err := p.pollXRs(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	for _, gvr := range p.cfg.ClaimGVRs {
		if err := p.pollClaims(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	// MR polling is fan-out: there can be 1000+ GVRs (one per provider resource
	// type). Sequential listing would take minutes; a bounded worker pool keeps
	// it to seconds without flooding the API server.
	if mrErrors := p.pollMRsConcurrent(ctx); mrErrors {
		hadErrors = true
	}

	// Enrich claims with composition data from XRs, XRs with claim data from claims,
	// and MRs with claim data from XRs.
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
	metrics.StoreClaims.Set(float64(claimCount))
	metrics.StoreXRs.Set(float64(xrCount))
	metrics.StoreMRs.Set(float64(mrCount))
	metrics.PollDuration.Observe(time.Since(start).Seconds())

	slog.Info("polling cycle complete",
		"claims", claimCount,
		"xrs", xrCount,
		"mrs", mrCount,
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

	// XRs are typically cluster-scoped, but respect namespace config if set.
	namespaces := p.cfg.Namespaces
	var allXRs []store.XRInfo

	if len(namespaces) == 0 {
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

// pollMRsConcurrent fans out MR polling across all configured GVRs using a
// bounded worker pool. It returns true if any GVR produced an error.
func (p *Poller) pollMRsConcurrent(ctx context.Context) (hadErrors bool) {
	if len(p.cfg.MRGVRs) == 0 {
		return false
	}

	var errCount atomic.Int64

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(mrPollConcurrency)

	for _, gvr := range p.cfg.MRGVRs {
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
	provider := p.cfg.MRProviderNames[gvrStr]

	namespaces := p.cfg.Namespaces
	var allMRs []store.MRInfo

	if len(namespaces) == 0 {
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

// listMRs lists MRs for a specific GVR and optional namespace.
// Only resources with the composite label (claim chain) are returned.
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
