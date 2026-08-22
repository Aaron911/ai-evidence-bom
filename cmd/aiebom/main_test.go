package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
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
	if envelope.PayloadType != signing.CanonicalPayloadType || envelope.Canonicalization != signing.CanonicalizationEvidenceV2 {
		t.Fatalf("canonical signature mode was not recorded: %+v", envelope)
	}
}

func TestScanRequiresExactSourceAuthorizationForVerifiedEvidence(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "observations.json")
	policyPath := filepath.Join(directory, "trust.json")
	graphPath := filepath.Join(directory, "evidence.json")
	input := []byte(`{
	  "source":"fixture",
	  "observations":[
	    {
	      "timestamp":"2026-08-22T08:00:00Z",
	      "level":"verified",
	      "source":"trusted-verifier",
	      "attributes":{
	        "gen_ai.provider.name":"provider",
	        "gen_ai.request.model":"stable-model",
	        "gen_ai.response.model":"weights-v1"
	      }
	    },
	    {
	      "timestamp":"2026-08-22T09:00:00Z",
	      "level":"verified",
	      "source":"malicious-adapter",
	      "attributes":{
	        "gen_ai.provider.name":"provider",
	        "gen_ai.request.model":"stable-model",
	        "gen_ai.response.model":"weights-v2",
	        "gen_ai.tool.call.arguments":"PRIVATE_SOURCE_TRUST_ARGUMENT_MUST_NOT_LEAK"
	      }
	    }
	  ]
	}`)
	policy := []byte(`{
	  "version":"0.1.0",
	  "sources":[{"source":"trusted-verifier","maxEvidence":"verified"}]
	}`)
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, policy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runScan([]string{
		"--input", inputPath,
		"--graph-out", graphPath,
		"--source-trust-policy", policyPath,
	}); err != nil {
		t.Fatal(err)
	}
	var graph model.Graph
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE_SOURCE_TRUST_ARGUMENT_MUST_NOT_LEAK") {
		t.Fatal("sensitive compact attribute reached the evidence graph")
	}
	if len(graph.Nodes) != 1 || graph.Nodes[0].Version != "weights-v1" {
		t.Fatalf("trusted version was not selected: %+v", graph.Nodes)
	}
	levels := map[string]model.EvidenceLevel{}
	for _, candidate := range graph.Nodes[0].FieldEvidence[0].Values {
		levels[candidate.Value] = candidate.Evidence.Level
	}
	if levels["weights-v1"] != model.EvidenceVerified || levels["weights-v2"] != model.EvidenceObserved {
		t.Fatalf("unexpected field evidence: %+v", graph.Nodes[0].FieldEvidence)
	}
}

func TestScanCapsVerifiedEvidenceWithoutPolicyFile(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "observations.json")
	graphPath := filepath.Join(directory, "evidence.json")
	input := []byte(`{
	  "source":"malicious-adapter",
	  "observations":[{
	    "level":"verified",
	    "attributes":{"gen_ai.request.model":"model-a"}
	  }]
	}`)
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runScan([]string{"--input", inputPath, "--graph-out", graphPath}); err != nil {
		t.Fatal(err)
	}
	var graph model.Graph
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 1 || graph.Nodes[0].Evidence.Level != model.EvidenceObserved {
		t.Fatalf("untrusted verified evidence was not capped: %+v", graph.Nodes)
	}
}
