package metrics

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// Package inventory metrics intentionally use a small label set (kind, name,
// healthy, installed). Package revisions are never emitted.
var (
	packageLabels = []string{"kind", "name", "healthy", "installed"}

	packageTotalDesc = prometheus.NewDesc(
		"crossplane_packages_total",
		"Number of Crossplane package meta-types (Provider, Configuration, Function) by kind, name, and Healthy/Installed status.",
		packageLabels,
		nil,
	)

	packageHealthyDesc = prometheus.NewDesc(
		"crossplane_packages_healthy",
		"Number of Healthy Crossplane packages by kind, name, and Healthy/Installed status.",
		packageLabels,
		nil,
	)

	packageInstalledDesc = prometheus.NewDesc(
		"crossplane_packages_installed",
		"Number of Installed Crossplane packages by kind, name, and Healthy/Installed status.",
		packageLabels,
		nil,
	)
)

// packageAggKey is the label tuple used to aggregate package metrics.
type packageAggKey struct {
	Kind      string
	Name      string
	Healthy   string
	Installed string
}

// packageAggVal holds aggregated counts for a package label tuple.
type packageAggVal struct {
	Total     int
	Healthy   int
	Installed int
}

// PackageCollector implements prometheus.Collector for Crossplane packages.
type PackageCollector struct {
	store store.Store
}

// NewPackageCollector creates a new PackageCollector.
func NewPackageCollector(s store.Store) *PackageCollector {
	return &PackageCollector{store: s}
}

// Describe sends the metric descriptors to the channel.
func (c *PackageCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- packageTotalDesc
	ch <- packageHealthyDesc
	ch <- packageInstalledDesc
}

// Collect snapshots the store, aggregates by label tuple, and emits gauges.
func (c *PackageCollector) Collect(ch chan<- prometheus.Metric) {
	packages := c.store.SnapshotPackages()

	agg := make(map[packageAggKey]*packageAggVal)
	for _, pkg := range packages {
		key := packageAggKey{
			Kind:      pkg.Kind,
			Name:      pkg.Name,
			Healthy:   boolToLabel(pkg.Healthy),
			Installed: boolToLabel(pkg.Installed),
		}
		v, ok := agg[key]
		if !ok {
			v = &packageAggVal{}
			agg[key] = v
		}
		v.Total++
		if pkg.Healthy {
			v.Healthy++
		}
		if pkg.Installed {
			v.Installed++
		}
	}

	for key, val := range agg {
		labels := []string{key.Kind, key.Name, key.Healthy, key.Installed}

		m, err := prometheus.NewConstMetric(
			packageTotalDesc,
			prometheus.GaugeValue,
			float64(val.Total),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create packages_total metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			packageHealthyDesc,
			prometheus.GaugeValue,
			float64(val.Healthy),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create packages_healthy metric", "error", err)
			continue
		}
		ch <- m

		m, err = prometheus.NewConstMetric(
			packageInstalledDesc,
			prometheus.GaugeValue,
			float64(val.Installed),
			labels...,
		)
		if err != nil {
			slog.Error("failed to create packages_installed metric", "error", err)
			continue
		}
		ch <- m
	}
}
