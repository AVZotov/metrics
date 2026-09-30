// Package encrypt implements hybrid RSA+AES encryption between agent and
// server. Only RSA is supported (PEM, PKCS#1 or PKCS#8/PKIX) for the
// asymmetric half; the AES key is passed via the X-Crypto-Key header.
package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	appErr "github.com/AVZotov/metrics/internal/errors"
)

// LoadPrivateKey reads an RSA private key from a PEM file at path. The
// parser is chosen by the PEM block type: "PRIVATE KEY" is PKCS#8, "RSA
// PRIVATE KEY" is PKCS#1. Returns appErr.ErrInvalidPEMBlock if the file
// isn't a valid PEM block, appErr.ErrUnexpectedKeyType if the block type
// is neither of those or the PKCS#8 key isn't RSA, and the parser's own
// error if the key bytes are malformed.
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(data)
	if pemBlock == nil {
		return nil, appErr.ErrInvalidPEMBlock
	}

	switch pemBlock.Type {
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
		}
		key, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: PKCS#8 key is %T, not RSA", appErr.ErrUnexpectedKeyType, k)
		}
		return key, nil
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#1 private key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("%w: %q is not a private key block", appErr.ErrUnexpectedKeyType, pemBlock.Type)
	}
}

// LoadPublicKey reads an RSA public key from a PEM file at path. The
// parser is chosen by the PEM block type: "PUBLIC KEY" is PKIX, "RSA
// PUBLIC KEY" is PKCS#1. Returns appErr.ErrInvalidPEMBlock if the file
// isn't a valid PEM block, appErr.ErrUnexpectedKeyType if the block type
// is neither of those or the PKIX key isn't RSA, and the parser's own
// error if the key bytes are malformed.
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(data)
	if pemBlock == nil {
		return nil, appErr.ErrInvalidPEMBlock
	}

	switch pemBlock.Type {
	case "PUBLIC KEY":
		k, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKIX public key: %w", err)
		}
		key, ok := k.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: PKIX key is %T, not RSA", appErr.ErrUnexpectedKeyType, k)
		}
		return key, nil
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(pemBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#1 public key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("%w: %q is not a public key block", appErr.ErrUnexpectedKeyType, pemBlock.Type)
	}
}

// EncryptHybrid encrypts data with generated AES key, then encrypts that
// key with pub key. Returns the AES-GCM-encrypted data and the RSA-OAEP-
// encrypted key (base64-encoded, ready for an HTTP header).
func EncryptHybrid(pub *rsa.PublicKey, data []byte) (encryptedData []byte, encryptedKeyB64 string, err error) {
	aesKey, err := generateAESKey()
	if err != nil {
		return nil, "", err
	}

	encryptedData, err = sealPayload(aesKey, data)
	if err != nil {
		return nil, "", err
	}

	encryptedKey, err := wrapAESKey(pub, aesKey)
	if err != nil {
		return nil, "", err
	}

	return encryptedData, base64.StdEncoding.EncodeToString(encryptedKey), nil
}

func generateAESKey() ([]byte, error) {
	const size = 32
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// sealPayload encrypts plaintext with AES-GCM under key, returning the
// random nonce followed by the sealed ciphertext.
func sealPayload(key, plaintext []byte) ([]byte, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcmBlock, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmBlock.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}
	sealed := gcmBlock.Seal(nonce, nonce, plaintext, nil)

	return sealed, nil
}

// wrapAESKey encrypts aesKey with pub using RSA-OAEP (SHA-256).
func wrapAESKey(pub *rsa.PublicKey, aesKey []byte) ([]byte, error) {
	hash := sha256.New()
	return rsa.EncryptOAEP(hash, rand.Reader, pub, aesKey, nil)
}

// DecryptHybrid reverses EncryptHybrid: decrypts encryptedKey with private
// to recover the AES key, then decrypts encrypted data with it.
func DecryptHybrid(private *rsa.PrivateKey, encryptedKey, encryptedData []byte) ([]byte, error) {
	key, err := unwrapAESKey(private, encryptedKey)
	if err != nil {
		return nil, err
	}

	return openPayload(key, encryptedData)
}

// unwrapAESKey reverses wrapAESKey, recovering the AES key with private.
func unwrapAESKey(private *rsa.PrivateKey, encryptedKey []byte) ([]byte, error) {
	hash := sha256.New()
	return rsa.DecryptOAEP(hash, nil, private, encryptedKey, nil)
}

// openPayload reverses sealPayload, splitting off the nonce and
// decrypting and authenticating the rest with AES-GCM under key.
func openPayload(key []byte, encryptedData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, appErr.ErrUnexpectedCipherLength
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
