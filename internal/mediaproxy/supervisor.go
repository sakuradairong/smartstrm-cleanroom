package mediaproxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
)

// Supervisor owns the media proxy listener so its configuration can change
// while the process keeps running. The listening address is fixed for the
// lifetime of the process because container port publishing is decided at
// startup; everything else is swapped in place.
type Supervisor struct {
	mu       sync.Mutex
	listen   string
	server   *http.Server
	listener net.Listener
	done     chan struct{}

	// handler is read by in-flight requests without holding mu, so a reload
	// never blocks on a long-running proxied response.
	handler atomic.Pointer[http.Handler]
}

func NewSupervisor() *Supervisor { return &Supervisor{} }

// Listen reports the address currently bound, or "" when the proxy is stopped.
func (s *Supervisor) Listen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listen
}

// Running reports whether the proxy is currently serving.
func (s *Supervisor) Running() bool { return s.Listen() != "" }

func (s *Supervisor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := s.handler.Load()
	if handler == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"media proxy is disabled"}`))
		return
	}
	(*handler).ServeHTTP(w, r)
}

// Apply reconciles the running proxy with cfg. It returns true when the change
// could not be applied live and needs a process restart, which only happens
// when the listening address of an already-running proxy changes.
func (s *Supervisor) Apply(cfg config.MediaProxyConfig, publicURL, signingSecret string) (bool, error) {
	if !cfg.Enabled {
		s.Stop()
		return false, nil
	}
	handler, err := New(cfg, publicURL, signingSecret)
	if err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Publish the handler before starting the listener so the first accepted
	// request already sees the new configuration.
	s.handler.Store(&handler)
	if s.listener != nil {
		// Already serving: a different address cannot be rebound because the
		// container only publishes the original port.
		return s.listen != cfg.Listen, nil
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		s.handler.Store(nil)
		return false, fmt.Errorf("listen on media proxy address %q: %w", cfg.Listen, err)
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	done := make(chan struct{})
	s.listen, s.listener, s.server, s.done = cfg.Listen, listener, server, done
	go func() {
		defer close(done)
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("media proxy stopped", "error", serveErr)
		}
	}()
	return false, nil
}

// Stop shuts the proxy down and releases its port. It is safe to call when the
// proxy is not running.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	server, done := s.server, s.done
	s.server, s.listener, s.done, s.listen = nil, nil, nil, ""
	s.handler.Store(nil)
	s.mu.Unlock()
	if server == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	<-done
}
