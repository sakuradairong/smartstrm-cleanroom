package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/app"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/mediaproxy"
)

func main() {
	configPath := flag.String("config", envOr("SMARTSTRM_CONFIG", "config.json"), "configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	application, err := app.New(cfg)
	if err != nil {
		slog.Error("initialize application", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := application.Start(ctx); err != nil {
		slog.Error("start background services", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	servers := []*http.Server{server}
	if cfg.MediaProxy.Enabled {
		proxyHandler, err := mediaproxy.New(cfg.MediaProxy, cfg.PublicURL, cfg.WebhookToken)
		if err != nil {
			slog.Error("initialize media proxy", "error", err)
			os.Exit(1)
		}
		proxyServer := mediaproxy.Server(cfg.MediaProxy, proxyHandler)
		servers = append(servers, proxyServer)
	}
	listeners := make([]net.Listener, 0, len(servers))
	for _, runningServer := range servers {
		listener, err := net.Listen("tcp", runningServer.Addr)
		if err != nil {
			for _, opened := range listeners {
				_ = opened.Close()
			}
			slog.Error("listen", "address", runningServer.Addr, "error", err)
			os.Exit(1)
		}
		listeners = append(listeners, listener)
	}
	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var shutdowns sync.WaitGroup
		for _, runningServer := range servers {
			shutdowns.Add(1)
			go func(current *http.Server) {
				defer shutdowns.Done()
				_ = current.Shutdown(shutdownCtx)
			}(runningServer)
		}
		shutdowns.Wait()
		close(shutdownDone)
	}()

	serverErrors := make(chan error, len(servers))
	for index, runningServer := range servers {
		go func(current *http.Server, listener net.Listener) {
			serverErrors <- current.Serve(listener)
		}(runningServer, listeners[index])
	}
	slog.Info("SmartStrm clean-room server started", "listen", cfg.Listen, "media_proxy", cfg.MediaProxy.Enabled)
	select {
	case <-ctx.Done():
		select {
		case <-shutdownDone:
		case <-time.After(11 * time.Second):
			slog.Error("http server shutdown timed out")
		}
		return
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped", "error", err)
			stop()
			os.Exit(1)
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
