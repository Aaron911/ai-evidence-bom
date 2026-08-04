package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"
)

type Envelope struct {
	Version              string    `json:"version"`
	Algorithm            string    `json:"algorithm"`
	CreatedAt            time.Time `json:"createdAt"`
	PayloadDigest        string    `json:"payloadDigest"`
	PublicKeyFingerprint string    `json:"publicKeyFingerprint"`
	Signature            string    `json:"signature"`
}

func GenerateKeyPair() (privatePEM, publicPEM []byte, err error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), nil
}

func Sign(payload, privatePEM []byte, createdAt time.Time) (Envelope, error) {
	privateKey, err := parsePrivateKey(privatePEM)
	if err != nil {
		return Envelope{}, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	digest := sha256.Sum256(payload)
	publicDigest := sha256.Sum256(publicKey)
	return Envelope{
		Version:              "0.1.0",
		Algorithm:            "ed25519",
		CreatedAt:            createdAt.UTC(),
		PayloadDigest:        "sha256:" + hex.EncodeToString(digest[:]),
		PublicKeyFingerprint: "sha256:" + hex.EncodeToString(publicDigest[:]),
		Signature:            base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}, nil
}

func Verify(payload []byte, envelope Envelope, publicPEM []byte) error {
	if envelope.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported signature algorithm %q", envelope.Algorithm)
	}
	publicKey, err := parsePublicKey(publicPEM)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if envelope.PayloadDigest != "sha256:"+hex.EncodeToString(digest[:]) {
		return fmt.Errorf("payload digest mismatch")
	}
	publicDigest := sha256.Sum256(publicKey)
	if envelope.PublicKeyFingerprint != "sha256:"+hex.EncodeToString(publicDigest[:]) {
		return fmt.Errorf("public key fingerprint mismatch")
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func parsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode private key PEM")
	}
	value, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	privateKey, ok := value.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ed25519")
	}
	return privateKey, nil
}

func parsePublicKey(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode public key PEM")
	}
	value, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	publicKey, ok := value.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ed25519")
	}
	return publicKey, nil
}
