package config

import (
	"flag"
	"fmt"
	"os"
)

type ServerConfig struct {
	Address
}

func NewServerConfig() *ServerConfig {
	conf := new(ServerConfig)
	parseServerFlags(conf)
	setServerDefaults(conf)
	return conf
}

func parseServerFlags(config *ServerConfig) {
	flag.Var(&config.Address, "a", "address in form host:port")
	
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
