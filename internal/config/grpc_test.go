package config

import (
	"os"
	"path/filepath"
	"testing"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grpcAddressCase is one source/precedence scenario for the gRPC address,
// shared by the server and agent config tests.
type grpcAddressCase struct {
	name string
	args func(t *testing.T) []string
	env  map[string]string
	want string
}

const (
	fileGRPCAddress = "filehost:3200"
	flagGRPCAddress = "flaghost:3201"
	envGRPCAddress  = "envhost:3202"
)

func grpcAddressCases() []grpcAddressCase {
	filePath := func(t *testing.T) string {
		t.Helper()
		return writeJSONConfig(t, `{"grpc_address": "`+fileGRPCAddress+`"}`)
	}
	return []grpcAddressCase{
		{
			name: "not set anywhere disables gRPC",
			args: func(*testing.T) []string { return nil },
			want: "",
		},
		{
			name: "file only",
			args: func(t *testing.T) []string { return []string{"-c", filePath(t)} },
			want: fileGRPCAddress,
		},
		{
			name: "flag only",
			args: func(*testing.T) []string { return []string{"-grpc-address", flagGRPCAddress} },
			want: flagGRPCAddress,
		},
		{
			name: "env only",
			args: func(*testing.T) []string { return nil },
			env:  map[string]string{"GRPC_ADDRESS": envGRPCAddress},
			want: envGRPCAddress,
		},
		{
			name: "flag overrides file",
			args: func(t *testing.T) []string {
				return []string{"-c", filePath(t), "-grpc-address", flagGRPCAddress}
			},
			want: flagGRPCAddress,
		},
		{
			name: "env overrides flag and file",
			args: func(t *testing.T) []string {
				return []string{"-c", filePath(t), "-grpc-address", flagGRPCAddress}
			},
			env:  map[string]string{"GRPC_ADDRESS": envGRPCAddress},
			want: envGRPCAddress,
		},
		{
			// caarlos0/env skips set-but-empty variables, so an empty
			// GRPC_ADDRESS can't clear a value from the flag or file.
			name: "set-but-empty env keeps flag value",
			args: func(t *testing.T) []string {
				return []string{"-c", filePath(t), "-grpc-address", flagGRPCAddress}
			},
			env:  map[string]string{"GRPC_ADDRESS": ""},
			want: flagGRPCAddress,
		},
		{
			name: "empty file value disables gRPC",
			args: func(t *testing.T) []string {
				return []string{"-c", writeJSONConfig(t, `{"grpc_address": ""}`)}
			},
			want: "",
		},
	}
}

// invalidGRPCAddresses aren't in host:port form.
var invalidGRPCAddresses = []string{"localhost", "a:b:c"}

func setArgsAndEnv(t *testing.T, args []string, env map[string]string) {
	t.Helper()
	resetFlags()
	origArgs := os.Args
	os.Args = append([]string{"cmd"}, args...)
	t.Cleanup(func() { os.Args = origArgs })
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestNewServerConfig_GRPCAddress(t *testing.T) {
	for _, tt := range grpcAddressCases() {
		t.Run(tt.name, func(t *testing.T) {
			setArgsAndEnv(t, tt.args(t), tt.env)
			setTLSEnv(t, serverTLSParams)
			cfg, err := NewServerConfig()
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.GRPCAddress)
		})
	}

	for _, bad := range invalidGRPCAddresses {
		t.Run("invalid value "+bad+" returns error", func(t *testing.T) {
			setArgsAndEnv(t, []string{"-grpc-address", bad}, nil)
			_, err := NewServerConfig()
			assert.ErrorContains(t, err, "grpc address")
		})
	}
}

func TestNewAgentConfig_GRPCAddress(t *testing.T) {
	for _, tt := range grpcAddressCases() {
		t.Run(tt.name, func(t *testing.T) {
			setArgsAndEnv(t, tt.args(t), tt.env)
			setTLSEnv(t, agentTLSParams)
			cfg, err := NewAgentConfig()
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.GRPCAddress)
		})
	}

	for _, bad := range invalidGRPCAddresses {
		t.Run("invalid value "+bad+" returns error", func(t *testing.T) {
			setArgsAndEnv(t, nil, map[string]string{"GRPC_ADDRESS": bad})
			_, err := NewAgentConfig()
			assert.ErrorContains(t, err, "grpc address")
		})
	}
}

// tlsParam is one gRPC TLS file setting under its flag, env and JSON names.
type tlsParam struct {
	flag string
	env  string
	json string
}

var (
	serverTLSParams = []tlsParam{
		{flag: "grpc-cert", env: "GRPC_CERT", json: "grpc_cert"},
		{flag: "grpc-key", env: "GRPC_KEY", json: "grpc_key"},
	}
	agentTLSParams = []tlsParam{
		{flag: "grpc-ca", env: "GRPC_CA", json: "grpc_ca"},
	}
)

// touchFile creates an empty file and returns its path. Config validation
// only checks that TLS files exist, so their contents don't matter here.
func touchFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	return path
}

// setTLSEnv points every param's env var at an existing file, so a test
// that enables gRPC for another reason passes TLS validation.
func setTLSEnv(t *testing.T, params []tlsParam) {
	t.Helper()
	for _, p := range params {
		t.Setenv(p.env, touchFile(t, p.json+".pem"))
	}
}

// grpcTLSSourceCase sets every TLS param from one or more sources and
// expects the values from the highest-precedence one (file < flag < env).
type grpcTLSSourceCase struct {
	name string
	// args and env enable gRPC and set the TLS params.
	args []string
	env  map[string]string
	// want maps each param's JSON name to the expected path.
	want map[string]string
	// wantErr is the expected validation error, for the error cases, and
	// wantParam the env var name the error must mention.
	wantErr   error
	wantParam string
}

func grpcTLSSourceCases(t *testing.T, params []tlsParam) []grpcTLSSourceCase {
	t.Helper()
	fromFile, fromFlag, fromEnv := map[string]string{}, map[string]string{}, map[string]string{}
	fileJSON := `{"grpc_address": "` + fileGRPCAddress + `"`
	flagArgs := []string{"-grpc-address", flagGRPCAddress}
	envVars := map[string]string{"GRPC_ADDRESS": envGRPCAddress}
	for _, p := range params {
		fromFile[p.json] = touchFile(t, "file-"+p.json+".pem")
		fromFlag[p.json] = touchFile(t, "flag-"+p.json+".pem")
		fromEnv[p.json] = touchFile(t, "env-"+p.json+".pem")
		fileJSON += `, "` + p.json + `": "` + fromFile[p.json] + `"`
		flagArgs = append(flagArgs, "-"+p.flag, fromFlag[p.json])
		envVars[p.env] = fromEnv[p.json]
	}
	configPath := writeJSONConfig(t, fileJSON+"}")
	withFile := append([]string{"-c", configPath}, flagArgs...)

	return []grpcTLSSourceCase{
		{name: "file only", args: []string{"-c", configPath}, want: fromFile},
		{name: "flag only", args: flagArgs, want: fromFlag},
		{name: "env only", env: envVars, want: fromEnv},
		{name: "flag overrides file", args: withFile, want: fromFlag},
		{name: "env overrides flag and file", args: withFile, env: envVars, want: fromEnv},
	}
}

// grpcTLSErrorCases enable gRPC with one TLS param missing or unusable.
func grpcTLSErrorCases(t *testing.T, params []tlsParam) []grpcTLSSourceCase {
	t.Helper()
	var cases []grpcTLSSourceCase
	for i, broken := range params {
		valid := map[string]string{"GRPC_ADDRESS": envGRPCAddress}
		for j, p := range params {
			if j != i {
				valid[p.env] = touchFile(t, p.json+".pem")
			}
		}
		withValue := func(v string) map[string]string {
			env := map[string]string{broken.env: v}
			for k, val := range valid {
				env[k] = val
			}
			return env
		}
		cases = append(cases,
			grpcTLSSourceCase{
				name:      broken.env + " not set",
				env:       valid,
				wantErr:   apperrors.ErrGRPCTLSNotConfigured,
				wantParam: broken.env,
			},
			grpcTLSSourceCase{
				name:      broken.env + " file missing",
				env:       withValue(filepath.Join(t.TempDir(), "absent.pem")),
				wantErr:   apperrors.ErrGRPCTLSFileUnavailable,
				wantParam: broken.env,
			},
			grpcTLSSourceCase{
				name:      broken.env + " is a directory",
				env:       withValue(t.TempDir()),
				wantErr:   apperrors.ErrGRPCTLSFileUnavailable,
				wantParam: broken.env,
			},
		)
	}
	return cases
}

func TestNewServerConfig_GRPCTLS(t *testing.T) {
	for _, tt := range grpcTLSSourceCases(t, serverTLSParams) {
		t.Run(tt.name, func(t *testing.T) {
			setArgsAndEnv(t, tt.args, tt.env)
			cfg, err := NewServerConfig()
			require.NoError(t, err)
			assert.Equal(t, tt.want["grpc_cert"], cfg.GRPCCert)
			assert.Equal(t, tt.want["grpc_key"], cfg.GRPCKey)
		})
	}

	for _, tt := range grpcTLSErrorCases(t, serverTLSParams) {
		t.Run("gRPC enabled, "+tt.name+" returns error", func(t *testing.T) {
			setArgsAndEnv(t, tt.args, tt.env)
			_, err := NewServerConfig()
			assert.ErrorIs(t, err, tt.wantErr)
			assert.ErrorContains(t, err, tt.wantParam)
		})
	}

	t.Run("gRPC disabled, TLS files are not required or checked", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "absent.pem")
		setArgsAndEnv(t, []string{"-grpc-cert", absent}, nil)
		cfg, err := NewServerConfig()
		require.NoError(t, err)
		assert.Equal(t, absent, cfg.GRPCCert)
		assert.Empty(t, cfg.GRPCKey)
	})
}

func TestNewAgentConfig_GRPCTLS(t *testing.T) {
	for _, tt := range grpcTLSSourceCases(t, agentTLSParams) {
		t.Run(tt.name, func(t *testing.T) {
			setArgsAndEnv(t, tt.args, tt.env)
			cfg, err := NewAgentConfig()
			require.NoError(t, err)
			assert.Equal(t, tt.want["grpc_ca"], cfg.GRPCCA)
		})
	}

	for _, tt := range grpcTLSErrorCases(t, agentTLSParams) {
		t.Run("gRPC enabled, "+tt.name+" returns error", func(t *testing.T) {
			setArgsAndEnv(t, tt.args, tt.env)
			_, err := NewAgentConfig()
			assert.ErrorIs(t, err, tt.wantErr)
			assert.ErrorContains(t, err, tt.wantParam)
		})
	}

	t.Run("gRPC disabled, TLS files are not required or checked", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "absent.pem")
		setArgsAndEnv(t, []string{"-grpc-ca", absent}, nil)
		cfg, err := NewAgentConfig()
		require.NoError(t, err)
		assert.Equal(t, absent, cfg.GRPCCA)
	})
}
