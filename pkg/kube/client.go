// Package kube provides Kubernetes client setup and resource polling for the exporter.
package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// restConfig resolves a *rest.Config, trying in-cluster first, then kubeconfig.
func restConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
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

	return cfg, nil
}

// NewDynamicClient creates a dynamic Kubernetes client.
// It tries in-cluster config first, then falls back to the default kubeconfig.
func NewDynamicClient() (dynamic.Interface, error) {
	cfg, err := restConfig()
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

// NewTypedClient creates a typed Kubernetes client (kubernetes.Interface).
// It tries in-cluster config first, then falls back to the default kubeconfig.
func NewTypedClient() (kubernetes.Interface, error) {
	cfg, err := restConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
