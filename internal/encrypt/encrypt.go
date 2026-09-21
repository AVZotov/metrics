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
	"errors"
	"io"
	"os"

	appErr "github.com/AVZotov/metrics/internal/errors"
)

// LoadPrivateKey reads an RSA private key from a PEM file at path. It
// tries the modern PKCS#8 format first, falling back to legacy PKCS#1.
// Returns appErr.ErrInvalidPEMBlock if the file isn't a valid PEM block,
// or appErr.ErrUnexpectedKeyType if it decodes but isn't an RSA private
// key.
func LoadPrivateKey(path string) (key *rsa.PrivateKey, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(data)
	if pemBlock == nil {
		return nil, appErr.ErrInvalidPEMBlock
	}
	k, pkcs8Err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	if pkcs8Err == nil {
		pkcs8Key, ok := k.(*rsa.PrivateKey)
		if ok {
			return pkcs8Key, nil
		}
	}
	key, pkcs1Err := x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
	if pkcs1Err == nil {
		return key, nil
	}
	err = errors.Join(err, pkcs1Err)
	return nil, errors.Join(err, appErr.ErrUnexpectedKeyType)
}

// LoadPublicKey reads an RSA public key from a PEM file at path. It
// tries the modern PKIX format first, falling back to legacy PKCS#1.
// Returns appErr.ErrInvalidPEMBlock if the file isn't a valid PEM block,
// or appErr.ErrUnexpectedKeyType if it decodes but isn't an RSA public
// key.
func LoadPublicKey(path string) (key *rsa.PublicKey, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pemBlock, _ := pem.Decode(data)
	if pemBlock == nil {
		return nil, appErr.ErrInvalidPEMBlock
	}
	k, pkixErr := x509.ParsePKIXPublicKey(pemBlock.Bytes)
	if pkixErr == nil {
		pkixKey, ok := k.(*rsa.PublicKey)
		if ok {
			return pkixKey, nil
		}
	}
	key, pkcsErr := x509.ParsePKCS1PublicKey(pemBlock.Bytes)
	if pkcsErr == nil {
		return key, nil
	}
	err = errors.Join(err, pkcsErr)

	return nil, errors.Join(err, appErr.ErrUnexpectedKeyType)
}

// EncryptHybrid encrypts data with generated AES key, then encrypts that
// key with pub key. Returns the AES-GCM-encrypted data and the RSA-OAEP-
// encrypted key (base64-encoded, ready for an HTTP header).
func EncryptHybrid(pub *rsa.PublicKey, data []byte) (encryptedData []byte, encryptedKeyB64 string, err error) {
	aesKey, err := generateAESKey()
	if err != nil {
		return nil, "", err
	}

	encryptedData, err = encryptAESBody(aesKey, data)
	if err != nil {
		return nil, "", err
	}

	encryptedKey, err := encryptAESKey(pub, aesKey)
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

func encryptAESBody(key, plaintext []byte) ([]byte, error) {
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

func encryptAESKey(pub *rsa.PublicKey, aesKey []byte) ([]byte, error) {
	hash := sha256.New()
	return rsa.EncryptOAEP(hash, rand.Reader, pub, aesKey, nil)
}
