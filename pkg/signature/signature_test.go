package signature

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestSignAndVerifyBase64(t *testing.T) {
	seed := []byte("01234567890123456789012345678901")
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	privateKeyB64 := base64.StdEncoding.EncodeToString(privateKey)
	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKey)

	body := []byte(`{"side":"BUY","amount":"1.5"}`)
	timestamp := "1710000000000"

	signatureValue, err := SignBase64(privateKeyB64, "POST", "/api/v1/orders", "symbol=BTCUSDT", body, timestamp)
	if err != nil {
		t.Fatalf("SignBase64() error = %v", err)
	}

	if err := VerifyBase64(publicKeyB64, signatureValue, "POST", "/api/v1/orders", "symbol=BTCUSDT", body, timestamp); err != nil {
		t.Fatalf("VerifyBase64() error = %v", err)
	}
}
