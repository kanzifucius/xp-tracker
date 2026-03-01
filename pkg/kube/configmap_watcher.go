package kube

import (
	"fmt"
	"log/slog"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/kanzifucius/xp-tracker/pkg/config"
)

// ConfigMapWatcher uses a Kubernetes informer to discover and track per-namespace
// ConfigMaps labeled with xp-tracker.kanzi.io/config=gvrs. It maintains a
// thread-safe registry of parsed NamespaceConfig entries that the poller reads
// each cycle.
type ConfigMapWatcher struct {
	mu       sync.RWMutex
	configs  map[string]*config.NamespaceConfig // keyed by "namespace/name"
	fallback *config.Config
	factory  informers.SharedInformerFactory
}

// NewConfigMapWatcher creates a ConfigMapWatcher that watches ConfigMaps across
// all namespaces with the label xp-tracker.kanzi.io/config=gvrs.
func NewConfigMapWatcher(client kubernetes.Interface, fallback *config.Config) *ConfigMapWatcher {
	labelSelector := fmt.Sprintf("%s=%s", config.ConfigMapLabelKey, config.ConfigMapLabelValue)

	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		0, // no resync — informer will receive events in real time
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = labelSelector
		}),
	)

	w := &ConfigMapWatcher{
		configs:  make(map[string]*config.NamespaceConfig),
		fallback: fallback,
		factory:  factory,
	}

	cmInformer := factory.Core().V1().ConfigMaps().Informer()
	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})

	return w
}

// Run starts the informer and blocks until the stop channel is closed.
// The informer syncs its cache before returning control, so NamespaceConfigs()
// will return accurate data after Run begins.
func (w *ConfigMapWatcher) Run(stopCh <-chan struct{}) {
	slog.Info("starting ConfigMap watcher",
		"label", config.ConfigMapLabelKey+"="+config.ConfigMapLabelValue,
	)
	w.factory.Start(stopCh)
	w.factory.WaitForCacheSync(stopCh)
	slog.Info("ConfigMap watcher cache synced")
}

// WaitForSync blocks until the informer cache has completed its initial list.
// This should be called after Run to ensure the watcher has loaded all existing
// ConfigMaps before the first poll cycle.
func (w *ConfigMapWatcher) WaitForSync(stopCh <-chan struct{}) bool {
	return cache.WaitForCacheSync(stopCh,
		w.factory.Core().V1().ConfigMaps().Informer().HasSynced,
	)
}

// NamespaceConfigs returns a snapshot of all currently valid NamespaceConfig entries.
func (w *ConfigMapWatcher) NamespaceConfigs() []config.NamespaceConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()

	out := make([]config.NamespaceConfig, 0, len(w.configs))
	for _, nsCfg := range w.configs {
		out = append(out, *nsCfg)
	}
	return out
}

// ConfigCount returns the number of currently tracked namespace configs.
func (w *ConfigMapWatcher) ConfigCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.configs)
}

func (w *ConfigMapWatcher) onAdd(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	w.upsert(cm)
}

func (w *ConfigMapWatcher) onUpdate(_, newObj interface{}) {
	cm, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	w.upsert(cm)
}

func (w *ConfigMapWatcher) onDelete(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		// Handle deleted final state unknown (tombstone).
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			slog.Warn("ConfigMap watcher: unexpected delete event type", "type", fmt.Sprintf("%T", obj))
			return
		}
		cm, ok = tombstone.Obj.(*corev1.ConfigMap)
		if !ok {
			slog.Warn("ConfigMap watcher: unexpected tombstone object type", "type", fmt.Sprintf("%T", tombstone.Obj))
			return
		}
	}

	key := cm.Namespace + "/" + cm.Name
	w.mu.Lock()
	delete(w.configs, key)
	w.mu.Unlock()

	slog.Info("namespace config removed",
		"namespace", cm.Namespace,
		"configmap", cm.Name,
	)
}

func (w *ConfigMapWatcher) upsert(cm *corev1.ConfigMap) {
	key := cm.Namespace + "/" + cm.Name

	nsCfg, err := config.ParseNamespaceConfigMap(cm, w.fallback)
	if err != nil {
		slog.Error("failed to parse namespace ConfigMap, skipping",
			"namespace", cm.Namespace,
			"configmap", cm.Name,
			"error", err,
		)
		// Remove any previously valid config for this key.
		w.mu.Lock()
		delete(w.configs, key)
		w.mu.Unlock()
		return
	}

	w.mu.Lock()
	w.configs[key] = nsCfg
	w.mu.Unlock()

	slog.Info("namespace config updated",
		"namespace", nsCfg.Namespace,
		"configmap", nsCfg.ConfigMapName,
		"claim_gvrs", len(nsCfg.ClaimGVRs),
		"xr_gvrs", len(nsCfg.XRGVRs),
	)
}
