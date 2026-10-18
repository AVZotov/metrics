package agent

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostIP_Loopback(t *testing.T) {
	// Dialing UDP sends no packets, so nothing has to listen on the port.
	ip, err := HostIP("127.0.0.1:8080")
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("127.0.0.1"), ip)
}

func TestHostIP_LocalhostPrefersIPv4(t *testing.T) {
	// The resolver may return ::1 first for "localhost" (macOS does);
	// HostIP must still pick the IPv4 loopback.
	ip, err := HostIP("localhost:8080")
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("127.0.0.1"), ip)
}

func TestHostIP_IPv6LiteralFallsBackToUDP(t *testing.T) {
	// udp4 can't dial an IPv6 literal, so this covers the "udp" fallback.
	if conn, err := net.Dial("udp6", "[::1]:8080"); err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	} else {
		_ = conn.Close()
	}

	ip, err := HostIP("[::1]:8080")
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("::1"), ip)
}

func TestHostIP_InvalidAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1", "127.0.0.1:notaport", "127.0.0.1:99999"} {
		t.Run(addr, func(t *testing.T) {
			ip, err := HostIP(addr)
			require.Error(t, err)
			assert.False(t, ip.IsValid())
		})
	}
}
