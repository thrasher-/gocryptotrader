package binance

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinanceSigning(t *testing.T) {
	payload := []byte("apiKey=test-key&symbol=BTCUSDT&timestamp=1649729878532")
	signature, err := signPayload(payload, "test-secret")
	require.NoError(t, err, "HMAC payload must sign")
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write(payload)
	assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), signature, "HMAC should match the transmitted payload")
	privateKey, encoded := testEd25519Key(t)
	signature, err = signPayload(payload, encoded)
	require.NoError(t, err, "Ed25519 payload must sign")
	decoded, err := base64.StdEncoding.DecodeString(signature)
	require.NoError(t, err, "Ed25519 signature must decode")
	assert.True(t, ed25519.Verify(ed25519.PublicKey(privateKey[ed25519.SeedSize:]), payload, decoded), "Ed25519 signature should verify")
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "RSA test key must generate")
	der, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err, "RSA test key must marshal")
	signature, err = signPayload(payload, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	require.NoError(t, err, "RSA payload must sign")
	decoded, err = base64.StdEncoding.DecodeString(signature)
	require.NoError(t, err, "RSA signature must decode")
	digest := sha256.Sum256(payload)
	assert.NoError(t, rsa.VerifyPKCS1v15(&rsaKey.PublicKey, crypto.SHA256, digest[:], decoded), "RSA signature should verify")
	for _, invalid := range []string{"-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----", encoded + "trailing bytes"} {
		_, err = signPayload(payload, invalid)
		assert.ErrorIs(t, err, errInvalidPrivateKey, "invalid private key should retain its sentinel")
		assert.NotContains(t, err.Error(), invalid, "key errors should not disclose the supplied key")
	}
	unsupported, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "unsupported test key must generate")
	der, err = x509.MarshalPKCS8PrivateKey(unsupported)
	require.NoError(t, err, "unsupported key must marshal")
	_, err = signPayload(payload, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	assert.ErrorIs(t, err, errInvalidPrivateKey, "unsupported key algorithm should retain its sentinel")
}
