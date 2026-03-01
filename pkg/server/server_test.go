package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// startTestServer launches a Server on ":0" (OS-assigned port) and returns
// the base URL and a cancel function. The caller must call cancel to stop the server.
func startTestServer(t *testing.T, s store.Store) (baseURL string, cancel context.CancelFunc) {
	t.Helper()

	srv := New(":0", s)
	srv.SetReady()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := srv.Run(ctx); err != nil {
			// Errors during shutdown are expected; only log unexpected ones.
			select {
			case <-ctx.Done():
			default:
				t.Logf("server error: %v", err)
			}
		}
	}()

	// Addr() blocks until the listener is bound.
	addr := srv.Addr()
	return "http://" + addr, cancel
}

// httpGet is a test helper that performs a GET request with context.
func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- test hits local test server only
	if err != nil {
		t.Fatalf("failed to GET %s: %v", url, err)
	}
	return resp
}

func TestServer_MetricsEndpoint_Integration(t *testing.T) {
	s := store.New()

	s.ReplaceClaims("platform.example.org/v1alpha1/postgresqlinstances", []store.ClaimInfo{
		{GVR: "platform.example.org/v1alpha1/postgresqlinstances", Group: "platform.example.org", Kind: "PostgreSQLInstance", Namespace: "team-a", Name: "db-1", Creator: "alice", Team: "backend", Composition: "prod-pg", Ready: true},
		{GVR: "platform.example.org/v1alpha1/postgresqlinstances", Group: "platform.example.org", Kind: "PostgreSQLInstance", Namespace: "team-a", Name: "db-2", Creator: "bob", Team: "backend", Composition: "prod-pg", Ready: false},
		{GVR: "platform.example.org/v1alpha1/postgresqlinstances", Group: "platform.example.org", Kind: "PostgreSQLInstance", Namespace: "team-b", Name: "db-3", Creator: "carol", Team: "frontend", Composition: "dev-pg", Ready: true},
	})
	s.ReplaceXRs("platform.example.org/v1alpha1/xpostgresqlinstances", []store.XRInfo{
		{GVR: "platform.example.org/v1alpha1/xpostgresqlinstances", Group: "platform.example.org", Kind: "XPostgreSQLInstance", Name: "xr-1", Composition: "prod-pg", Ready: true},
		{GVR: "platform.example.org/v1alpha1/xpostgresqlinstances", Group: "platform.example.org", Kind: "XPostgreSQLInstance", Name: "xr-2", Composition: "dev-pg", Ready: false},
	})

	baseURL, cancel := startTestServer(t, s)
	defer cancel()

	resp := httpGet(t, baseURL+"/metrics")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	text := string(body)

	// Verify claim metrics are present.
	expectedMetrics := []string{
		"crossplane_claims_total",
		"crossplane_claims_ready",
		"crossplane_xr_total",
		"crossplane_xr_ready",
	}
	for _, name := range expectedMetrics {
		if !strings.Contains(text, name) {
			t.Errorf("missing metric %q in output", name)
		}
	}

	// Verify some label values appear.
	expectedLabels := []string{
		`group="platform.example.org"`,
		`kind="PostgreSQLInstance"`,
		`namespace="team-a"`,
		`composition="prod-pg"`,
		`creator="alice"`,
		`team="backend"`,
		`kind="XPostgreSQLInstance"`,
	}
	for _, label := range expectedLabels {
		if !strings.Contains(text, label) {
			t.Errorf("missing label %q in output", label)
		}
	}

	// Verify HELP lines.
	if !strings.Contains(text, "# HELP crossplane_claims_total") {
		t.Error("missing HELP for crossplane_claims_total")
	}
	if !strings.Contains(text, "# HELP crossplane_xr_total") {
		t.Error("missing HELP for crossplane_xr_total")
	}

	// Verify TYPE lines.
	if !strings.Contains(text, "# TYPE crossplane_claims_total gauge") {
		t.Error("missing TYPE gauge for crossplane_claims_total")
	}
	if !strings.Contains(text, "# TYPE crossplane_xr_total gauge") {
		t.Error("missing TYPE gauge for crossplane_xr_total")
	}
}

func TestServer_HealthzEndpoint(t *testing.T) {
	s := store.New()
	baseURL, cancel := startTestServer(t, s)
	defer cancel()

	resp := httpGet(t, baseURL+"/healthz")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("expected 'ok', got %q", string(body))
	}
}

func TestServer_ReadyzEndpoint(t *testing.T) {
	s := store.New()
	srv := New(":0", s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Run(ctx)
	}()

	// Addr() blocks until the listener is bound.
	baseURL := "http://" + srv.Addr()

	// Before SetReady, should return 503.
	resp := httpGet(t, baseURL+"/readyz")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before SetReady, got %d", resp.StatusCode)
	}

	// After SetReady, should return 200.
	srv.SetReady()
	resp = httpGet(t, baseURL+"/readyz")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after SetReady, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("expected 'ok', got %q", string(body))
	}
}
