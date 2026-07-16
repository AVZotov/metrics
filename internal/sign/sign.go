package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

func Sign(data []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(data []byte, key string, signature string) bool {
	expectedMAC := Sign(data, key)
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedMAC)) == 1
}
