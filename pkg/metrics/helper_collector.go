package metrics

import (
	"log/slog"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

const conditionMessageMaxRunes = 128

var helperIdentityLabels = []string{"group", "kind", "version", "namespace", "name"}

// HelperCollector emits optional dashboard-helper metrics (Phase 4).
// All families default off via config flags.
type HelperCollector struct {
	store                 store.Store
	emitTransitions       bool
	emitConditionInfo     bool
	includeIdentityLabels bool
	xrReadyTransitionDesc *prometheus.Desc
	mrReadyTransitionDesc *prometheus.Desc
	xrConditionInfoDesc   *prometheus.Desc
	mrConditionInfoDesc   *prometheus.Desc
}

// NewHelperCollector creates a HelperCollector. Pass false for unused families.
func NewHelperCollector(s store.Store, emitTransitions, emitConditionInfo, includeIdentityLabels bool) *HelperCollector {
	c := &HelperCollector{
		store:                 s,
		emitTransitions:       emitTransitions,
		emitConditionInfo:     emitConditionInfo,
		includeIdentityLabels: includeIdentityLabels,
	}
	idLabels := helperIdentityLabels
	if includeIdentityLabels {
		idLabels = append(append([]string{}, helperIdentityLabels...), "uid", "composition_revision")
	}
	if emitTransitions {
		c.xrReadyTransitionDesc = prometheus.NewDesc(
			"crossplane_xr_ready_last_transition_timestamp_seconds",
			"Unix timestamp of the XR Ready condition lastTransitionTime. Opt-in via CONDITION_TRANSITION_TIMESTAMPS.",
			idLabels,
			nil,
		)
		c.mrReadyTransitionDesc = prometheus.NewDesc(
			"crossplane_mr_ready_last_transition_timestamp_seconds",
			"Unix timestamp of the MR Ready condition lastTransitionTime. Opt-in via CONDITION_TRANSITION_TIMESTAMPS.",
			idLabels,
			nil,
		)
	}
	if emitConditionInfo {
		infoLabels := append(append([]string{}, idLabels...), "type", "reason", "message")
		c.xrConditionInfoDesc = prometheus.NewDesc(
			"crossplane_xr_condition_info",
			"Info-style gauge (1) for the XR Ready condition including truncated message. Opt-in via CONDITION_MESSAGE_INFO; watch cardinality/PII.",
			infoLabels,
			nil,
		)
		c.mrConditionInfoDesc = prometheus.NewDesc(
			"crossplane_mr_condition_info",
			"Info-style gauge (1) for the MR Ready condition including truncated message. Opt-in via CONDITION_MESSAGE_INFO; watch cardinality/PII.",
			infoLabels,
			nil,
		)
	}
	return c
}

// Describe sends enabled helper descriptors.
func (c *HelperCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.xrReadyTransitionDesc != nil {
		ch <- c.xrReadyTransitionDesc
		ch <- c.mrReadyTransitionDesc
	}
	if c.xrConditionInfoDesc != nil {
		ch <- c.xrConditionInfoDesc
		ch <- c.mrConditionInfoDesc
	}
}

// Collect emits enabled helper gauges.
func (c *HelperCollector) Collect(ch chan<- prometheus.Metric) {
	if c.emitTransitions || c.emitConditionInfo {
		for _, xr := range c.store.SnapshotXRs() {
			c.emitXR(ch, xr)
		}
		for _, mr := range c.store.SnapshotMRs() {
			c.emitMR(ch, mr)
		}
	}
}

func (c *HelperCollector) identityValues(group, kind, version, namespace, name, uid, compositionRevision string) []string {
	labels := []string{group, kind, version, namespace, name}
	if c.includeIdentityLabels {
		labels = append(labels, uid, compositionRevision)
	}
	return labels
}

func (c *HelperCollector) emitXR(ch chan<- prometheus.Metric, xr store.XRInfo) {
	id := c.identityValues(xr.Group, xr.Kind, xr.Version, xr.Namespace, xr.Name, xr.UID, xr.CompositionRevision)
	if c.emitTransitions && !xr.ReadyLastTransition.IsZero() {
		m, err := prometheus.NewConstMetric(c.xrReadyTransitionDesc, prometheus.GaugeValue, float64(xr.ReadyLastTransition.Unix()), id...)
		if err != nil {
			slog.Error("failed to create xr ready transition metric", "error", err)
		} else {
			ch <- m
		}
	}
	if c.emitConditionInfo && (xr.Reason != "" || xr.ReadyMessage != "") {
		labels := append(append([]string{}, id...), "Ready", xr.Reason, truncateRunes(xr.ReadyMessage, conditionMessageMaxRunes))
		m, err := prometheus.NewConstMetric(c.xrConditionInfoDesc, prometheus.GaugeValue, 1, labels...)
		if err != nil {
			slog.Error("failed to create xr condition info metric", "error", err)
		} else {
			ch <- m
		}
	}
}

func (c *HelperCollector) emitMR(ch chan<- prometheus.Metric, mr store.MRInfo) {
	id := c.identityValues(mr.Group, mr.Kind, mr.Version, mr.Namespace, mr.Name, mr.UID, "")
	if c.emitTransitions && !mr.ReadyLastTransition.IsZero() {
		m, err := prometheus.NewConstMetric(c.mrReadyTransitionDesc, prometheus.GaugeValue, float64(mr.ReadyLastTransition.Unix()), id...)
		if err != nil {
			slog.Error("failed to create mr ready transition metric", "error", err)
		} else {
			ch <- m
		}
	}
	if c.emitConditionInfo && (mr.Reason != "" || mr.ReadyMessage != "") {
		labels := append(append([]string{}, id...), "Ready", mr.Reason, truncateRunes(mr.ReadyMessage, conditionMessageMaxRunes))
		m, err := prometheus.NewConstMetric(c.mrConditionInfoDesc, prometheus.GaugeValue, 1, labels...)
		if err != nil {
			slog.Error("failed to create mr condition info metric", "error", err)
		} else {
			ch <- m
		}
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
