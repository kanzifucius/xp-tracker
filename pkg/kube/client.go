// Package kube provides Kubernetes client setup and resource polling for the exporter.
package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewDynamicClient creates a dynamic Kubernetes client.
// It tries in-cluster config first, then falls back to the default kubeconfig.
func NewDynamicClient() (dynamic.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		applyClientLimits(cfg)
		return dynamic.NewForConfig(cfg)
	}

	// Fall back to kubeconfig for local development.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, hErr := os.UserHomeDir()
		if hErr != nil {
			return nil, fmt.Errorf("cannot determine kubeconfig path: %w", hErr)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	applyClientLimits(cfg)
	return dynamic.NewForConfig(cfg)
}

// applyClientLimits raises the default client-side rate limits.
//
// The Kubernetes client defaults (5 QPS / 10 burst) are too low when the
// poller lists hundreds of MR GVRs concurrently. 100 QPS / 200 burst gives
// the concurrent worker pool (mrPollConcurrency workers) enough headroom
// without approaching EKS API server limits.
func applyClientLimits(cfg *rest.Config) {
	cfg.QPS = 100
	cfg.Burst = 200
}
