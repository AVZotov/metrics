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

var _ flag.Value = (*AgentConfig)(nil)

type AgentConfig struct {
	Host           string
	Port           int
	PollInterval   uint
	ReportInterval uint
}

func (s *AgentConfig) String() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func (s *AgentConfig) Set(str string) error {
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

func NewAgentConfig() *AgentConfig {
	conf := new(AgentConfig)
	parseAgentFlags(conf)
	setAgentDefaults(conf)
	return conf
}

func parseAgentFlags(config *AgentConfig) {
	flag.Var(config, "a", "address in form host:port")
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
	// this sets defaults if poll or report equals to zero
	if s.PollInterval == 0 {
		s.PollInterval = PollInterval
	}
	if s.ReportInterval == 0 {
		s.ReportInterval = ReportInterval
	}
}
