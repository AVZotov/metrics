package config

import (
	"os"
	"path/filepath"
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
	assert.Equal(t, StoreInterval, cfg.StoreInterval)
	assert.Equal(t, Restore, cfg.Restore)
	assert.Equal(t, FileStoragePath, cfg.FileStoragePath)
}

func TestParseFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		inputPath string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "empty path is allowed (mem-only mode)",
			inputPath: "",
			wantPath:  "",
		},
		{
			name:      "existing directory returns error",
			inputPath: tmpDir,
			wantErr:   true,
		},
		{
			name:      "simple relative path is unchanged",
			inputPath: filepath.Join("data", "metrics.json"),
			wantPath:  filepath.Join("data", "metrics.json"),
		},
		{
			name:      "dot-dot components are resolved",
			inputPath: filepath.Join("data", "..", "metrics.json"),
			wantPath:  "metrics.json",
		},
		{
			name:      "redundant separators removed",
			inputPath: "data" + string(filepath.Separator) + string(filepath.Separator) + "metrics.json",
			wantPath:  filepath.Join("data", "metrics.json"),
		},
		{
			name:      "trailing separator is removed",
			inputPath: filepath.Join("data", "metrics.json") + string(filepath.Separator),
			wantPath:  filepath.Join("data", "metrics.json"),
		},
		{
			name:      "absolute path to non-existing file is accepted",
			inputPath: filepath.Join(tmpDir, "metrics.json"),
			wantPath:  filepath.Join(tmpDir, "metrics.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ServerConfig{FileStoragePath: tt.inputPath}
			err := parseFilePath(cfg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, cfg.FileStoragePath)
		})
	}
}

func TestValidateDSN(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerConfig
		wantErr bool
	}{
		{
			name:    "DSN not provided - no error",
			cfg:     ServerConfig{DSNSet: false, DSN: ""},
			wantErr: false,
		},
		{
			name:    "DSN explicitly set but empty - error",
			cfg:     ServerConfig{DSNSet: true, DSN: ""},
			wantErr: true,
		},
		{
			name:    "DSN set with value - no error",
			cfg:     ServerConfig{DSNSet: true, DSN: "postgres://user:pass@localhost/db"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDSN(&tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestParseServerEnv(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		wantHost        string
		wantPort        int
		wantStoreInt    int
		wantRestore     bool
		wantStoragePath string
		wantDSNSet      bool
		wantDSN         string
		wantErr         bool
	}{
		{
			name:            "no env vars preserves defaults",
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "ADDRESS overrides host and port",
			envVars:         map[string]string{"ADDRESS": "localhost:9090"},
			wantHost:        "localhost",
			wantPort:        9090,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "STORE_INTERVAL overrides default",
			envVars:         map[string]string{"STORE_INTERVAL": "60"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    60,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "RESTORE=false overrides default",
			envVars:         map[string]string{"RESTORE": "false"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     false,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "FILE_STORAGE_PATH overrides default",
			envVars:         map[string]string{"FILE_STORAGE_PATH": "/tmp/metrics.json"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: "/tmp/metrics.json",
		},
		{
			name: "all env vars set",
			envVars: map[string]string{
				"ADDRESS":           "remotehost:7070",
				"STORE_INTERVAL":    "120",
				"RESTORE":           "false",
				"FILE_STORAGE_PATH": "/var/metrics.json",
			},
			wantHost:        "remotehost",
			wantPort:        7070,
			wantStoreInt:    120,
			wantRestore:     false,
			wantStoragePath: "/var/metrics.json",
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
		{
			name:    "invalid STORE_INTERVAL value returns error",
			envVars: map[string]string{"STORE_INTERVAL": "notanumber"},
			wantErr: true,
		},
		{
			name:            "DATABASE_DSN set but empty marks DSNSet true",
			envVars:         map[string]string{"DATABASE_DSN": ""},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
			wantDSNSet:      true,
			wantDSN:         "",
		},
		{
			name:            "DATABASE_DSN set with value marks DSNSet true",
			envVars:         map[string]string{"DATABASE_DSN": "postgres://user:pass@localhost/db"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
			wantDSNSet:      true,
			wantDSN:         "postgres://user:pass@localhost/db",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
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
				assert.Equal(t, tt.wantStoreInt, cfg.StoreInterval)
				assert.Equal(t, tt.wantRestore, cfg.Restore)
				assert.Equal(t, tt.wantStoragePath, cfg.FileStoragePath)
				assert.Equal(t, tt.wantDSNSet, cfg.DSNSet)
				assert.Equal(t, tt.wantDSN, cfg.DSN)
			},
		)
	}
}

func TestParseServerFlags(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantHost        string
		wantPort        int
		wantStoreInt    int
		wantRestore     bool
		wantStoragePath string
		wantDSNSet      bool
		wantDSN         string
		wantErr         error
	}{
		{
			name:            "no flags preserves defaults",
			args:            []string{"cmd"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "-a flag overrides address",
			args:            []string{"cmd", "-a", "remotehost:9000"},
			wantHost:        "remotehost",
			wantPort:        9000,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "-i flag overrides store interval",
			args:            []string{"cmd", "-i", "60"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    60,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "-r flag disables restore",
			args:            []string{"cmd", "-r=false"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     false,
			wantStoragePath: FileStoragePath,
		},
		{
			name:            "-f flag overrides file storage path",
			args:            []string{"cmd", "-f", "/tmp/metrics.json"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: "/tmp/metrics.json",
		},
		{
			name: "all flags set",
			args: []string{
				"cmd", "-a", "remotehost:9000", "-i", "120", "-r=false", "-f", "/var/metrics.json",
			},
			wantHost:        "remotehost",
			wantPort:        9000,
			wantStoreInt:    120,
			wantRestore:     false,
			wantStoragePath: "/var/metrics.json",
		},
		{
			name:    "unknown positional argument returns error",
			args:    []string{"cmd", "unknownarg"},
			wantErr: apperrors.ErrUnknownFlags,
		},
		{
			name:            "-d flag with empty value marks DSNSet true",
			args:            []string{"cmd", "-d", ""},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
			wantDSNSet:      true,
			wantDSN:         "",
		},
		{
			name:            "-d flag with value marks DSNSet true",
			args:            []string{"cmd", "-d", "postgres://user:pass@localhost/db"},
			wantHost:        Host,
			wantPort:        Port,
			wantStoreInt:    StoreInterval,
			wantRestore:     Restore,
			wantStoragePath: FileStoragePath,
			wantDSNSet:      true,
			wantDSN:         "postgres://user:pass@localhost/db",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
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
				assert.Equal(t, tt.wantStoreInt, cfg.StoreInterval)
				assert.Equal(t, tt.wantRestore, cfg.Restore)
				assert.Equal(t, tt.wantStoragePath, cfg.FileStoragePath)
				assert.Equal(t, tt.wantDSNSet, cfg.DSNSet)
				assert.Equal(t, tt.wantDSN, cfg.DSN)
			},
		)
	}
}
