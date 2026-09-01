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

	assert.Equal(t, host, cfg.Host)
	assert.Equal(t, port, cfg.Port)
	assert.Equal(t, storeInterval, cfg.StoreInterval)
	assert.Equal(t, restore, cfg.Restore)
	assert.Equal(t, fileStoragePath, cfg.FileStoragePath)
}

func TestCleanFilePath(t *testing.T) {
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
			got, err := cleanFilePath(tt.inputPath)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

func TestParseFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &ServerConfig{FileStoragePath: filepath.Join("data", "..", "metrics.json")}
	require.NoError(t, parseFilePath(cfg))
	assert.Equal(t, "metrics.json", cfg.FileStoragePath)

	cfg = &ServerConfig{FileStoragePath: tmpDir}
	require.Error(t, parseFilePath(cfg))
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

func TestParseAuditFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &ServerConfig{Audit: AuditConfig{File: filepath.Join("data", "audit.log")}}
	require.NoError(t, parseAuditFilePath(cfg))
	assert.Equal(t, filepath.Join("data", "audit.log"), cfg.Audit.File)

	cfg = &ServerConfig{Audit: AuditConfig{File: tmpDir}}
	require.Error(t, parseAuditFilePath(cfg))
}

func TestValidateAuditURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "empty URL is allowed",
			url:     "",
			wantErr: false,
		},
		{
			name:    "valid absolute URL",
			url:     "http://example.com/audit",
			wantErr: false,
		},
		{
			name:    "missing scheme and host is an error",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "missing host is an error",
			url:     "http://",
			wantErr: true,
		},
		{
			name:    "malformed URL is an error",
			url:     "http://[::1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ServerConfig{Audit: AuditConfig{URL: tt.url}}
			err := validateAuditURL(cfg)
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
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "ADDRESS overrides host and port",
			envVars:         map[string]string{"ADDRESS": "localhost:9090"},
			wantHost:        "localhost",
			wantPort:        9090,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "STORE_INTERVAL overrides default",
			envVars:         map[string]string{"STORE_INTERVAL": "60"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    60,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "RESTORE=false overrides default",
			envVars:         map[string]string{"RESTORE": "false"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     false,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "FILE_STORAGE_PATH overrides default",
			envVars:         map[string]string{"FILE_STORAGE_PATH": "/tmp/metrics.json"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
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
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
			wantDSNSet:      true,
			wantDSN:         "",
		},
		{
			name:            "DATABASE_DSN set with value marks DSNSet true",
			envVars:         map[string]string{"DATABASE_DSN": "postgres://user:pass@localhost/db"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
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
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "-a flag overrides address",
			args:            []string{"cmd", "-a", "remotehost:9000"},
			wantHost:        "remotehost",
			wantPort:        9000,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "-i flag overrides store interval",
			args:            []string{"cmd", "-i", "60"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    60,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "-r flag disables restore",
			args:            []string{"cmd", "-r=false"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     false,
			wantStoragePath: fileStoragePath,
		},
		{
			name:            "-f flag overrides file storage path",
			args:            []string{"cmd", "-f", "/tmp/metrics.json"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
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
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
			wantDSNSet:      true,
			wantDSN:         "",
		},
		{
			name:            "-d flag with value marks DSNSet true",
			args:            []string{"cmd", "-d", "postgres://user:pass@localhost/db"},
			wantHost:        host,
			wantPort:        port,
			wantStoreInt:    storeInterval,
			wantRestore:     restore,
			wantStoragePath: fileStoragePath,
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
