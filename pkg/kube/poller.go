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

// NamespaceConfigProvider supplies per-namespace GVR configurations.
// The ConfigMapWatcher implements this interface.
type NamespaceConfigProvider interface {
	NamespaceConfigs() []config.NamespaceConfig
}

// Poller periodically lists Crossplane claims and XRs from the Kubernetes API
// and updates the in-memory store.
type Poller struct {
	client    dynamic.Interface
	cfg       *config.Config
	store     store.Store
	nsConfigs NamespaceConfigProvider // optional; nil means no namespace config polling
}

// NewPoller creates a new Poller.
func NewPoller(client dynamic.Interface, cfg *config.Config, s store.Store) *Poller {
	return &Poller{
		client: client,
		cfg:    cfg,
		store:  s,
	}
}

// SetNamespaceConfigProvider sets the provider for per-namespace GVR configurations.
// This must be called before Run.
func (p *Poller) SetNamespaceConfigProvider(provider NamespaceConfigProvider) {
	p.nsConfigs = provider
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

	// Track centrally-polled GVRs to avoid duplicating work for namespace configs.
	centralClaimGVRs := make(map[string]struct{}, len(p.cfg.ClaimGVRs))
	centralXRGVRs := make(map[string]struct{}, len(p.cfg.XRGVRs))

	// Poll XRs first so composition data is available for claim enrichment.
	for _, gvr := range p.cfg.XRGVRs {
		centralXRGVRs[GVRString(gvr)] = struct{}{}
		if err := p.pollXRs(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	for _, gvr := range p.cfg.ClaimGVRs {
		centralClaimGVRs[GVRString(gvr)] = struct{}{}
		if err := p.pollClaims(ctx, gvr); err != nil {
			hadErrors = true
			metrics.PollErrors.WithLabelValues(GVRString(gvr)).Inc()
		}
	}

	// Poll per-namespace configs (if any).
	if p.nsConfigs != nil {
		nsConfigs := p.nsConfigs.NamespaceConfigs()
		metrics.NamespaceConfigs.Set(float64(len(nsConfigs)))

		if len(nsConfigs) > 0 {
			nsErrors := p.pollNamespaceConfigsList(ctx, nsConfigs, centralClaimGVRs, centralXRGVRs)
			if nsErrors {
				hadErrors = true
			}
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

// pollNamespaceConfigsList polls GVRs from the given per-namespace configs.
// Claims are scoped to the config's namespace; XRs are polled cluster-wide.
// GVRs already covered by the central config are skipped to avoid double-counting.
// Returns true if any errors occurred.
func (p *Poller) pollNamespaceConfigsList(ctx context.Context, nsConfigs []config.NamespaceConfig, centralClaimGVRs, centralXRGVRs map[string]struct{}) bool {
	slog.Debug("polling namespace configs", "count", len(nsConfigs))
	var hadErrors bool

	for _, nsCfg := range nsConfigs {
		keys := KeysFromNamespaceConfig(&nsCfg, p.cfg)

		// Poll namespace-scoped XRs first (cluster-wide) so composition data
		// is available for claim enrichment.
		for _, gvr := range nsCfg.XRGVRs {
			gvrStr := GVRString(gvr)
			if _, ok := centralXRGVRs[gvrStr]; ok {
				// Already polled centrally — skip to avoid replacing with partial data.
				slog.Debug("skipping namespace XR GVR (already polled centrally)",
					"namespace", nsCfg.Namespace,
					"gvr", gvrStr,
				)
				continue
			}

			if err := p.pollXRsWithKeys(ctx, gvr, "", keys); err != nil {
				hadErrors = true
				metrics.PollErrors.WithLabelValues(gvrStr).Inc()
				slog.Error("failed to poll namespace XRs",
					"gvr", gvrStr,
					"source_namespace", nsCfg.Namespace,
					"error", err,
				)
			}
		}

		// Poll namespace-scoped claims — restricted to the config's namespace.
		for _, gvr := range nsCfg.ClaimGVRs {
			gvrStr := GVRString(gvr)
			if _, ok := centralClaimGVRs[gvrStr]; ok {
				// Already polled centrally — skip.
				slog.Debug("skipping namespace claim GVR (already polled centrally)",
					"namespace", nsCfg.Namespace,
					"gvr", gvrStr,
				)
				continue
			}

			if err := p.pollClaimsScoped(ctx, gvr, nsCfg.Namespace, keys); err != nil {
				hadErrors = true
				metrics.PollErrors.WithLabelValues(gvrStr).Inc()
				slog.Error("failed to poll namespace claims",
					"gvr", gvrStr,
					"namespace", nsCfg.Namespace,
					"error", err,
				)
			}
		}
	}

	return hadErrors
}

// pollClaimsScoped lists claims for a GVR in a specific namespace using custom keys,
// and updates the store.
func (p *Poller) pollClaimsScoped(ctx context.Context, gvr schema.GroupVersionResource, namespace string, keys ConvertKeys) error {
	gvrStr := GVRString(gvr)
	ri := p.client.Resource(gvr).Namespace(namespace)

	var claims []store.ClaimInfo
	var continueToken string
	for {
		opts := metav1.ListOptions{
			Limit:    500,
			Continue: continueToken,
		}
		list, err := ri.List(ctx, opts)
		if err != nil {
			slog.Error("failed to list scoped claims", "gvr", gvrStr, "namespace", namespace, "error", err)
			return err
		}

		for _, item := range list.Items {
			claims = append(claims, UnstructuredToClaimWithKeys(item, gvr, keys))
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}

	p.store.ReplaceClaims(gvrStr, claims)
	slog.Debug("namespace claims updated", "gvr", gvrStr, "namespace", namespace, "count", len(claims))
	return nil
}

// pollXRsWithKeys lists XRs for a GVR using custom keys and updates the store.
// If namespace is empty, lists cluster-wide.
func (p *Poller) pollXRsWithKeys(ctx context.Context, gvr schema.GroupVersionResource, namespace string, keys ConvertKeys) error {
	gvrStr := GVRString(gvr)

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
			slog.Error("failed to list XRs with keys", "gvr", gvrStr, "error", err)
			return err
		}

		for _, item := range list.Items {
			xrs = append(xrs, UnstructuredToXRWithKeys(item, gvr, keys))
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}

	p.store.ReplaceXRs(gvrStr, xrs)
	slog.Debug("namespace XRs updated", "gvr", gvrStr, "count", len(xrs))
	return nil
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
