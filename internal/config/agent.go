package config

import (
	"flag"
	"fmt"
	"os"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/caarlos0/env/v11"
)

type AgentConfig struct {
	Address        `env:"ADDRESS"`
	PollInterval   uint   `env:"POLL_INTERVAL"`
	ReportInterval uint   `env:"REPORT_INTERVAL"`
	RateLimit      uint   `env:"RATE_LIMIT"`
	Key            string `env:"KEY"`
}

func NewAgentConfig() (*AgentConfig, error) {
	conf := new(AgentConfig)
	setAgentDefaults(conf)
	if err := parseAgentFlags(conf); err != nil {
		return nil, err
	}
	if err := parseAgentEnv(conf); err != nil {
		return nil, err
	}
	if err := validateAgentConfig(conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func setAgentDefaults(cfg *AgentConfig) {
	cfg.Host = Host
	cfg.Port = Port
	cfg.PollInterval = PollInterval
	cfg.ReportInterval = ReportInterval
	cfg.RateLimit = RateLimit
}

func parseAgentFlags(cfg *AgentConfig) error {
	flag.Var(&cfg.Address, "a", "address in form host:port")
	pollInterval := flag.Uint("p", PollInterval, "poll interval in seconds")
	reportInterval := flag.Uint("r", ReportInterval, "report interval in seconds")
	rateLimit := flag.Uint("l", RateLimit, "max number of concurrent outgoing report requests")
	key := flag.String("k", "", "signing key")

	flag.Parse()

	cfg.PollInterval = *pollInterval
	cfg.ReportInterval = *reportInterval
	cfg.RateLimit = *rateLimit
	cfg.Key = *key

	if flag.NArg() > 0 {
		for _, arg := range flag.Args() {
			_, _ = fmt.Fprintf(os.Stderr, "unknown argument: %s\n", arg)
		}
		flag.Usage()
		return apperrors.ErrUnknownFlags
	}
	return nil
}

func parseAgentEnv(cfg *AgentConfig) error {
	return env.Parse(cfg)
}

func validateAgentConfig(cfg *AgentConfig) error {
	if cfg.PollInterval == 0 {
		return apperrors.ErrInvalidPollInterval
	}
	if cfg.ReportInterval == 0 {
		return apperrors.ErrInvalidReportInterval
	}
	if cfg.RateLimit == 0 {
		return apperrors.ErrInvalidRateLimit
	}
	return nil
}
