// Command agent polls runtime and system metrics on an interval and reports
// them to the metrics server, with retry on recoverable send failures.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/AVZotov/metrics/internal/agent"
	"github.com/AVZotov/metrics/internal/buildinfo"
	"github.com/AVZotov/metrics/internal/config"
	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"go.uber.org/zap"
)

var buildVersion string
var buildDate string
var buildCommit string

// shutdownTimeout bounds how long the agent waits, on shutdown, for
// in-flight and queued sends to finish, and is used as the context timeout
// for any send issued after the signal context is cancelled (since the
// cancelled ctx itself can no longer be used to bound a request).
const shutdownTimeout = 10 * time.Second

func main() {
	buildinfo.Print(buildVersion, buildDate, buildCommit)
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()
	if err := run(logger); err != nil {
		logger.Fatal(err.Error())
	}
}

func run(logger *zap.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer cancel()

	cfg, err := config.NewAgentConfig()
	if err != nil {
		return err
	}
	client := &http.Client{}
	baseURL := fmt.Sprintf("http://%s", cfg.String())
	a, err := agent.NewAgent(client, baseURL, cfg.Key, cfg.CryptoKey)
	if err != nil {
		return err
	}
	jobs := make(chan []models.Metrics, cfg.RateLimit)

	var wg sync.WaitGroup
	for i := uint(0); i < cfg.RateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reportWorker(ctx, jobs, a, logger)
		}()
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		collectLoop(ctx, a, time.Duration(cfg.PollInterval)*time.Second)
	}()
	go func() {
		defer wg.Done()
		gopsutilLoop(ctx, a, logger, time.Duration(cfg.PollInterval)*time.Second)
	}()
	go func() {
		defer wg.Done()
		reportLoop(ctx, a, jobs, time.Duration(cfg.ReportInterval)*time.Second)
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received, draining in-flight and queued work")

	if waitWithTimeout(&wg, shutdownTimeout) {
		logger.Info("all agent loops stopped cleanly")
	} else {
		logger.Warn("shutdown timeout expired before all agent loops stopped")
	}

	flushPending(a, logger)

	logger.Info("agent shut down")
	return nil
}

// waitWithTimeout waits for wg to finish, returning false if timeout
// elapses first. The goroutine it starts to watch wg leaks if wg never
// finishes, which is acceptable here since it only happens once, at
// process shutdown.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// flushPending sends the agent's remaining unsent snapshot after all
// collect/report loops have stopped, so metrics collected since the last
// report tick aren't lost on shutdown.
func flushPending(a *agent.Agent, logger *zap.Logger) {
	metrics := a.Snapshot()
	if len(metrics) == 0 {
		return
	}

	flushCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := a.SendWithRetry(flushCtx, metrics); err != nil {
		logReportError(logger, err)
		return
	}
	a.AckSent(metrics)
	logger.Info("final metrics flush succeeded")
}

func collectLoop(ctx context.Context, a *agent.Agent, duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Collect()
		}
	}
}

func gopsutilLoop(ctx context.Context, a *agent.Agent, logger *zap.Logger, duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.CollectGopsutil(); err != nil {
				logger.Warn("gopsutil collection failed", zap.Error(err))
			}
		}
	}
}

// reportLoop is the sole closer of jobs: closing it here, rather than in
// reportWorker, lets workers safely range over jobs until every queued
// send has been drained instead of abandoning them the moment ctx is
// cancelled.
func reportLoop(ctx context.Context, a *agent.Agent, jobs chan<- []models.Metrics, duration time.Duration) {
	defer close(jobs)
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := a.Snapshot()
			if len(metrics) == 0 {
				continue
			}
			select {
			case jobs <- metrics:
			case <-ctx.Done():
				return
			}
		}
	}
}

func reportWorker(ctx context.Context, jobs <-chan []models.Metrics, a *agent.Agent, logger *zap.Logger) {
	for metrics := range jobs {
		if err := sendMetrics(ctx, a, metrics); err != nil {
			logReportError(logger, err)
			continue
		}
		a.AckSent(metrics)
	}
}

// sendMetrics sends metrics using ctx during normal operation. Once ctx is
// cancelled, SendWithRetry's retry loop would abort immediately against
// it, so shutdown-time sends fall back to a fresh context bounded by
// shutdownTimeout, giving queued jobs a real chance to be delivered.
func sendMetrics(ctx context.Context, a *agent.Agent, metrics []models.Metrics) error {
	if ctx.Err() != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return a.SendWithRetry(shutdownCtx, metrics)
	}
	return a.SendWithRetry(ctx, metrics)
}

func logReportError(logger *zap.Logger, err error) {
	if retryErr, ok := errors.AsType[*apperrors.RetryError](err); ok && retryErr.Succeeded {
		logger.Warn("report succeeded after retries", zap.Error(err))
		return
	}
	logger.Error("report failed", zap.Error(err))
}
