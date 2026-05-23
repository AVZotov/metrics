package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	
	e "github.com/AVZotov/metrics/internal/errors"
)

var _ flag.Value = (*ServerConfig)(nil)

type ServerConfig struct {
	Host string
	Port int
}

func (s *ServerConfig) String() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func (s *ServerConfig) Set(str string) error {
	hp := strings.Split(str, ":")
	if len(hp) != 2 {
		return errors.New("need address in form host:port")
	}
	s.Host = hp[0]
	p, err := strconv.Atoi(hp[1])
	if err != nil {
		return e.ErrInvalidValue
	}
	s.Port = p
	return nil
}

func NewServerConfig() *ServerConfig {
	conf := new(ServerConfig)
	parseServerFlags(conf)
	setServerDefaults(conf)
	return conf
}

func parseServerFlags(config *ServerConfig) {
	flag.Var(config, "a", "address in form host:port")
	
	flag.Parse()
	
	if flag.NArg() > 0 {
		for _, arg := range flag.Args() {
			_, _ = fmt.Fprintf(os.Stderr, "unknown argument: %s\n", arg)
		}
		flag.Usage()
		os.Exit(1)
	}
}

func setServerDefaults(s *ServerConfig) {
	if s.Host == "" {
		s.Host = Host
	}
	if s.Port == 0 {
		s.Port = Port
	}
}
