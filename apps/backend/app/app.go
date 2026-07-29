package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"alga/api"
	"alga/config"
	"alga/correlator"
	"alga/db"
	"alga/logger"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
	"alga/worker"
)

type App struct {
	cfg              *config.Config
	dbCli            *db.Client
	valkeyClient     *valkey.Client
	rabbitClient     *rabbitmq.Client
	sseBroker        *sse.Broker
	scheduler        *worker.InvestigationScheduler
	workerSet        *worker.WorkerSet
	correlator       *correlator.Correlator
	server           *http.Server
	stores           *store.Stores
	shutdownCtx      context.Context
	shutdownCancel   context.CancelFunc
	loginLimiter     api.LoginRateLimiting
	rateLimiter      api.RateLimiting
	agentRateLimiter api.RateLimiting
	apiServer        *api.Server
	cache            *valkey.Cache
	cancelSet        *valkey.CancelSet
	idempotency      *valkey.IdempotencyCache
	// otelShutdown flushes and stops the OpenTelemetry TracerProvider. It is a
	// no-op when tracing is disabled (the default). Set in wire().
	otelShutdown func(context.Context) error
}

func New(cfg *config.Config) (*App, error) {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	a := &App{
		cfg:            cfg,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}
	if err := a.wire(); err != nil {
		shutdownCancel()
		return nil, err
	}
	return a, nil
}

func (a *App) Run() error {
	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("http server panic", "component", "app", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		logger.Info("Starting server", "port", a.cfg.Port)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	// Honor SIGINT (Ctrl-C, local dev) and SIGTERM (Kubernetes pod rollout);
	// missing SIGTERM previously left the graceful-shutdown handler inert
	// during deployments.
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		a.shutdownCancel()
		if shutErr := a.Shutdown(context.Background()); shutErr != nil {
			logger.Error("Server-error-path shutdown returned error", "component", "app", "error", shutErr)
		}
		return fmt.Errorf("server error: %w", err)
	case <-quit:
		logger.Info("Shutting down server")
	}

	a.shutdownCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.Shutdown(ctx)
}

func (a *App) Shutdown(ctx context.Context) error {
	// Flush and stop the OpenTelemetry TracerProvider so any buffered spans
	// are exported before exit. No-op when tracing is disabled (the default).
	if a.otelShutdown != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := a.otelShutdown(flushCtx); err != nil {
			logger.Error("failed to shut down tracing", "error", err)
		}
		flushCancel()
	}

	if a.correlator != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := a.correlator.Flush(flushCtx); err != nil {
			logger.Error("Failed to flush correlator on shutdown", "error", err)
		}
		flushCancel()
	}

	if a.scheduler != nil {
		a.scheduler.Stop()
	}

	if a.workerSet != nil {
		a.workerSet.Stop()
	}

	if a.apiServer != nil {
		a.apiServer.StopSummaryScheduler()
	}

	if a.loginLimiter != nil {
		a.loginLimiter.Stop()
	}
	if a.rateLimiter != nil {
		a.rateLimiter.Stop()
	}
	if a.agentRateLimiter != nil {
		a.agentRateLimiter.Stop()
	}

	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil {
			logger.Error("Server forced to shutdown", "error", err)
		}
	}

	if a.rabbitClient != nil {
		_ = a.rabbitClient.Close()
	}
	if a.valkeyClient != nil {
		a.valkeyClient.Close()
	}
	if a.dbCli != nil {
		a.dbCli.Close()
	}

	logger.Info("Server exited")
	logger.Close()
	return nil
}
