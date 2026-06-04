package config

import (
	"flag"
	"fmt"
	"os"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/caarlos0/env/v11"
)

type ServerConfig struct {
	Address `env:"ADDRESS"`
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
	return conf, nil
}

func setServerDefaults(s *ServerConfig) {
	s.Host = Host
	s.Port = Port
}

func parseServerFlags(config *ServerConfig) error {
	flag.Var(&config.Address, "a", "address in form host:port")

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
