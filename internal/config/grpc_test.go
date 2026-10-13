package config

import (
	"os"
	"testing"

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
