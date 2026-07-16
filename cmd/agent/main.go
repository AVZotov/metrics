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

	go collectLoop(ctx, a, time.Duration(cfg.PollInterval)*time.Second)
	go reportLoop(ctx, a, logger, time.Duration(cfg.ReportInterval)*time.Second)

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

func reportLoop(ctx context.Context, a *agent.Agent, logger *zap.Logger, duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.Report(ctx); err != nil {
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
