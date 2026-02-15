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

// New creates a new metrics Server.
// It registers claim and XR collectors with a dedicated Prometheus registry.
func New(addr string, s store.Store) *Server {
	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.NewClaimCollector(s))
	registry.MustRegister(metrics.NewXRCollector(s))
	metrics.RegisterSelfMetrics(registry)

	srv := &Server{
		registry:  registry,
		listening: make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: false, // stick to classic Prometheus text format
	}))
	mux.HandleFunc("GET /bookkeeping", bookkeepingHandler(s))
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

// SetReady marks the server as ready. Call this after the first successful
// polling cycle completes.
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
