// Package server provides the HTTP metrics server for the exporter.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/kanzifucius/xp-tracker/pkg/metrics"
	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// Server serves Prometheus metrics over HTTP.
type Server struct {
	httpServer *http.Server
	registry   *prometheus.Registry
	listener   net.Listener
	ready      atomic.Bool
	listening  chan struct{} // closed once the listener is bound
}

// Options configures optional server behaviour.
type Options struct {
	// MRXRParentLabels includes xr_kind / xr_namespace / xr_group on MR series.
	// When omitted (zero value), defaults to true.
	MRXRParentLabels *bool
	// XRParentLabels includes parent_kind / parent_name / parent_namespace on XR series.
	// When omitted (zero value), defaults to true.
	XRParentLabels *bool
	// MROwnerXRLabels includes owner_xr_name / owner_xr_kind / owner_xr_namespace /
	// owner_xr_group on MR series. When omitted, defaults to true.
	MROwnerXRLabels *bool
}

// New creates a new metrics Server.
// It registers claim, XR, MR, and package collectors with a dedicated Prometheus registry.
// Parent-related labels default to enabled; pass Options to override.
func New(addr string, s store.Store, opts ...Options) *Server {
	includeMRXRParent := true
	includeXRParent := true
	includeMROwnerXR := true
	if len(opts) > 0 {
		if opts[0].MRXRParentLabels != nil {
			includeMRXRParent = *opts[0].MRXRParentLabels
		}
		if opts[0].XRParentLabels != nil {
			includeXRParent = *opts[0].XRParentLabels
		}
		if opts[0].MROwnerXRLabels != nil {
			includeMROwnerXR = *opts[0].MROwnerXRLabels
		}
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.NewClaimCollector(s))
	registry.MustRegister(metrics.NewXRCollector(s, includeXRParent))
	registry.MustRegister(metrics.NewMRCollector(s, includeMRXRParent, includeMROwnerXR))
	registry.MustRegister(metrics.NewPackageCollector(s))
	metrics.RegisterSelfMetrics(registry)

	srv := &Server{
		registry:  registry,
		listening: make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: false, // stick to classic Prometheus text format
	}))
	mux.HandleFunc("GET /healthz", srv.healthzHandler)
	mux.HandleFunc("GET /readyz", srv.readyzHandler)

	srv.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}
	return srv
}

// Addr returns the listener address. It blocks until Run has opened the
// listener (or the listening channel is closed). Useful for tests using ":0".
func (s *Server) Addr() string {
	<-s.listening
	return s.listener.Addr().String()
}

// SetReady marks the server as ready. Call this after the first polling cycle
// completes (including cycles that had partial GVR errors).
func (s *Server) SetReady() { s.ready.Store(true) }

// healthzHandler responds with 200 OK if the process is alive.
func (s *Server) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// readyzHandler responds with 200 OK only after SetReady has been called,
// indicating the first poll cycle has completed and metrics are populated.
func (s *Server) readyzHandler(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready\n"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// Run starts the HTTP server. It blocks until the server is stopped.
// When ctx is cancelled, the server shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.httpServer.Addr)
	if err != nil {
		close(s.listening)
		return err
	}
	s.listener = ln
	close(s.listening)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("metrics server listening", "addr", ln.Addr().String())
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down metrics server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
