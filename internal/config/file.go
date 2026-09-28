package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	apperrors "github.com/AVZotov/metrics/internal/errors"
)

// configPathEnvVar is the environment variable that points at a JSON
// config file. It's checked before flag.Parse runs, since the file's
// values are merged in as new defaults before flags are registered.
const configPathEnvVar = "CONFIG"

// discoverConfigPath finds the JSON config file path from args (normally
// os.Args[1:]) before flag.Parse is called. The CONFIG env var wins over a
// -c/-config/--c/--config flag, matching this project's convention that
// env beats flags. Returns "" if no config file is set.
func discoverConfigPath(args []string) string {
	if v, ok := os.LookupEnv(configPathEnvVar); ok && v != "" {
		return v
	}
	return scanArgsForConfigPath(args)
}

// scanArgsForConfigPath manually scans args for -c/-config/--c/--config,
// including the "-flag=value" form, without registering or parsing any
// flag set. It's needed because the config file must be read before
// flag.Parse runs (its values become new flag defaults).
func scanArgsForConfigPath(args []string) string {
	for i, arg := range args {
		for _, name := range [...]string{"config", "c"} {
			for _, dash := range [...]string{"--", "-"} {
				flagName := dash + name
				if val, ok := strings.CutPrefix(arg, flagName+"="); ok {
					return val
				}
				if arg == flagName && i+1 < len(args) {
					return args[i+1]
				}
			}
		}
	}
	return ""
}

// readConfigFile reads the JSON config file at path and decodes it into
// out, rejecting unknown fields so a typo'd key fails fast instead of
// being silently ignored. Returns apperrors.ErrConfigFileUnavailable if
// path can't be read, or apperrors.ErrConfigFileMalformed if it isn't
// valid JSON for out's shape.
func readConfigFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", apperrors.ErrConfigFileUnavailable, path, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%w: %s: %v", apperrors.ErrConfigFileMalformed, path, err)
	}
	return nil
}

// parseDurationSeconds parses s (e.g. "1s") as a whole, non-negative
// number of seconds. Returns apperrors.ErrInvalidDuration if s doesn't
// parse as a duration or doesn't represent a whole number of seconds.
func parseDurationSeconds(s string) (int, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %v", apperrors.ErrInvalidDuration, s, err)
	}
	if d < 0 || d%time.Second != 0 {
		return 0, fmt.Errorf("%w: %q must be a non-negative whole number of seconds", apperrors.ErrInvalidDuration, s)
	}
	return int(d / time.Second), nil
}
