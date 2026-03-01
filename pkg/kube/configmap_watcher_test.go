package kube

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kanzifucius/xp-tracker/pkg/config"
)

func makeConfigMap(namespace, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				config.ConfigMapLabelKey: config.ConfigMapLabelValue,
			},
		},
		Data: data,
	}
}

func startWatcher(t *testing.T, objects []runtime.Object, fallback *config.Config) (*ConfigMapWatcher, chan struct{}) {
	t.Helper()
	client := fake.NewClientset(objects...)
	w := NewConfigMapWatcher(client, fallback)
	stopCh := make(chan struct{})
	go w.Run(stopCh)
	if !w.WaitForSync(stopCh) {
		t.Fatal("failed to sync ConfigMap watcher cache")
	}
	// Give the event handlers a moment to process.
	time.Sleep(100 * time.Millisecond)
	return w, stopCh
}

func TestConfigMapWatcher_DiscoversExistingConfigMaps(t *testing.T) {
	cm1 := makeConfigMap("team-a", "tracker", map[string]string{
		"CLAIM_GVRS": "g/v1/things",
	})
	cm2 := makeConfigMap("team-b", "tracker", map[string]string{
		"XR_GVRS": "g/v1/xthings",
	})

	fallback := &config.Config{
		CreatorAnnotationKey: "default/creator",
	}

	w, stopCh := startWatcher(t, []runtime.Object{cm1, cm2}, fallback)
	defer close(stopCh)

	configs := w.NamespaceConfigs()
	if len(configs) != 2 {
		t.Fatalf("expected 2 namespace configs, got %d", len(configs))
	}

	if w.ConfigCount() != 2 {
		t.Errorf("ConfigCount: expected 2, got %d", w.ConfigCount())
	}

	// Verify configs are in the map.
	byNS := make(map[string]config.NamespaceConfig)
	for _, c := range configs {
		byNS[c.Namespace] = c
	}

	teamA, ok := byNS["team-a"]
	if !ok {
		t.Fatal("missing team-a config")
	}
	if len(teamA.ClaimGVRs) != 1 {
		t.Errorf("team-a claim GVRs: expected 1, got %d", len(teamA.ClaimGVRs))
	}
	if teamA.CreatorAnnotationKey != "default/creator" {
		t.Errorf("team-a creator key should inherit fallback, got %q", teamA.CreatorAnnotationKey)
	}

	teamB, ok := byNS["team-b"]
	if !ok {
		t.Fatal("missing team-b config")
	}
	if len(teamB.XRGVRs) != 1 {
		t.Errorf("team-b XR GVRs: expected 1, got %d", len(teamB.XRGVRs))
	}
}

func TestConfigMapWatcher_IgnoresInvalidConfigMaps(t *testing.T) {
	// ConfigMap with no GVRs — should be skipped.
	cm := makeConfigMap("bad-ns", "bad-config", map[string]string{
		"SOME_OTHER_KEY": "value",
	})

	w, stopCh := startWatcher(t, []runtime.Object{cm}, nil)
	defer close(stopCh)

	if w.ConfigCount() != 0 {
		t.Errorf("expected 0 configs (invalid CM should be skipped), got %d", w.ConfigCount())
	}
}

func TestConfigMapWatcher_HandlesAddEvent(t *testing.T) {
	w, stopCh := startWatcher(t, nil, nil)
	defer close(stopCh)

	if w.ConfigCount() != 0 {
		t.Fatalf("expected 0 configs initially, got %d", w.ConfigCount())
	}

	// Simulate an add event via the handler directly.
	cm := makeConfigMap("team-c", "new-config", map[string]string{
		"CLAIM_GVRS": "g/v1/widgets",
	})
	w.onAdd(cm)

	if w.ConfigCount() != 1 {
		t.Errorf("expected 1 config after add, got %d", w.ConfigCount())
	}
	configs := w.NamespaceConfigs()
	if configs[0].Namespace != "team-c" {
		t.Errorf("expected namespace team-c, got %q", configs[0].Namespace)
	}
}

func TestConfigMapWatcher_HandlesUpdateEvent(t *testing.T) {
	cm := makeConfigMap("team-d", "updater", map[string]string{
		"CLAIM_GVRS": "g/v1/things",
	})

	w, stopCh := startWatcher(t, []runtime.Object{cm}, nil)
	defer close(stopCh)

	// Simulate update.
	updated := cm.DeepCopy()
	updated.Data["CLAIM_GVRS"] = "g/v1/things, g/v1/widgets"
	w.onUpdate(cm, updated)

	configs := w.NamespaceConfigs()
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if len(configs[0].ClaimGVRs) != 2 {
		t.Errorf("expected 2 claim GVRs after update, got %d", len(configs[0].ClaimGVRs))
	}
}

func TestConfigMapWatcher_HandlesDeleteEvent(t *testing.T) {
	cm := makeConfigMap("team-e", "deleter", map[string]string{
		"CLAIM_GVRS": "g/v1/things",
	})

	w, stopCh := startWatcher(t, []runtime.Object{cm}, nil)
	defer close(stopCh)

	if w.ConfigCount() != 1 {
		t.Fatalf("expected 1 config before delete, got %d", w.ConfigCount())
	}

	w.onDelete(cm)

	if w.ConfigCount() != 0 {
		t.Errorf("expected 0 configs after delete, got %d", w.ConfigCount())
	}
}

func TestConfigMapWatcher_UpdateToInvalidRemovesConfig(t *testing.T) {
	cm := makeConfigMap("team-f", "going-bad", map[string]string{
		"CLAIM_GVRS": "g/v1/things",
	})

	w, stopCh := startWatcher(t, []runtime.Object{cm}, nil)
	defer close(stopCh)

	if w.ConfigCount() != 1 {
		t.Fatalf("expected 1 config initially, got %d", w.ConfigCount())
	}

	// Update to invalid (no GVRs).
	bad := cm.DeepCopy()
	bad.Data = map[string]string{"UNRELATED": "value"}
	w.onUpdate(cm, bad)

	if w.ConfigCount() != 0 {
		t.Errorf("expected 0 configs after invalid update, got %d", w.ConfigCount())
	}
}

func TestConfigMapWatcher_AnnotationKeyInheritance(t *testing.T) {
	fallback := &config.Config{
		CreatorAnnotationKey: "central/creator",
		TeamAnnotationKey:    "central/team",
	}

	// cm1 overrides creator, inherits team.
	cm1 := makeConfigMap("ns1", "cm1", map[string]string{
		"CLAIM_GVRS":             "g/v1/things",
		"CREATOR_ANNOTATION_KEY": "custom/creator",
	})

	// cm2 inherits both.
	cm2 := makeConfigMap("ns2", "cm2", map[string]string{
		"XR_GVRS": "g/v1/xthings",
	})

	w, stopCh := startWatcher(t, []runtime.Object{cm1, cm2}, fallback)
	defer close(stopCh)

	byNS := make(map[string]config.NamespaceConfig)
	for _, c := range w.NamespaceConfigs() {
		byNS[c.Namespace] = c
	}

	ns1 := byNS["ns1"]
	if ns1.CreatorAnnotationKey != "custom/creator" {
		t.Errorf("ns1 creator should be overridden, got %q", ns1.CreatorAnnotationKey)
	}
	if ns1.TeamAnnotationKey != "central/team" {
		t.Errorf("ns1 team should inherit fallback, got %q", ns1.TeamAnnotationKey)
	}

	ns2 := byNS["ns2"]
	if ns2.CreatorAnnotationKey != "central/creator" {
		t.Errorf("ns2 creator should inherit fallback, got %q", ns2.CreatorAnnotationKey)
	}
	if ns2.TeamAnnotationKey != "central/team" {
		t.Errorf("ns2 team should inherit fallback, got %q", ns2.TeamAnnotationKey)
	}
}

func TestConfigMapWatcher_ConcurrentAccess(t *testing.T) {
	w, stopCh := startWatcher(t, nil, nil)
	defer close(stopCh)

	// Hammer the watcher from multiple goroutines.
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			cm := makeConfigMap("ns", "cm", map[string]string{
				"CLAIM_GVRS": "g/v1/things",
			})
			for j := 0; j < 100; j++ {
				w.onAdd(cm)
				_ = w.NamespaceConfigs()
				_ = w.ConfigCount()
				w.onDelete(cm)
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// If we get here without a race condition panic, the test passes.
}
