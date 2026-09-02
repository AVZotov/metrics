package config

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	e "github.com/AVZotov/metrics/internal/errors"
)

var _ flag.Value = (*Address)(nil)

// Address is a host:port pair. It implements flag.Value and
// encoding.TextUnmarshaler so it can be parsed straight from a flag or env var.
type Address struct {
	Host string
	Port int
}

// String returns the address as "host:port".
func (a *Address) String() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

// Set parses str in "host:port" form into a. Returns an error if str isn't
// in that form, or e.ErrInvalidValue if the port isn't a valid number.
func (a *Address) Set(str string) error {
	hp := strings.Split(str, ":")
	if len(hp) != 2 {
		return errors.New("need address in form host:port")
	}
	a.Host = hp[0]
	p, err := strconv.Atoi(hp[1])
	if err != nil {
		return e.ErrInvalidValue
	}
	a.Port = p
	return nil
}

// UnmarshalText parses data the same way Set does, so Address can be
// populated from env vars.
func (a *Address) UnmarshalText(data []byte) error {
	return a.Set(string(data))
}
