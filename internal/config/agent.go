package config

import (
	"flag"
	"fmt"
	"os"
)

type AgentConfig struct {
	Address
	PollInterval   uint
	ReportInterval uint
}

func NewAgentConfig() *AgentConfig {
	conf := new(AgentConfig)
	parseAgentFlags(conf)
	setAgentDefaults(conf)
	return conf
}

func parseAgentFlags(config *AgentConfig) {
	flag.Var(&config.Address, "a", "address in form host:port")
	pollInterval := flag.Uint("p", PollInterval, "poll interval in seconds")
	reportInterval := flag.Uint("r", ReportInterval, "report interval in seconds")
	
	flag.Parse()
	
	config.PollInterval = *pollInterval
	config.ReportInterval = *reportInterval
	
	if flag.NArg() > 0 {
		for _, arg := range flag.Args() {
			_, _ = fmt.Fprintf(os.Stderr, "unknown argument: %s\n", arg)
		}
		flag.Usage()
		os.Exit(1)
	}
}

func setAgentDefaults(s *AgentConfig) {
	if s.Host == "" {
		s.Host = Host
	}
	if s.Port == 0 {
		s.Port = Port
	}
	// this sets defaults if poll or report intervals equals to zero
	if s.PollInterval == 0 {
		s.PollInterval = PollInterval
	}
	if s.ReportInterval == 0 {
		s.ReportInterval = ReportInterval
	}
}
