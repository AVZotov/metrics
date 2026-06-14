package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	
	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/caarlos0/env/v11"
)

type ServerConfig struct {
	Address         `env:"ADDRESS"`
	StoreInterval   int    `env:"STORE_INTERVAL"`
	Restore         bool   `env:"RESTORE"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
}

func NewServerConfig() (*ServerConfig, error) {
	conf := new(ServerConfig)
	setServerDefaults(conf)
	if err := parseServerFlags(conf); err != nil {
		return nil, err
	}
	if err := parseServerEnv(conf); err != nil {
		return nil, err
	}
	if err := validateServetConfig(conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func validateServetConfig(conf *ServerConfig) error {
	if conf.FileStoragePath == "" {
		return errors.New("file storage path cannot be empty")
	}
	return nil
}

func setServerDefaults(s *ServerConfig) {
	s.Host = Host
	s.Port = Port
	s.StoreInterval = StoreInterval
	s.Restore = Restore
	s.FileStoragePath = FileStoragePath
}

func parseServerFlags(config *ServerConfig) error {
	flag.Var(&config.Address, "a", "address in form host:port")
	flag.IntVar(&config.StoreInterval, "i", StoreInterval, "metrics save interval in seconds")
	flag.BoolVar(&config.Restore, "r", Restore, "restore store on server restart")
	flag.StringVar(&config.FileStoragePath, "f", FileStoragePath, "store path")
	
	flag.Parse()
	
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
	return env.Parse(cfg)
}
