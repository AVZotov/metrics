package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign returns the hex-encoded HMAC-SHA256 signature of data using key.
func Sign(data []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is the correct hex-encoded HMAC-SHA256
// signature of data under key.
func Verify(data []byte, key string, signature string) bool {
	expectedMAC := Sign(data, key)

	expectedBytes, err := hex.DecodeString(expectedMAC)
	if err != nil {
		return false
	}

	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	return hmac.Equal(signatureBytes, expectedBytes)
}
