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

	return dynamic.NewForConfig(cfg)
}
