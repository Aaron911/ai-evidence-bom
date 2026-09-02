package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
	sarifpkg "github.com/Aaron911/ai-evidence-bom/internal/sarif"
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

func TestLoadSourceAuthenticationPolicy(t *testing.T) {
	token := "cli-source-credential-that-is-longer-than-32-bytes"
	digest := sha256.Sum256([]byte(token))
	directory := t.TempDir()
	policyPath := filepath.Join(directory, "source-auth.json")
	policyJSON := fmt.Sprintf(`{
	  "version":"0.1.0",
	  "bindings":[{"source":"cli-verifier","tokenSha256":"%x"}]
	}`, digest)
	if err := os.WriteFile(policyPath, []byte(policyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := loadSourceAuthenticationPolicy(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if source, ok := policy.Authenticate(token); !ok || source != "cli-verifier" {
		t.Fatalf("loaded policy source=%q ok=%v", source, ok)
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

func TestRunSARIFMapsScannerURIToDigestedArtifact(t *testing.T) {
	directory := t.TempDir()
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	if err := os.Mkdir("runtime", 0o755); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join("runtime", "main.go")
	if err := os.WriteFile(artifactPath, []byte("package runtime\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifactURI, digest, err := sarifpkg.ArtifactIdentity(artifactPath, sarifpkg.DefaultMaxArtifactBytes)
	if err != nil {
		t.Fatal(err)
	}
	graph := model.Graph{
		SchemaVersion: model.SchemaVersion,
		GeneratedAt:   time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		Source:        "test-runtime",
		Nodes: []model.Node{{
			ID:         model.StableNodeID("mcp_server", "", "test-server"),
			Type:       "mcp_server",
			Name:       "test-server",
			Digests:    map[string]string{"sha256": digest},
			Properties: map[string]string{sarifpkg.PropertyArtifactURI: artifactURI},
			Evidence:   model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1},
		}},
		Edges: []model.Edge{},
	}
	inputPath := filepath.Join(directory, "base.json")
	sarifPath := filepath.Join(directory, "findings.sarif")
	outputPath := filepath.Join(directory, "enriched.json")
	if err := writeJSON(inputPath, graph, 0o600); err != nil {
		t.Fatal(err)
	}
	sarifData := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","semanticVersion":"2.28.0","rules":[{"id":"G204","defaultConfiguration":{"level":"error"}}]}},"invocations":[{"executionSuccessful":true}],"results":[{"ruleId":"G204","locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.go"}}}]}]}]}`
	if err := os.WriteFile(sarifPath, []byte(sarifData), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runSARIF([]string{
		"--input", inputPath,
		"--sarif", sarifPath,
		"--artifact", artifactPath,
		"--sarif-artifact-uri", "main.go",
		"--output", outputPath,
	}); err != nil {
		t.Fatal(err)
	}
	var enriched model.Graph
	if err := readJSON(outputPath, &enriched); err != nil {
		t.Fatal(err)
	}
	if len(enriched.Nodes) != 2 || len(enriched.Edges) != 1 {
		t.Fatalf("unexpected enriched graph: %+v", enriched)
	}
	found := false
	for _, node := range enriched.Nodes {
		found = found || node.Type == "finding" && node.Name == "G204" && node.Provider == "gosec"
	}
	if !found {
		t.Fatalf("finding was not imported: %+v", enriched.Nodes)
	}
}
