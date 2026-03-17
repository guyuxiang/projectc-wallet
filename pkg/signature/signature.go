package signature

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

func BodySHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func BuildSigningString(method string, requestPath string, queryString string, body []byte, timestamp string) string {
	return strings.Join([]string{
		strings.ToUpper(method),
		requestPath,
		queryString,
		BodySHA256Hex(body),
		timestamp,
	}, "\n")
}

func SignBase64(privateKeyB64 string, method string, requestPath string, queryString string, body []byte, timestamp string) (string, error) {
	privateKey, err := parsePrivateKey(privateKeyB64)
	if err != nil {
		return "", err
	}
	signingString := BuildSigningString(method, requestPath, queryString, body, timestamp)
	signature := ed25519.Sign(privateKey, []byte(signingString))
	return base64.StdEncoding.EncodeToString(signature), nil
}

func VerifyBase64(publicKeyB64 string, signatureB64 string, method string, requestPath string, queryString string, body []byte, timestamp string) error {
	publicKey, err := parsePublicKey(publicKeyB64)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("invalid ed25519 signature length")
	}
	signingString := BuildSigningString(method, requestPath, queryString, body, timestamp)
	if !ed25519.Verify(publicKey, []byte(signingString), signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func parsePublicKey(publicKeyB64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("invalid ed25519 public key length")
	}
	return ed25519.PublicKey(raw), nil
}

func parsePrivateKey(privateKeyB64 string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	switch len(raw) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	default:
		return nil, errors.New("invalid ed25519 private key length")
	}
}
