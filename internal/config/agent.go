package config

import (
	"flag"
	"fmt"
	"os"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/caarlos0/env/v11"
)

// AgentConfig holds the agent's runtime configuration, populated from
// flags and environment variables (env vars take precedence).
type AgentConfig struct {
	Address        `env:"ADDRESS"`
	PollInterval   uint   `env:"POLL_INTERVAL"`
	ReportInterval uint   `env:"REPORT_INTERVAL"`
	RateLimit      uint   `env:"RATE_LIMIT"`
	Key            string `env:"KEY"`
}

// NewAgentConfig builds an AgentConfig from defaults, flags, and env vars.
// Returns an error if flag parsing fails or poll interval, report interval,
// or rate limit end up zero.
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
	cfg.Host = host
	cfg.Port = port
	cfg.PollInterval = pollInterval
	cfg.ReportInterval = reportInterval
	cfg.RateLimit = rateLimit
}

func parseAgentFlags(cfg *AgentConfig) error {
	flag.Var(&cfg.Address, "a", "address in form host:port")
	pollIntervalFlag := flag.Uint("p", pollInterval, "poll interval in seconds")
	reportIntervalFlag := flag.Uint("r", reportInterval, "report interval in seconds")
	rateLimitFlag := flag.Uint("l", rateLimit, "max number of concurrent outgoing report requests")
	key := flag.String("k", "", "signing key")

	flag.Parse()

	cfg.PollInterval = *pollIntervalFlag
	cfg.ReportInterval = *reportIntervalFlag
	cfg.RateLimit = *rateLimitFlag
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
