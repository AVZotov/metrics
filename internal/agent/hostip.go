package agent

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// HostIP returns the local IP address the OS would use to reach serverAddr
// ("host:port", no scheme). It "connects" a UDP socket, which only selects
// a route and local address — no packets are sent, so the server doesn't
// need to be up. A hostname in serverAddr is still resolved via DNS.
// IPv4-mapped IPv6 addresses are unmapped to plain IPv4.
//
// IPv4 is tried first, falling back to any address family only if that
// fails (e.g. for an IPv6 literal). The server's trusted_subnet is usually
// an IPv4 range, while the resolver may return ::1 first for "localhost"
// (as on macOS), which would put the agent outside the subnet.
func HostIP(serverAddr string) (netip.Addr, error) {
	conn, err4 := net.Dial("udp4", serverAddr)
	if err4 != nil {
		var err error
		conn, err = net.Dial("udp", serverAddr)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("dial %s: %w", serverAddr, errors.Join(err4, err))
		}
	}
	defer func() { _ = conn.Close() }()

	addr := conn.LocalAddr()
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return netip.Addr{}, fmt.Errorf("unexpected local address type %T", addr)
	}
	addrPort := udpAddr.AddrPort()
	ip := addrPort.Addr()
	return ip.Unmap(), nil
}
