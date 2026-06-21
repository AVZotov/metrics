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

type Address struct {
	Host string
	Port int
}

func (a *Address) String() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

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

func (a *Address) UnmarshalText(data []byte) error {
	return a.Set(string(data))
}
