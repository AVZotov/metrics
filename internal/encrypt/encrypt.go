// Package encrypt implements hybrid RSA+AES encryption between agent and
// server. Only RSA is supported (PEM, PKCS#1 or PKCS#8/PKIX) for the
// asymmetric half; the AES key is passed via the X-Crypto-Key header.
package encrypt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	
	appErr "github.com/AVZotov/metrics/internal/errors"
)

func loadPrivateKey(path string) (key *rsa.PrivateKey, err error) {
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

func loadPublicKey(path string) (key *rsa.PublicKey, err error) {
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
