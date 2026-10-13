// Package trustedsubnet decides whether a client IP reported by an agent
// belongs to the server's trusted subnet. Both the HTTP and the gRPC
// transports use it so they apply exactly the same rule.
package trustedsubnet

import (
	"errors"
	"fmt"
	"net/netip"
)

// Reasons Check rejects an IP. Callers can tell them apart with errors.Is.
var (
	ErrNoIP          = errors.New("client IP not provided")
	ErrInvalidIP     = errors.New("client IP is not a valid address")
	ErrOutsideSubnet = errors.New("client IP is outside the trusted subnet")
)

// Check reports whether rawIP is allowed by subnet. It returns nil when it
// is, or when subnet is the zero Prefix (IsValid() == false), which disables
// the check. Otherwise it returns an error wrapping ErrNoIP, ErrInvalidIP or
// ErrOutsideSubnet. IPv4-mapped IPv6 addresses are unmapped before the
// subnet test, so "::ffff:192.168.0.5" matches 192.168.0.0/24.
func Check(subnet netip.Prefix, rawIP string) error {
	if !subnet.IsValid() {
		return nil
	}
	if rawIP == "" {
		return ErrNoIP
	}
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidIP, err)
	}
	if !subnet.Contains(ip.Unmap()) {
		return ErrOutsideSubnet
	}
	return nil
}
