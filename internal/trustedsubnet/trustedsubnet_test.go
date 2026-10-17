package trustedsubnet

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheck(t *testing.T) {
	trusted := netip.MustParsePrefix("192.168.0.0/24")

	tests := []struct {
		name    string
		subnet  netip.Prefix
		rawIP   string
		wantErr error
	}{
		{name: "zero subnet rejects every IP", subnet: netip.Prefix{}, rawIP: "192.168.0.5", wantErr: ErrOutsideSubnet},
		{name: "IPv4 inside subnet", subnet: trusted, rawIP: "192.168.0.5"},
		{name: "IPv4-mapped IPv6 inside subnet", subnet: trusted, rawIP: "::ffff:192.168.0.5"},
		{name: "empty IP", subnet: trusted, rawIP: "", wantErr: ErrNoIP},
		{name: "unparsable IP", subnet: trusted, rawIP: "garbage", wantErr: ErrInvalidIP},
		{name: "IPv4 outside subnet", subnet: trusted, rawIP: "192.168.1.1", wantErr: ErrOutsideSubnet},
		{name: "IPv6 outside IPv4 subnet", subnet: trusted, rawIP: "2001:db8::1", wantErr: ErrOutsideSubnet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(tt.subnet, tt.rawIP)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
