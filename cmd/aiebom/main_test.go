package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Aaron911/ai-evidence-bom/internal/signing"
)

func TestIsLoopbackListen(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"127.0.0.1:4318", true},
		{"[::1]:4318", true},
		{"localhost:4318", true},
		{":4318", false},
		{"0.0.0.0:4318", false},
		{"invalid", false},
	}
	for _, test := range tests {
		if got := isLoopbackListen(test.address); got != test.want {
			t.Errorf("isLoopbackListen(%q)=%v want=%v", test.address, got, test.want)
		}
	}
}

func TestCanonicalEvidenceSignAndVerifyCommands(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "evidence.json")
	reformattedPath := filepath.Join(directory, "evidence-reformatted.json")
	privatePath := filepath.Join(directory, "private.pem")
	publicPath := filepath.Join(directory, "public.pem")
	signaturePath := filepath.Join(directory, "evidence.sig.json")

	input := []byte(`{"schemaVersion":"0.7.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[],"edges":[]}`)
	reformatted := []byte("{\n  \"edges\": [],\n  \"nodes\": [],\n  \"generatedAt\": \"2026-08-07T00:00:00Z\",\n  \"schemaVersion\": \"0.7.0\"\n}\n")
	privatePEM, publicPEM, err := signing.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		inputPath:       input,
		reformattedPath: reformatted,
		privatePath:     privatePEM,
		publicPath:      publicPEM,
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := runSign([]string{
		"--input", inputPath,
		"--private-key", privatePath,
		"--output", signaturePath,
		"--canonical-evidence",
	}); err != nil {
		t.Fatal(err)
	}
	if err := runVerify([]string{
		"--input", reformattedPath,
		"--public-key", publicPath,
		"--signature", signaturePath,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(signaturePath)
	if err != nil {
		t.Fatal(err)
	}
	var envelope signing.Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.PayloadType != signing.CanonicalPayloadType || envelope.Canonicalization != signing.CanonicalizationEvidenceV1 {
		t.Fatalf("canonical signature mode was not recorded: %+v", envelope)
	}
}
