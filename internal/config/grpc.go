package config

import (
	"fmt"
	"os"

	apperrors "github.com/AVZotov/metrics/internal/errors"
)

// grpcTLSFile is a TLS file gRPC needs, named the way the user sets it.
type grpcTLSFile struct {
	name string
	path string
}

// validateGRPCTLS checks that every file in files is set, exists and is
// not a directory. It doesn't parse the files: a malformed certificate or
// key fails later, when the server or agent loads it at startup. gRPC has
// no plaintext mode, so with grpcAddress set a missing file is a
// configuration error. With gRPC disabled the files aren't needed and
// aren't checked.
func validateGRPCTLS(grpcAddress string, files ...grpcTLSFile) error {
	if grpcAddress == "" {
		return nil
	}
	for _, f := range files {
		if f.path == "" {
			return fmt.Errorf("%w: %s", apperrors.ErrGRPCTLSNotConfigured, f.name)
		}
		info, err := os.Stat(f.path)
		if err != nil {
			return fmt.Errorf("%w: %s %q: %w", apperrors.ErrGRPCTLSFileUnavailable, f.name, f.path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%w: %s %q is a directory", apperrors.ErrGRPCTLSFileUnavailable, f.name, f.path)
		}
	}
	return nil
}
