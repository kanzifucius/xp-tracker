package kube

import (
	"context"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/kanzifucius/xp-tracker/pkg/config"
	"github.com/kanzifucius/xp-tracker/pkg/metrics"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

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

	// Enrich claims with composition data from XRs.
	p.store.EnrichClaimCompositions()

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
	metrics.StoreClaims.Set(float64(claimCount))
	metrics.StoreXRs.Set(float64(xrCount))
	metrics.PollDuration.Observe(time.Since(start).Seconds())

	slog.Info("polling cycle complete",
		"claims", claimCount,
		"xrs", xrCount,
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
