package config

import (
	"flag"
	"os"
	"testing"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Меняем поведение на "продолжить" и сбрасываем перед каждым суб тестом
func resetFlags() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
}

func TestSetAgentDefaults(t *testing.T) {
	cfg := &AgentConfig{}
	setAgentDefaults(cfg)

	assert.Equal(t, Host, cfg.Host)
	assert.Equal(t, Port, cfg.Port)
	assert.Equal(t, uint(PollInterval), cfg.PollInterval)
	assert.Equal(t, uint(ReportInterval), cfg.ReportInterval)
	assert.Equal(t, uint(RateLimit), cfg.RateLimit)
}

func TestValidateAgentConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AgentConfig
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     AgentConfig{PollInterval: 2, ReportInterval: 10, RateLimit: 1},
			wantErr: nil,
		},
		{
			name:    "zero poll interval",
			cfg:     AgentConfig{PollInterval: 0, ReportInterval: 10, RateLimit: 1},
			wantErr: apperrors.ErrInvalidPollInterval,
		},
		{
			name:    "zero report interval",
			cfg:     AgentConfig{PollInterval: 2, ReportInterval: 0, RateLimit: 1},
			wantErr: apperrors.ErrInvalidReportInterval,
		},
		{
			name:    "zero rate limit",
			cfg:     AgentConfig{PollInterval: 2, ReportInterval: 10, RateLimit: 0},
			wantErr: apperrors.ErrInvalidRateLimit,
		},
		{
			name:    "both intervals zero returns poll error first",
			cfg:     AgentConfig{PollInterval: 0, ReportInterval: 0, RateLimit: 1},
			wantErr: apperrors.ErrInvalidPollInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgentConfig(&tt.cfg)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestParseAgentEnv(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		wantHost   string
		wantPort   int
		wantPoll   uint
		wantReport uint
		wantErr    bool
	}{
		{
			name:       "no env vars preserves defaults",
			wantHost:   Host,
			wantPort:   Port,
			wantPoll:   PollInterval,
			wantReport: ReportInterval,
		},
		{
			name:       "ADDRESS overrides host and port",
			envVars:    map[string]string{"ADDRESS": "localhost:9090"},
			wantHost:   "localhost",
			wantPort:   9090,
			wantPoll:   PollInterval,
			wantReport: ReportInterval,
		},
		{
			name:       "POLL_INTERVAL and REPORT_INTERVAL override defaults",
			envVars:    map[string]string{"POLL_INTERVAL": "5", "REPORT_INTERVAL": "30"},
			wantHost:   Host,
			wantPort:   Port,
			wantPoll:   5,
			wantReport: 30,
		},
		{
			name: "all env vars set",
			envVars: map[string]string{
				"ADDRESS":         "localhost:7070",
				"POLL_INTERVAL":   "3",
				"REPORT_INTERVAL": "15",
			},
			wantHost:   "localhost",
			wantPort:   7070,
			wantPoll:   3,
			wantReport: 15,
		},
		{
			name:    "invalid ADDRESS format returns error",
			envVars: map[string]string{"ADDRESS": "badaddress"},
			wantErr: true,
		},
		{
			name:    "invalid POLL_INTERVAL value returns error",
			envVars: map[string]string{"POLL_INTERVAL": "notanumber"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg := &AgentConfig{}
			setAgentDefaults(cfg)
			err := parseAgentEnv(cfg)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, cfg.Host)
			assert.Equal(t, tt.wantPort, cfg.Port)
			assert.Equal(t, tt.wantPoll, cfg.PollInterval)
			assert.Equal(t, tt.wantReport, cfg.ReportInterval)
		})
	}
}

func TestParseAgentFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantHost   string
		wantPort   int
		wantPoll   uint
		wantReport uint
		wantErr    error
	}{
		{
			name:       "no flags preserves defaults",
			args:       []string{"cmd"},
			wantHost:   Host,
			wantPort:   Port,
			wantPoll:   PollInterval,
			wantReport: ReportInterval,
		},
		{
			name:       "-a flag overrides address",
			args:       []string{"cmd", "-a", "localhost:9000"},
			wantHost:   "localhost",
			wantPort:   9000,
			wantPoll:   PollInterval,
			wantReport: ReportInterval,
		},
		{
			name:       "-p and -r flags override intervals",
			args:       []string{"cmd", "-p", "4", "-r", "20"},
			wantHost:   Host,
			wantPort:   Port,
			wantPoll:   4,
			wantReport: 20,
		},
		{
			name:       "all flags set",
			args:       []string{"cmd", "-a", "remotehost:9000", "-p", "4", "-r", "20"},
			wantHost:   "remotehost",
			wantPort:   9000,
			wantPoll:   4,
			wantReport: 20,
		},
		{
			name:    "unknown positional argument returns error",
			args:    []string{"cmd", "unknownarg"},
			wantErr: apperrors.ErrUnknownFlags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags()
			origArgs := os.Args
			os.Args = tt.args
			defer func() { os.Args = origArgs }()

			cfg := &AgentConfig{}
			setAgentDefaults(cfg)
			err := parseAgentFlags(cfg)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, cfg.Host)
			assert.Equal(t, tt.wantPort, cfg.Port)
			assert.Equal(t, tt.wantPoll, cfg.PollInterval)
			assert.Equal(t, tt.wantReport, cfg.ReportInterval)
		})
	}
}
