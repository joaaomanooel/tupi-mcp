package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joaaomanooel/tupi-mcp/internal/config"
	"github.com/joaaomanooel/tupi-mcp/internal/mcpserver"
	"github.com/joaaomanooel/tupi-mcp/internal/octoprint"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 2 * time.Minute
	maxHeaderBytes    = 32 << 10
	shutdownTimeout   = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("tupi-mcp stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	settings, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	if missing := mcpserver.PhysicalToolsWithoutLimits(settings.EnabledTools, settings.PhysicalLimits); len(missing) > 0 {
		logger.Warn("physical tools are not published because their deployment limits are missing or invalid", "tools", missing)
	}
	client, err := octoprint.NewClient(
		settings.OctoPrintBaseURL,
		settings.OctoPrintAPIKey,
		&http.Client{Timeout: settings.OctoPrintTimeout},
		settings.OctoPrintMaxBodyBytes,
		settings.OctoPrintAllowInsecureHTTP,
	)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", settings.ListenAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	server, err := mcpserver.NewServer(client, settings.EnabledTools, settings.PhysicalLimits)
	if err != nil {
		return err
	}
	handler, err := mcpserver.NewHandler(mcpserver.Options{
		Server:              server,
		AllowedHosts:        settings.AllowedHosts,
		AllowedOrigins:      settings.AllowedOrigins,
		MaxRequestBodyBytes: settings.MaxRequestBodyBytes,
	})
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              settings.ListenAddress,
		Handler:           handler,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	sessionsClosed := make(chan struct{})
	var closeSessionsOnce sync.Once
	httpServer.RegisterOnShutdown(func() {
		closeSessionsOnce.Do(func() {
			for session := range server.Sessions() {
				_ = session.Close()
			}
			close(sessionsClosed)
		})
	})

	serverError := make(chan error, 1)
	go func() {
		serverError <- httpServer.Serve(listener)
	}()
	logger.Info("MCP server listening", "address", settings.ListenAddress, "path", "/mcp")

	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down MCP server")
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		_ = httpServer.Close()
		return err
	}
	select {
	case <-sessionsClosed:
	case <-shutdownContext.Done():
		return shutdownContext.Err()
	}
	for session := range server.Sessions() {
		_ = session.Close()
	}
	if err := <-serverError; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
