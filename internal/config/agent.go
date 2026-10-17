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
	// CryptoKey is the path to an RSA public key PEM file used to encrypt
	// agent-to-server payloads. Empty disables encryption.
	CryptoKey string `env:"CRYPTO_KEY"`
	// GRPCAddress is the host:port of the server's gRPC endpoint. When set,
	// metrics are sent over gRPC instead of HTTP; empty keeps HTTP.
	GRPCAddress string `env:"GRPC_ADDRESS"`
	// GRPCCA is the path to the PEM certificate the agent trusts for the
	// gRPC server's TLS certificate. Required when GRPCAddress is set.
	GRPCCA string `env:"GRPC_CA"`
}

// NewAgentConfig builds an AgentConfig from defaults, flags, and env vars.
// Returns an error if flag parsing fails, poll interval, report interval,
// or rate limit end up zero, the gRPC address isn't in host:port form, or
// gRPC is enabled without a readable trusted CA certificate.
func NewAgentConfig() (*AgentConfig, error) {
	conf := new(AgentConfig)
	setAgentDefaults(conf)
	if path := discoverConfigPath(os.Args[1:]); path != "" {
		if err := applyAgentConfigFile(conf, path); err != nil {
			return nil, err
		}
	}
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
	pollIntervalFlag := flag.Uint("p", cfg.PollInterval, "poll interval in seconds")
	reportIntervalFlag := flag.Uint("r", cfg.ReportInterval, "report interval in seconds")
	rateLimitFlag := flag.Uint("l", cfg.RateLimit, "max number of concurrent outgoing report requests")
	key := flag.String("k", cfg.Key, "signing key")
	cryptoKey := flag.String(
		"crypto-key", cfg.CryptoKey, "path to RSA public key file for encrypting agent-to-server messages",
	)
	grpcAddress := flag.String(
		"grpc-address", cfg.GRPCAddress,
		"server gRPC address in form host:port; empty sends metrics over HTTP. Requires -grpc-ca. "+
			"gRPC traffic is encrypted with TLS; signing (-k) and body encryption (-crypto-key) apply to HTTP only",
	)
	grpcCA := flag.String(
		"grpc-ca", cfg.GRPCCA,
		"path to the PEM certificate trusted for the gRPC server's TLS certificate; required with -grpc-address",
	)
	var configPath string
	flag.StringVar(&configPath, "c", "", "path to JSON config file")
	flag.StringVar(&configPath, "config", "", "path to JSON config file")

	flag.Parse()

	cfg.PollInterval = *pollIntervalFlag
	cfg.ReportInterval = *reportIntervalFlag
	cfg.RateLimit = *rateLimitFlag
	cfg.Key = *key
	cfg.CryptoKey = *cryptoKey
	cfg.GRPCAddress = *grpcAddress
	cfg.GRPCCA = *grpcCA

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
	if cfg.CryptoKey != "" {
		if _, err := os.Stat(cfg.CryptoKey); err != nil {
			return apperrors.ErrCryptoKeyUnavailable
		}
	}
	if err := validateGRPCAddress(cfg.GRPCAddress); err != nil {
		return err
	}
	return validateGRPCTLS(cfg.GRPCAddress, grpcTLSFile{name: "-grpc-ca/GRPC_CA", path: cfg.GRPCCA})
}

// agentFileConfig is the JSON shape of the agent's config file. Fields are
// pointers so a key absent from the file leaves the built-in default
// untouched, which a zero-valued struct field couldn't distinguish from
// an explicit zero/empty value.
type agentFileConfig struct {
	Address        *string `json:"address"`
	ReportInterval *string `json:"report_interval"`
	PollInterval   *string `json:"poll_interval"`
	CryptoKey      *string `json:"crypto_key"`
	GRPCAddress    *string `json:"grpc_address"`
	GRPCCA         *string `json:"grpc_ca"`
}

// applyAgentConfigFile reads the JSON config file at path and merges its
// values into cfg as new defaults, to be called before parseAgentFlags so
// flag.Parse (and later env.Parse) can still override them. Only keys
// present in the file are applied.
func applyAgentConfigFile(cfg *AgentConfig, path string) error {
	var fc agentFileConfig
	if err := readConfigFile(path, &fc); err != nil {
		return err
	}

	if fc.Address != nil {
		if err := cfg.Set(*fc.Address); err != nil {
			return err
		}
	}
	if fc.ReportInterval != nil {
		secs, err := parseDurationSeconds(*fc.ReportInterval)
		if err != nil {
			return err
		}
		cfg.ReportInterval = uint(secs)
	}
	if fc.PollInterval != nil {
		secs, err := parseDurationSeconds(*fc.PollInterval)
		if err != nil {
			return err
		}
		cfg.PollInterval = uint(secs)
	}
	if fc.CryptoKey != nil {
		cfg.CryptoKey = *fc.CryptoKey
	}
	if fc.GRPCAddress != nil {
		cfg.GRPCAddress = *fc.GRPCAddress
	}
	if fc.GRPCCA != nil {
		cfg.GRPCCA = *fc.GRPCCA
	}

	return nil
}
