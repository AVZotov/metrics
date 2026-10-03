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

// shutdownTimeout bounds, on shutdown, how long an in-flight send may run
// before it is cancelled, and separately bounds the final flush.
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
	serverAddr := cfg.String()
	baseURL := fmt.Sprintf("http://%s", serverAddr)
	// The agent can't know whether the server enforces a trusted subnet, so
	// a failure here only drops the X-Real-IP header instead of exiting.
	realIP, err := agent.HostIP(serverAddr)
	if err != nil {
		logger.Warn("could not determine host IP, X-Real-IP header disabled", zap.Error(err))
	} else {
		logger.Info("determined host IP for X-Real-IP header", zap.String("ip", realIP.String()))
	}
	a, err := agent.NewAgent(client, baseURL, cfg.Key, cfg.CryptoKey, realIP)
	if err != nil {
		return err
	}
	jobs := make(chan []models.Metrics, cfg.RateLimit)

	// sendCtx bounds every report send. It is deliberately not derived from
	// ctx: a send in flight when the signal arrives should run to completion,
	// since aborting a request the server may already be applying makes the
	// final flush deliver the same counter deltas again. It is cancelled
	// only if draining overruns shutdownTimeout.
	sendCtx, cancelSends := context.WithCancel(context.Background())
	defer cancelSends()

	var wg sync.WaitGroup
	for i := uint(0); i < cfg.RateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reportWorker(ctx, sendCtx, jobs, a, logger)
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
	logger.Info("shutdown signal received, stopping agent loops")

	stopWorkers(&wg, cancelSends, shutdownTimeout, logger)

	flushPending(a, logger)

	logger.Info("agent shut down")
	return nil
}

// stopWorkers waits up to timeout for all agent goroutines to finish. If
// that expires, it cancels in-flight sends via cancelSends and then waits
// for the goroutines to actually return, so the caller can flush the
// agent's snapshot without a worker still delivering the same counter
// deltas. The second wait is short: cancelling aborts both a pending retry
// backoff and an in-flight request.
//
// Known limitation: a request cancelled here may already be applied by the
// server (it doesn't stop processing when the client goes away), and since
// the agent never saw it confirmed, the final flush resends its deltas and
// they are counted twice. This needs the server to still be processing a
// request after the full timeout; closing it for good would require
// server-side deduplication of batches.
func stopWorkers(wg *sync.WaitGroup, cancelSends context.CancelFunc, timeout time.Duration, logger *zap.Logger) {
	if waitWithTimeout(wg, timeout) {
		logger.Info("all agent loops stopped cleanly")
		return
	}
	logger.Warn("shutdown timeout expired, cancelling in-flight sends")
	cancelSends()
	wg.Wait()
	logger.Info("all agent loops stopped after cancellation")
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
// report tick, and any queued or aborted report, aren't lost on shutdown.
// Unacked counter deltas stay in the agent, so the snapshot already covers
// every report that was never confirmed delivered.
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

func reportLoop(ctx context.Context, a *agent.Agent, jobs chan<- []models.Metrics, duration time.Duration) {
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

// reportWorker sends queued snapshots with sendCtx until ctx is cancelled.
// A send already in progress when ctx is cancelled is finished, not
// aborted. Queued jobs are abandoned rather than drained: their counter
// deltas were never acked, so the final flush delivers them anyway.
func reportWorker(ctx, sendCtx context.Context, jobs <-chan []models.Metrics, a *agent.Agent, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case metrics := <-jobs:
			// select picks randomly when both cases are ready; don't start a
			// new send once shutdown has begun.
			if ctx.Err() != nil {
				return
			}
			if err := a.SendWithRetry(sendCtx, metrics); err != nil {
				logReportError(logger, err)
				continue
			}
			a.AckSent(metrics)
		}
	}
}

func logReportError(logger *zap.Logger, err error) {
	if retryErr, ok := errors.AsType[*apperrors.RetryError](err); ok && retryErr.Succeeded {
		logger.Warn("report succeeded after retries", zap.Error(err))
		return
	}
	logger.Error("report failed", zap.Error(err))
}
