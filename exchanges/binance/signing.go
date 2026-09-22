package binance

import (
	"crypto"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"strings"
)

var (
	errInvalidPrivateKey  = errors.New("invalid Binance signing private key")
	errEd25519KeyRequired = errors.New("session.logon requires an Ed25519 private key")
)

// Binance API secrets may contain an HMAC secret or an unencrypted PKCS#8
// private key. Asymmetric signatures use base64, whereas HMAC uses hex.
func signPayload(payload []byte, secret string) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(secret), "-----BEGIN") {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(payload)
		return hex.EncodeToString(mac.Sum(nil)), nil
	}
	key, err := parsePrivateKey(secret)
	if err != nil {
		return "", err
	}
	switch key := key.(type) {
	case ed25519.PrivateKey:
		return base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload)), nil
	case *rsa.PrivateKey:
		digest := sha256.Sum256(payload)
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(signature), nil
	default:
		return "", errInvalidPrivateKey
	}
}

func parsePrivateKey(secret string) (any, error) {
	block, rest := pem.Decode([]byte(secret))
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errInvalidPrivateKey
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errInvalidPrivateKey
	}
	return key, nil
}
