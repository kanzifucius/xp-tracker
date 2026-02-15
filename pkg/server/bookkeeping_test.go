package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

func TestBookkeeping_Empty(t *testing.T) {
	s := store.New()
	handler := bookkeepingHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/bookkeeping", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}

	var resp BookkeepingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Claims) != 0 {
		t.Errorf("expected 0 claims, got %d", len(resp.Claims))
	}
	if len(resp.XRs) != 0 {
		t.Errorf("expected 0 XRs, got %d", len(resp.XRs))
	}
	if resp.GeneratedAt == "" {
		t.Error("expected non-empty generatedAt")
	}

	// Verify generatedAt is valid RFC3339.
	if _, err := time.Parse(time.RFC3339, resp.GeneratedAt); err != nil {
		t.Errorf("generatedAt is not valid RFC3339: %v", err)
	}
}

func TestBookkeeping_WithData(t *testing.T) {
	s := store.New()

	createdAt := time.Now().Add(-1 * time.Hour)
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{
			GVR: "g/v1/things", Group: "g", Kind: "Thing",
			Namespace: "ns1", Name: "claim-a",
			Creator: "alice", Team: "backend", Composition: "comp-a",
			Ready: true, Reason: "Available", CreatedAt: createdAt,
		},
		{
			GVR: "g/v1/things", Group: "g", Kind: "Thing",
			Namespace: "ns1", Name: "claim-b",
			Creator: "bob", Team: "frontend", Composition: "comp-b",
			Ready: false, Reason: "Pending", CreatedAt: createdAt,
		},
	})
	s.ReplaceXRs("g/v1/xthings", []store.XRInfo{
		{
			GVR: "g/v1/xthings", Group: "g", Kind: "XThing",
			Name: "xr-1", Composition: "comp-a",
			Ready: true, Reason: "Available", CreatedAt: createdAt,
		},
	})

	handler := bookkeepingHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/bookkeeping", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp BookkeepingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Claims) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(resp.Claims))
	}
	if len(resp.XRs) != 1 {
		t.Fatalf("expected 1 XR, got %d", len(resp.XRs))
	}

	// Find claim-a in the response.
	var claimA *ClaimDTO
	for i := range resp.Claims {
		if resp.Claims[i].Name == "claim-a" {
			claimA = &resp.Claims[i]
			break
		}
	}
	if claimA == nil {
		t.Fatal("claim-a not found in response")
	}

	if claimA.Group != "g" {
		t.Errorf("claim group: got %q, want %q", claimA.Group, "g")
	}
	if claimA.Kind != "Thing" {
		t.Errorf("claim kind: got %q, want %q", claimA.Kind, "Thing")
	}
	if claimA.Namespace != "ns1" {
		t.Errorf("claim namespace: got %q, want %q", claimA.Namespace, "ns1")
	}
	if claimA.Creator != "alice" {
		t.Errorf("claim creator: got %q, want %q", claimA.Creator, "alice")
	}
	if claimA.Team != "backend" {
		t.Errorf("claim team: got %q, want %q", claimA.Team, "backend")
	}
	if claimA.Composition != "comp-a" {
		t.Errorf("claim composition: got %q, want %q", claimA.Composition, "comp-a")
	}
	if !claimA.Ready {
		t.Error("claim ready: expected true")
	}
	if claimA.Reason != "Available" {
		t.Errorf("claim reason: got %q, want %q", claimA.Reason, "Available")
	}

	// AgeSeconds should be approximately 3600 (1 hour).
	if claimA.AgeSeconds < 3500 || claimA.AgeSeconds > 3700 {
		t.Errorf("claim ageSeconds: got %d, expected ~3600", claimA.AgeSeconds)
	}

	// Verify XR fields.
	xr := resp.XRs[0]
	if xr.Name != "xr-1" {
		t.Errorf("xr name: got %q, want %q", xr.Name, "xr-1")
	}
	if xr.Composition != "comp-a" {
		t.Errorf("xr composition: got %q, want %q", xr.Composition, "comp-a")
	}
	if !xr.Ready {
		t.Error("xr ready: expected true")
	}
	if xr.AgeSeconds < 3500 || xr.AgeSeconds > 3700 {
		t.Errorf("xr ageSeconds: got %d, expected ~3600", xr.AgeSeconds)
	}
}

func TestBookkeeping_GeneratedAtIsUTC(t *testing.T) {
	s := store.New()
	handler := bookkeepingHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/bookkeeping", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp BookkeepingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	parsed, err := time.Parse(time.RFC3339, resp.GeneratedAt)
	if err != nil {
		t.Fatalf("generatedAt is not valid RFC3339: %v", err)
	}

	if parsed.Location() != time.UTC {
		t.Errorf("generatedAt should be UTC, got %v", parsed.Location())
	}
}

func TestBookkeeping_ZeroCreatedAt(t *testing.T) {
	s := store.New()
	s.ReplaceClaims("g/v1/things", []store.ClaimInfo{
		{GVR: "g/v1/things", Group: "g", Kind: "Thing", Namespace: "ns1", Name: "a"},
	})

	handler := bookkeepingHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/bookkeeping", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp BookkeepingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(resp.Claims))
	}

	// With zero CreatedAt, ageSeconds will be very large; just verify it doesn't panic.
	if resp.Claims[0].AgeSeconds < 0 {
		t.Error("ageSeconds should not be negative")
	}
}

func TestBookkeeping_Integration(t *testing.T) {
	s := store.New()

	createdAt := time.Now().Add(-30 * time.Minute)
	s.ReplaceClaims("platform.example.org/v1alpha1/postgresqlinstances", []store.ClaimInfo{
		{
			GVR:   "platform.example.org/v1alpha1/postgresqlinstances",
			Group: "platform.example.org", Kind: "PostgreSQLInstance",
			Namespace: "team-a", Name: "db-1",
			Creator: "alice", Team: "backend", Composition: "prod-pg",
			Ready: true, Reason: "Ready", CreatedAt: createdAt,
		},
	})
	s.ReplaceXRs("platform.example.org/v1alpha1/xpostgresqlinstances", []store.XRInfo{
		{
			GVR:   "platform.example.org/v1alpha1/xpostgresqlinstances",
			Group: "platform.example.org", Kind: "XPostgreSQLInstance",
			Name: "xr-db-1", Composition: "prod-pg",
			Ready: true, Reason: "Ready", CreatedAt: createdAt,
		},
	})

	// Start actual server.
	addr := "127.0.0.1:19877"
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
		resp, err = http.Get("http://" + addr + "/bookkeeping")
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

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}

	var bkResp BookkeepingResponse
	if err := json.NewDecoder(resp.Body).Decode(&bkResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(bkResp.Claims) != 1 {
		t.Errorf("expected 1 claim, got %d", len(bkResp.Claims))
	}
	if len(bkResp.XRs) != 1 {
		t.Errorf("expected 1 XR, got %d", len(bkResp.XRs))
	}

	if bkResp.Claims[0].Name != "db-1" {
		t.Errorf("claim name: got %q, want %q", bkResp.Claims[0].Name, "db-1")
	}
	if bkResp.XRs[0].Name != "xr-db-1" {
		t.Errorf("xr name: got %q, want %q", bkResp.XRs[0].Name, "xr-db-1")
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
