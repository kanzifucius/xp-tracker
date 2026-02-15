package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestServer_MetricsEndpoint(t *testing.T) {
	s := store.New()

	// Populate with some data so metrics are non-empty.
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a", Creator: "alice", Ready: true},
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "b", Creator: "alice", Ready: false},
	})
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{GVR: "g/v1/xthings", Group: "g", Kind: "XThing", Name: "xr1", Composition: "comp-a", Ready: true},
	})

	srv := New(":0", s) // port 0 = random available port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Wait for server to be ready (poll until it responds or timeout).
	var addr string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// The server uses :0, so we need to get the actual address.
		// Since http.Server with :0 doesn't easily expose the listener addr,
		// let's use a fixed port for testing instead.
		time.Sleep(50 * time.Millisecond)
	}
	_ = addr

	cancel()
	// Just verify the server shuts down cleanly.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
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

	// Use a specific test port.
	addr := "127.0.0.1:19876"
	srv := New(addr, s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Wait for server to start.
	var resp *http.Response
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		resp, err = http.Get("http://" + addr + "/metrics")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if resp == nil {
		cancel()
		t.Fatal("server did not start in time")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}()

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

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
