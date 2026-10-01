package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	dbcfg "github.com/AVZotov/metrics/internal/config/db"
	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/caarlos0/env/v11"
)

// AuditConfig holds where audit events are sent: a file path, a URL, or both.
type AuditConfig struct {
	File string `env:"AUDIT_FILE"`
	URL  string `env:"AUDIT_URL"`
}

// ServerConfig holds the server's runtime configuration, populated from
// flags and environment variables (env vars take precedence).
type ServerConfig struct {
	Address             `env:"ADDRESS"`
	StoreInterval       int    `env:"STORE_INTERVAL"`
	Restore             bool   `env:"RESTORE"`
	FileStoragePath     string `env:"FILE_STORAGE_PATH"`
	EnablePprof         bool   `env:"ENABLE_PPROF"`
	ShutdownGracePeriod uint
	DSN                 string `env:"DATABASE_DSN"`
	DSNSet              bool
	DB                  dbcfg.Config
	Key                 string `env:"KEY"`
	Audit               AuditConfig
	// CryptoKey is the path to an RSA private key PEM file used to decrypt
	// agent-to-server payloads. Empty disables decryption.
	CryptoKey string `env:"CRYPTO_KEY"`
}

// NewServerConfig builds a ServerConfig from defaults, flags, and env vars.
// Returns an error if flag parsing fails, the DSN is explicitly set but
// empty, or the audit file path or audit URL is invalid.
func NewServerConfig() (*ServerConfig, error) {
	conf := new(ServerConfig)
	setServerDefaults(conf)
	if path := discoverConfigPath(os.Args[1:]); path != "" {
		if err := applyServerConfigFile(conf, path); err != nil {
			return nil, err
		}
	}
	if err := parseServerFlags(conf); err != nil {
		return nil, err
	}
	if err := parseServerEnv(conf); err != nil {
		return nil, err
	}
	if err := parseFilePath(conf); err != nil {
		return nil, err
	}
	if err := validateDSN(conf); err != nil {
		return nil, err
	}
	if err := parseAuditFilePath(conf); err != nil {
		return nil, err
	}
	if err := validateAuditURL(conf); err != nil {
		return nil, err
	}
	if err := validateCryptoKey(conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func setServerDefaults(s *ServerConfig) {
	s.Host = host
	s.Port = port
	s.StoreInterval = storeInterval
	s.Restore = restore
	s.FileStoragePath = fileStoragePath
	s.EnablePprof = enablePprof
	s.ShutdownGracePeriod = serverShutdownGracePeriod
	s.DB = dbcfg.Config{ConnectTimeout: dbConnectTimeout, QueryTimeout: dbQueryTimeout}
}

func parseServerFlags(config *ServerConfig) error {
	flag.Var(&config.Address, "a", "address in form host:port")
	flag.IntVar(&config.StoreInterval, "i", config.StoreInterval, "metrics save interval in seconds")
	flag.BoolVar(&config.Restore, "r", config.Restore, "restore store on server restart")
	flag.StringVar(&config.FileStoragePath, "f", config.FileStoragePath, "store path")
	flag.BoolVar(&config.EnablePprof, "enable-pprof", enablePprof, "mount /debug/pprof profiler endpoints")
	flag.StringVar(&config.DSN, "d", config.DSN, "database connection DSN")
	flag.StringVar(&config.Key, "k", config.Key, "signing key")
	flag.StringVar(&config.Audit.File, "audit-file", "", "path to audit log file")
	flag.StringVar(&config.Audit.URL, "audit-url", "", "URL to send audit events")
	flag.StringVar(
		&config.CryptoKey, "crypto-key", config.CryptoKey,
		"path to RSA private key file for decrypting agent-to-server messages",
	)
	var configPath string
	flag.StringVar(&configPath, "c", "", "path to JSON config file")
	flag.StringVar(&configPath, "config", "", "path to JSON config file")

	flag.Parse()

	flag.Visit(
		func(f *flag.Flag) {
			if f.Name == "d" {
				config.DSNSet = true
			}
		},
	)

	if flag.NArg() > 0 {
		for _, arg := range flag.Args() {
			_, _ = fmt.Fprintf(os.Stderr, "unknown argument: %s\n", arg)
		}
		flag.Usage()
		return apperrors.ErrUnknownFlags
	}
	return nil
}

func parseServerEnv(cfg *ServerConfig) error {
	if _, ok := os.LookupEnv("DATABASE_DSN"); ok {
		cfg.DSNSet = true
	}
	return env.Parse(cfg)
}

func cleanFilePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	cleaned := filepath.Clean(path)
	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		return "", errors.New("path must point to a file, not a directory")
	}
	return cleaned, nil
}

func parseFilePath(cfg *ServerConfig) error {
	cleaned, err := cleanFilePath(cfg.FileStoragePath)
	if err != nil {
		return err
	}
	cfg.FileStoragePath = cleaned
	return nil
}

func validateDSN(cfg *ServerConfig) error {
	if cfg.DSNSet && cfg.DSN == "" {
		return errors.New("database DSN explicitly provided but is empty")
	}
	return nil
}

func parseAuditFilePath(cfg *ServerConfig) error {
	cleaned, err := cleanFilePath(cfg.Audit.File)
	if err != nil {
		return err
	}
	cfg.Audit.File = cleaned
	return nil
}

func validateAuditURL(cfg *ServerConfig) error {
	if cfg.Audit.URL == "" {
		return nil
	}

	parsed, err := url.Parse(cfg.Audit.URL)
	if err != nil {
		return fmt.Errorf("audit URL is invalid: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("audit URL must be an absolute URL with scheme and host")
	}

	return nil
}

func validateCryptoKey(cfg *ServerConfig) error {
	if cfg.CryptoKey == "" {
		return nil
	}
	if _, err := os.Stat(cfg.CryptoKey); err != nil {
		return apperrors.ErrCryptoKeyUnavailable
	}
	return nil
}

// serverFileConfig is the JSON shape of the server's config file. Fields
// are pointers so a key absent from the file leaves the built-in default
// untouched, which a zero-valued struct field couldn't distinguish from
// an explicit zero/false/empty value.
type serverFileConfig struct {
	Address       *string `json:"address"`
	Restore       *bool   `json:"restore"`
	StoreInterval *string `json:"store_interval"`
	StoreFile     *string `json:"store_file"`
	DatabaseDSN   *string `json:"database_dsn"`
	CryptoKey     *string `json:"crypto_key"`
}

// applyServerConfigFile reads the JSON config file at path and merges its
// values into cfg as new defaults, to be called before parseServerFlags
// so flag.Parse (and later env.Parse) can still override them. Only keys
// present in the file are applied. A non-empty database_dsn sets DSNSet,
// the same way the -d flag does; an empty one means "not configured",
// like an absent key.
func applyServerConfigFile(cfg *ServerConfig, path string) error {
	var fc serverFileConfig
	if err := readConfigFile(path, &fc); err != nil {
		return err
	}

	if fc.Address != nil {
		if err := cfg.Set(*fc.Address); err != nil {
			return err
		}
	}
	if fc.Restore != nil {
		cfg.Restore = *fc.Restore
	}
	if fc.StoreInterval != nil {
		secs, err := parseDurationSeconds(*fc.StoreInterval)
		if err != nil {
			return err
		}
		cfg.StoreInterval = secs
	}
	if fc.StoreFile != nil {
		cfg.FileStoragePath = *fc.StoreFile
	}
	if fc.DatabaseDSN != nil && *fc.DatabaseDSN != "" {
		cfg.DSN = *fc.DatabaseDSN
		cfg.DSNSet = true
	}
	if fc.CryptoKey != nil {
		cfg.CryptoKey = *fc.CryptoKey
	}

	return nil
}
