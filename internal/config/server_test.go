package config

import (
	"os"
	"testing"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetServerDefaults(t *testing.T) {
	cfg := &ServerConfig{}
	setServerDefaults(cfg)

	assert.Equal(t, Host, cfg.Host)
	assert.Equal(t, Port, cfg.Port)
}

func TestParseServerEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "no env vars preserves defaults",
			wantHost: Host,
			wantPort: Port,
		},
		{
			name:     "ADDRESS overrides host and port",
			envVars:  map[string]string{"ADDRESS": "localhost:9090"},
			wantHost: "localhost",
			wantPort: 9090,
		},
		{
			name:    "invalid ADDRESS format returns error",
			envVars: map[string]string{"ADDRESS": "badaddress"},
			wantErr: true,
		},
		{
			name:    "ADDRESS with non-numeric port returns error",
			envVars: map[string]string{"ADDRESS": "localhost:notaport"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg := &ServerConfig{}
			setServerDefaults(cfg)
			err := parseServerEnv(cfg)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, cfg.Host)
			assert.Equal(t, tt.wantPort, cfg.Port)
		})
	}
}

func TestParseServerFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantHost string
		wantPort int
		wantErr  error
	}{
		{
			name:     "no flags preserves defaults",
			args:     []string{"cmd"},
			wantHost: Host,
			wantPort: Port,
		},
		{
			name:     "-a flag overrides address",
			args:     []string{"cmd", "-a", "remotehost:9000"},
			wantHost: "remotehost",
			wantPort: 9000,
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

			cfg := &ServerConfig{}
			setServerDefaults(cfg)
			err := parseServerFlags(cfg)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, cfg.Host)
			assert.Equal(t, tt.wantPort, cfg.Port)
		})
	}
}
