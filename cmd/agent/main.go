package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AVZotov/metrics/internal/agent"
	"github.com/AVZotov/metrics/internal/config"
	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"go.uber.org/zap"
)

func main() {
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	cfg, err := config.NewAgentConfig()
	if err != nil {
		return err
	}
	client := &http.Client{}
	baseURL := fmt.Sprintf("http://%s", cfg.String())
	a := agent.NewAgent(client, baseURL, cfg.Key)

	jobs := make(chan []models.Metrics, cfg.RateLimit)
	for i := uint(0); i < cfg.RateLimit; i++ {
		go reportWorker(ctx, jobs, a, logger)
	}

	go collectLoop(ctx, a, time.Duration(cfg.PollInterval)*time.Second)
	go gopsutilLoop(ctx, a, logger, time.Duration(cfg.PollInterval)*time.Second)
	go reportLoop(ctx, a, jobs, time.Duration(cfg.ReportInterval)*time.Second)

	<-ctx.Done()
	logger.Info("shutting down agent")
	return nil
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

func reportWorker(ctx context.Context, jobs <-chan []models.Metrics, a *agent.Agent, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case metrics := <-jobs:
			if err := a.SendWithRetry(ctx, metrics); err != nil {
				logReportError(logger, err)
			}
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
