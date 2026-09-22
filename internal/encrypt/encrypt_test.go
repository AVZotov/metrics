package encrypt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	appErr "github.com/AVZotov/metrics/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func writePEM(t *testing.T, blockType string, der []byte) string {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	path := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	return path
}

func TestLoadPrivateKey_PKCS8(t *testing.T) {
	key := generateTestRSAKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	path := writePEM(t, "PRIVATE KEY", der)

	got, err := LoadPrivateKey(path)
	require.NoError(t, err)
	assert.Equal(t, key.N, got.N)
}

func TestLoadPrivateKey_PKCS1(t *testing.T) {
	key := generateTestRSAKey(t)
	der := x509.MarshalPKCS1PrivateKey(key)
	path := writePEM(t, "RSA PRIVATE KEY", der)

	got, err := LoadPrivateKey(path)
	require.NoError(t, err)
	assert.Equal(t, key.N, got.N)
}

func TestLoadPrivateKey_InvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.pem")
	require.NoError(t, os.WriteFile(path, []byte("not a pem block"), 0o600))

	_, err := LoadPrivateKey(path)
	assert.ErrorIs(t, err, appErr.ErrInvalidPEMBlock)
}

func TestLoadPrivateKey_WrongKeyType(t *testing.T) {
	key := generateTestRSAKey(t)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	path := writePEM(t, "PUBLIC KEY", der)

	_, err = LoadPrivateKey(path)
	assert.ErrorIs(t, err, appErr.ErrUnexpectedKeyType)
}

func TestLoadPublicKey_PKIX(t *testing.T) {
	key := generateTestRSAKey(t)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	path := writePEM(t, "PUBLIC KEY", der)

	got, err := LoadPublicKey(path)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey.N, got.N)
}

func TestLoadPublicKey_PKCS1(t *testing.T) {
	key := generateTestRSAKey(t)
	der := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	path := writePEM(t, "RSA PUBLIC KEY", der)

	got, err := LoadPublicKey(path)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey.N, got.N)
}

func TestLoadPublicKey_InvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.pem")
	require.NoError(t, os.WriteFile(path, []byte("not a pem block"), 0o600))

	_, err := LoadPublicKey(path)
	assert.ErrorIs(t, err, appErr.ErrInvalidPEMBlock)
}

func TestLoadPublicKey_WrongKeyType(t *testing.T) {
	key := generateTestRSAKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	path := writePEM(t, "PRIVATE KEY", der)

	_, err = LoadPublicKey(path)
	assert.ErrorIs(t, err, appErr.ErrUnexpectedKeyType)
}

func TestEncryptHybrid_NoErrorAndValidOutput(t *testing.T) {
	key := generateTestRSAKey(t)
	plaintext := []byte("some metrics payload")

	encryptedData, encryptedKeyB64, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)

	assert.NotEmpty(t, encryptedKeyB64)
	_, err = base64.StdEncoding.DecodeString(encryptedKeyB64)
	assert.NoError(t, err, "encryptedKeyB64 should be valid base64")

	assert.NotEqual(t, plaintext, encryptedData, "encrypted data should differ from plaintext")
}

func TestEncryptHybrid_ProducesFreshKeyAndCiphertextEachCall(t *testing.T) {
	key := generateTestRSAKey(t)
	plaintext := []byte("some metrics payload")

	data1, keyB64_1, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)
	data2, keyB64_2, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)

	assert.NotEqual(t, data1, data2, "ciphertext should differ between calls (fresh AES key/nonce)")
	assert.NotEqual(t, keyB64_1, keyB64_2, "encrypted AES key should differ between calls (RSA-OAEP is randomized)")
}

func TestDecryptHybrid_RoundTrip(t *testing.T) {
	key := generateTestRSAKey(t)
	plaintext := []byte("some metrics payload")

	encryptedData, encryptedKeyB64, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)
	encryptedKey, err := base64.StdEncoding.DecodeString(encryptedKeyB64)
	require.NoError(t, err)

	got, err := DecryptHybrid(key, encryptedKey, encryptedData)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestDecryptHybrid_WrongPrivateKeyFails(t *testing.T) {
	key := generateTestRSAKey(t)
	wrongKey := generateTestRSAKey(t)
	plaintext := []byte("some metrics payload")

	encryptedData, encryptedKeyB64, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)
	encryptedKey, err := base64.StdEncoding.DecodeString(encryptedKeyB64)
	require.NoError(t, err)

	_, err = DecryptHybrid(wrongKey, encryptedKey, encryptedData)
	assert.Error(t, err)
}

func TestDecryptHybrid_TruncatedCiphertextReturnsUnexpectedCipherLength(t *testing.T) {
	key := generateTestRSAKey(t)
	plaintext := []byte("some metrics payload")

	_, encryptedKeyB64, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)
	encryptedKey, err := base64.StdEncoding.DecodeString(encryptedKeyB64)
	require.NoError(t, err)

	tooShort := []byte("short")
	_, err = DecryptHybrid(key, encryptedKey, tooShort)
	assert.ErrorIs(t, err, appErr.ErrUnexpectedCipherLength)
}

func TestDecryptHybrid_TamperedCiphertextFailsAuthentication(t *testing.T) {
	key := generateTestRSAKey(t)
	plaintext := []byte("some metrics payload")

	encryptedData, encryptedKeyB64, err := EncryptHybrid(&key.PublicKey, plaintext)
	require.NoError(t, err)
	encryptedKey, err := base64.StdEncoding.DecodeString(encryptedKeyB64)
	require.NoError(t, err)

	tampered := make([]byte, len(encryptedData))
	copy(tampered, encryptedData)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = DecryptHybrid(key, encryptedKey, tampered)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, appErr.ErrUnexpectedCipherLength)
}
