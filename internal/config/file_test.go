package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanArgsForConfigPath(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no args", args: nil, want: ""},
		{name: "no config flag", args: []string{"-a", "localhost:8080"}, want: ""},
		{name: "-c space form", args: []string{"-c", "cfg.json"}, want: "cfg.json"},
		{name: "--c space form", args: []string{"--c", "cfg.json"}, want: "cfg.json"},
		{name: "-config space form", args: []string{"-config", "cfg.json"}, want: "cfg.json"},
		{name: "--config space form", args: []string{"--config", "cfg.json"}, want: "cfg.json"},
		{name: "-c= form", args: []string{"-c=cfg.json"}, want: "cfg.json"},
		{name: "--c= form", args: []string{"--c=cfg.json"}, want: "cfg.json"},
		{name: "-config= form", args: []string{"-config=cfg.json"}, want: "cfg.json"},
		{name: "--config= form", args: []string{"--config=cfg.json"}, want: "cfg.json"},
		{name: "flag among others", args: []string{"-a", "localhost:8080", "-c", "cfg.json", "-r"}, want: "cfg.json"},
		{name: "trailing flag with no value", args: []string{"-c"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scanArgsForConfigPath(tt.args))
		})
	}
}

func TestDiscoverConfigPath(t *testing.T) {
	t.Run("env wins over flag", func(t *testing.T) {
		t.Setenv(configPathEnvVar, "from-env.json")
		assert.Equal(t, "from-env.json", discoverConfigPath([]string{"-c", "from-flag.json"}))
	})

	t.Run("falls back to flag scan when env unset", func(t *testing.T) {
		assert.Equal(t, "from-flag.json", discoverConfigPath([]string{"-c", "from-flag.json"}))
	})

	t.Run("empty when neither set", func(t *testing.T) {
		assert.Equal(t, "", discoverConfigPath(nil))
	})
}

func TestReadConfigFile(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		var out serverFileConfig
		err := readConfigFile(filepath.Join(t.TempDir(), "missing.json"), &out)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConfigFileUnavailable)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"address": `), 0o600))

		var out serverFileConfig
		err := readConfigFile(path, &out)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConfigFileMalformed)
	})

	t.Run("unknown key", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"not_a_real_key": "x"}`), 0o600))

		var out serverFileConfig
		err := readConfigFile(path, &out)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConfigFileMalformed)
	})

	t.Run("valid JSON decodes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cfg.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"address": "localhost:9090"}`), 0o600))

		var out serverFileConfig
		require.NoError(t, readConfigFile(path, &out))
		require.NotNil(t, out.Address)
		assert.Equal(t, "localhost:9090", *out.Address)
	})
}

func TestParseDurationSeconds(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "whole seconds", input: "1s", want: 1},
		{name: "multiple seconds", input: "5s", want: 5},
		{name: "minutes convert to seconds", input: "2m", want: 120},
		{name: "sub-second duration errors", input: "500ms", wantErr: true},
		{name: "unparsable duration errors", input: "not-a-duration", wantErr: true},
		{name: "negative duration errors", input: "-1s", wantErr: true},
		{name: "zero is a whole number of seconds", input: "0s", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDurationSeconds(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, apperrors.ErrInvalidDuration)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, time.Duration(tt.want)*time.Second, time.Duration(got)*time.Second)
		})
	}
}
