package config

import (
	"flag"
	"os"
	"path/filepath"
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

	assert.Equal(t, host, cfg.Host)
	assert.Equal(t, port, cfg.Port)
	assert.Equal(t, uint(pollInterval), cfg.PollInterval)
	assert.Equal(t, uint(reportInterval), cfg.ReportInterval)
	assert.Equal(t, uint(rateLimit), cfg.RateLimit)
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

func TestValidateAgentConfig_CryptoKey(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(existing, []byte("dummy"), 0o600))

	tests := []struct {
		name    string
		cfg     AgentConfig
		wantErr error
	}{
		{
			name:    "empty crypto key path is valid",
			cfg:     AgentConfig{PollInterval: 2, ReportInterval: 10, RateLimit: 1, CryptoKey: ""},
			wantErr: nil,
		},
		{
			name:    "existing crypto key path is valid",
			cfg:     AgentConfig{PollInterval: 2, ReportInterval: 10, RateLimit: 1, CryptoKey: existing},
			wantErr: nil,
		},
		{
			name:    "nonexistent crypto key path returns error",
			cfg:     AgentConfig{PollInterval: 2, ReportInterval: 10, RateLimit: 1, CryptoKey: "/nonexistent/path/to/key.pem"},
			wantErr: apperrors.ErrCryptoKeyUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgentConfig(&tt.cfg)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
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
			wantHost:   host,
			wantPort:   port,
			wantPoll:   pollInterval,
			wantReport: reportInterval,
		},
		{
			name:       "ADDRESS overrides host and port",
			envVars:    map[string]string{"ADDRESS": "localhost:9090"},
			wantHost:   "localhost",
			wantPort:   9090,
			wantPoll:   pollInterval,
			wantReport: reportInterval,
		},
		{
			name:       "POLL_INTERVAL and REPORT_INTERVAL override defaults",
			envVars:    map[string]string{"POLL_INTERVAL": "5", "REPORT_INTERVAL": "30"},
			wantHost:   host,
			wantPort:   port,
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

func TestApplyAgentConfigFile(t *testing.T) {
	t.Run("absent keys leave defaults untouched", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		path := writeJSONConfig(t, `{}`)

		require.NoError(t, applyAgentConfigFile(cfg, path))

		assert.Equal(t, host, cfg.Host)
		assert.Equal(t, port, cfg.Port)
		assert.Equal(t, uint(pollInterval), cfg.PollInterval)
		assert.Equal(t, uint(reportInterval), cfg.ReportInterval)
		assert.Equal(t, "", cfg.CryptoKey)
	})

	t.Run("present keys override defaults", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		keyPath := filepath.Join(t.TempDir(), "key.pem")
		require.NoError(t, os.WriteFile(keyPath, []byte("dummy"), 0o600))
		path := writeJSONConfig(
			t, `{
			"address": "filehost:1234",
			"report_interval": "9s",
			"poll_interval": "3s",
			"crypto_key": "`+keyPath+`"
		}`,
		)

		require.NoError(t, applyAgentConfigFile(cfg, path))

		assert.Equal(t, "filehost", cfg.Host)
		assert.Equal(t, 1234, cfg.Port)
		assert.Equal(t, uint(9), cfg.ReportInterval)
		assert.Equal(t, uint(3), cfg.PollInterval)
		assert.Equal(t, keyPath, cfg.CryptoKey)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		err := applyAgentConfigFile(cfg, filepath.Join(t.TempDir(), "missing.json"))
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConfigFileUnavailable)
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		path := writeJSONConfig(t, `{"address": `)
		err := applyAgentConfigFile(cfg, path)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConfigFileMalformed)
	})

	t.Run("unknown key returns error", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		path := writeJSONConfig(t, `{"bogus_key": "x"}`)
		err := applyAgentConfigFile(cfg, path)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConfigFileMalformed)
	})

	t.Run("bad duration returns error", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		path := writeJSONConfig(t, `{"poll_interval": "not-a-duration"}`)
		err := applyAgentConfigFile(cfg, path)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrInvalidDuration)
	})

	t.Run("sub-second duration returns error", func(t *testing.T) {
		cfg := &AgentConfig{}
		setAgentDefaults(cfg)
		path := writeJSONConfig(t, `{"report_interval": "500ms"}`)
		err := applyAgentConfigFile(cfg, path)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrInvalidDuration)
	})
}

func TestNewAgentConfig_Precedence(t *testing.T) {
	run := func(t *testing.T, args []string, env map[string]string) *AgentConfig {
		t.Helper()
		resetFlags()
		origArgs := os.Args
		os.Args = append([]string{"cmd"}, args...)
		t.Cleanup(func() { os.Args = origArgs })
		for k, v := range env {
			t.Setenv(k, v)
		}
		cfg, err := NewAgentConfig()
		require.NoError(t, err)
		return cfg
	}

	t.Run("default only", func(t *testing.T) {
		cfg := run(t, nil, nil)
		assert.Equal(t, uint(pollInterval), cfg.PollInterval)
		assert.Equal(t, host, cfg.Host)
	})

	t.Run("file only", func(t *testing.T) {
		path := writeJSONConfig(t, `{"address": "filehost:1111", "poll_interval": "4s"}`)
		cfg := run(t, []string{"-c", path}, nil)
		assert.Equal(t, "filehost", cfg.Host)
		assert.Equal(t, 1111, cfg.Port)
		assert.Equal(t, uint(4), cfg.PollInterval)
	})

	t.Run("file overridden by flag", func(t *testing.T) {
		path := writeJSONConfig(t, `{"poll_interval": "4s"}`)
		cfg := run(t, []string{"-c", path, "-p", "8"}, nil)
		assert.Equal(t, uint(8), cfg.PollInterval)
	})

	t.Run("file overridden by env", func(t *testing.T) {
		path := writeJSONConfig(t, `{"poll_interval": "4s"}`)
		cfg := run(t, []string{"-c", path}, map[string]string{"POLL_INTERVAL": "6"})
		assert.Equal(t, uint(6), cfg.PollInterval)
	})

	t.Run("flag overridden by env: env wins", func(t *testing.T) {
		path := writeJSONConfig(t, `{"poll_interval": "4s"}`)
		cfg := run(t, []string{"-c", path, "-p", "8"}, map[string]string{"POLL_INTERVAL": "6"})
		assert.Equal(t, uint(6), cfg.PollInterval)
	})

	t.Run("absent keys keep default through full pipeline", func(t *testing.T) {
		path := writeJSONConfig(t, `{"poll_interval": "4s"}`)
		cfg := run(t, []string{"-c", path}, nil)
		assert.Equal(t, host, cfg.Host)
		assert.Equal(t, port, cfg.Port)
		assert.Equal(t, uint(reportInterval), cfg.ReportInterval)
	})

	t.Run("config path via CONFIG env", func(t *testing.T) {
		path := writeJSONConfig(t, `{"poll_interval": "11s"}`)
		cfg := run(t, nil, map[string]string{"CONFIG": path})
		assert.Equal(t, uint(11), cfg.PollInterval)
	})
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
			wantHost:   host,
			wantPort:   port,
			wantPoll:   pollInterval,
			wantReport: reportInterval,
		},
		{
			name:       "-a flag overrides address",
			args:       []string{"cmd", "-a", "localhost:9000"},
			wantHost:   "localhost",
			wantPort:   9000,
			wantPoll:   pollInterval,
			wantReport: reportInterval,
		},
		{
			name:       "-p and -r flags override intervals",
			args:       []string{"cmd", "-p", "4", "-r", "20"},
			wantHost:   host,
			wantPort:   port,
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
