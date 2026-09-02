package sign

import (
	"bytes"
	"testing"
)

func BenchmarkSign(b *testing.B) {
	const key = "super secret key"
	tests := []struct {
		name string
		data []byte
		key  string
	}{
		{name: "nil", data: nil, key: key},
		{name: "100 bytes", data: bytes.Repeat([]byte("a"), 100), key: key},
		{name: "1024 bytes", data: bytes.Repeat([]byte("a"), 1024), key: key},
		{name: "10240 bytes", data: bytes.Repeat([]byte("a"), 10240), key: key},
	}
	for _, tt := range tests {
		b.Run(
			tt.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_ = Sign(tt.data, tt.key)
				}
			},
		)
	}
}
