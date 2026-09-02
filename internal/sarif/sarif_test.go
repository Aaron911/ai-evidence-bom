package sarif

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

const (
	testArtifactURI = "scripts/live/mcp_runtime/main.go"
	testDigest      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestImportBindsSemgrepResultAndDropsContent(t *testing.T) {
	graph := targetGraph(testArtifactURI, testDigest)
	data := semgrepSARIF(testArtifactURI, `
      {
        "ruleId": "aiebom.mcp-shell-injection",
        "message": {"text": "PRIVATE_SARIF_MESSAGE_MUST_NOT_LEAK"},
        "locations": [{"physicalLocation": {
          "artifactLocation": {"uri": "`+testArtifactURI+`"},
          "region": {"startLine": 10, "snippet": {"text": "PRIVATE_SARIF_SOURCE_MUST_NOT_LEAK"}}
        }}]
      },
      {
        "ruleId": "aiebom.mcp-shell-injection",
        "message": {"text": "same rule, second occurrence"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "`+testArtifactURI+`"}}}]
      },
      {
        "ruleId": "aiebom.suppressed",
        "suppressions": [{"kind": "inSource"}],
        "message": {"text": "suppressed"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "`+testArtifactURI+`"}}}]
      },
      {
        "ruleId": "aiebom.other-artifact",
        "message": {"text": "other"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "other.go"}}}]
      }
`)

	observedAt := time.Date(2026, 9, 2, 1, 2, 3, 0, time.UTC)
	result, err := Import(graph, data, testArtifactURI, testArtifactURI, testDigest, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attached != 1 || result.SkippedResults != 2 {
		t.Fatalf("unexpected import summary: %+v", result)
	}
	finding := nodeByType(t, result.Graph, "finding")
	if finding.Name != "aiebom.mcp-shell-injection" || finding.Provider != "Semgrep OSS" {
		t.Fatalf("unexpected finding identity: %+v", finding)
	}
	if finding.Properties[PropertyFindingLevel] != "error" {
		t.Fatalf("SARIF rule default level was not resolved: %+v", finding.Properties)
	}
	if finding.Properties[PropertyScannerVersion] != "1.146.0" ||
		finding.Properties[PropertyFindingState] != "scanner-reported" ||
		finding.Properties[PropertyTargetURI] != testArtifactURI ||
		finding.Properties[PropertyTargetDigest] != testDigest {
		t.Fatalf("finding provenance is incomplete: %+v", finding.Properties)
	}
	if finding.Evidence.Level != model.EvidenceObserved || finding.Evidence.ObservationCount != 2 ||
		len(finding.Evidence.Sources) != 1 || finding.Evidence.Sources[0] != "sarif:Semgrep OSS@1.146.0" {
		t.Fatalf("unexpected finding evidence: %+v", finding.Evidence)
	}
	if !hasEdge(result.Graph, targetNodeID(), finding.ID, "affected_by") {
		t.Fatalf("component-to-finding edge missing: %+v", result.Graph.Edges)
	}
	encoded, err := json.Marshal(result.Graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"PRIVATE_SARIF_MESSAGE_MUST_NOT_LEAK", "PRIVATE_SARIF_SOURCE_MUST_NOT_LEAK", "same rule, second occurrence"} {
		if strings.Contains(string(encoded), marker) {
			t.Fatalf("SARIF content marker reached graph: %s", marker)
		}
	}
}

func TestImportRequiresArtifactURIAndDigest(t *testing.T) {
	data := semgrepSARIF(testArtifactURI, `
      {"ruleId":"aiebom.rule","message":{"text":"x"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"`+testArtifactURI+`"}}}]}
`)
	tests := []struct {
		name    string
		graph   model.Graph
		digest  string
		wantErr string
	}{
		{name: "digest mismatch", graph: targetGraph(testArtifactURI, strings.Repeat("a", 64)), digest: testDigest, wantErr: "SHA-256 did not"},
		{name: "display name only", graph: targetGraph("different.go", testDigest), digest: testDigest, wantErr: "no component matches"},
		{name: "missing digest", graph: targetGraph(testArtifactURI, ""), digest: testDigest, wantErr: "SHA-256 did not"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Import(test.graph, data, testArtifactURI, testArtifactURI, test.digest, time.Now()); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Import error=%v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestImportRejectsConflictedBinding(t *testing.T) {
	graph := targetGraph(testArtifactURI, testDigest)
	graph.Nodes[0].FieldEvidence = []model.FieldEvidence{{
		Field:         model.FieldDigest,
		Key:           "sha256",
		SelectedValue: testDigest,
		Conflict:      true,
		Values: []model.FieldValueEvidence{
			{Value: testDigest, Evidence: model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1}},
			{Value: strings.Repeat("f", 64), Evidence: model.EvidenceSummary{Level: model.EvidenceDeclared, ObservationCount: 1}},
		},
	}}
	data := semgrepSARIF(testArtifactURI, `
      {"ruleId":"aiebom.rule","message":{"text":"x"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"`+testArtifactURI+`"}}}]}
`)
	if _, err := Import(graph, data, testArtifactURI, testArtifactURI, testDigest, time.Now()); err == nil || !strings.Contains(err.Error(), "SHA-256 did not") {
		t.Fatalf("conflicted binding was accepted: %v", err)
	}
}

func TestImportRejectsInconsistentSelectedBindingEvidence(t *testing.T) {
	graph := targetGraph(testArtifactURI, testDigest)
	graph.Nodes[0].FieldEvidence = []model.FieldEvidence{{
		Field:         model.FieldDigest,
		Key:           "sha256",
		SelectedValue: strings.Repeat("f", 64),
		Values: []model.FieldValueEvidence{{
			Value:    strings.Repeat("f", 64),
			Evidence: model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1},
		}},
	}}
	data := semgrepSARIF(testArtifactURI, `
      {"ruleId":"aiebom.rule","locations":[{"physicalLocation":{"artifactLocation":{"uri":"`+testArtifactURI+`"}}}]}
`)
	if _, err := Import(graph, data, testArtifactURI, testArtifactURI, testDigest, time.Now()); err == nil || !strings.Contains(err.Error(), "SHA-256 did not") {
		t.Fatalf("inconsistent selected binding evidence was accepted: %v", err)
	}
}

func TestImportValidatesBindingCriticalSARIF(t *testing.T) {
	graph := targetGraph(testArtifactURI, testDigest)
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "version", data: `{"version":"2.0.0","runs":[]}`, wantErr: "version must be"},
		{name: "trailing", data: string(semgrepSARIF(testArtifactURI, "")) + `{}`, wantErr: "multiple JSON values"},
		{name: "failed invocation", data: `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"invocations":[{"executionSuccessful":false}],"results":[]}]}`, wantErr: "executionSuccessful=false"},
		{name: "bad kind", data: `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleId":"r","kind":"danger","message":{"text":"x"},"locations":[]}]}]}`, wantErr: "invalid kind"},
		{name: "bad rule index", data: `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleIndex":1,"message":{"text":"x"},"locations":[]}]}]}`, wantErr: "out of range"},
		{name: "bad suppression status", data: `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleId":"r","suppressions":[{"kind":"external","status":"ignored"}],"locations":[]}]}]}`, wantErr: "invalid status"},
		{name: "missing suppression kind", data: `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleId":"r","suppressions":[{"status":"accepted"}],"locations":[]}]}]}`, wantErr: "invalid kind"},
		{name: "URI base", data: `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleId":"r","locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.go","uriBaseId":"SRCROOT"}}}]}]}]}`, wantErr: "uriBaseId is unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Import(graph, []byte(test.data), testArtifactURI, testArtifactURI, testDigest, time.Now()); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Import error=%v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestImportUsesExplicitScannerRelativeURI(t *testing.T) {
	graph := targetGraph(testArtifactURI, testDigest)
	data := semgrepSARIF("main.go", `
      {"ruleId":"aiebom.rule","message":{"text":"x"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.go"}}}]}
`)
	result, err := Import(graph, data, testArtifactURI, "main.go", testDigest, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Attached != 1 || result.SARIFArtifactURI != "main.go" || result.ArtifactURI != testArtifactURI {
		t.Fatalf("unexpected mapped import: %+v", result)
	}
	if _, err := Import(graph, data, testArtifactURI, "other.go", testDigest, time.Now()); err != nil {
		t.Fatalf("non-matching but valid scanner URI should be skipped: %v", err)
	}
	if _, err := Import(graph, data, testArtifactURI, "../main.go", testDigest, time.Now()); err == nil || !strings.Contains(err.Error(), "clean relative URI") {
		t.Fatalf("unsafe scanner URI was accepted: %v", err)
	}
}

func TestImportHonorsSuppressionStatusAndAllLocations(t *testing.T) {
	graph := targetGraph(testArtifactURI, testDigest)
	rejected := semgrepSARIF(testArtifactURI, `
      {"ruleId":"aiebom.rule","suppressions":[{"kind":"external","status":"rejected"}],"locations":[{"physicalLocation":{"artifactLocation":{"uri":"`+testArtifactURI+`"}}}]}
`)
	result, err := Import(graph, rejected, testArtifactURI, testArtifactURI, testDigest, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Attached != 1 || result.SkippedResults != 0 {
		t.Fatalf("rejected suppression hid finding: %+v", result)
	}
	underReview := semgrepSARIF(testArtifactURI, `
      {"ruleId":"aiebom.rule","suppressions":[{"kind":"external","status":"underReview"}],"locations":[{"physicalLocation":{"artifactLocation":{"uri":"`+testArtifactURI+`"}}}]}
`)
	result, err = Import(graph, underReview, testArtifactURI, testArtifactURI, testDigest, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Attached != 1 || result.SkippedResults != 0 {
		t.Fatalf("under-review finding was hidden: %+v", result)
	}

	mixedLocations := semgrepSARIF(testArtifactURI, `
      {"ruleId":"aiebom.rule","analysisTarget":{"uri":"other.go"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"`+testArtifactURI+`"}}}]}
`)
	result, err = Import(graph, mixedLocations, testArtifactURI, testArtifactURI, testDigest, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Attached != 0 || result.SkippedResults != 1 {
		t.Fatalf("analysis target hid a conflicting result location: %+v", result)
	}
}

func TestArtifactIdentity(t *testing.T) {
	file, err := os.CreateTemp(".", "sarif-artifact-*.go")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	t.Cleanup(func() { _ = os.Remove(path) })
	content := []byte("package fixture\n")
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	uri, digest, err := ArtifactIdentity(filepath.Clean(path), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(content)
	if uri != filepath.ToSlash(filepath.Clean(path)) || digest != hex.EncodeToString(want[:]) {
		t.Fatalf("identity=(%q,%q), want=(%q,%q)", uri, digest, filepath.ToSlash(filepath.Clean(path)), hex.EncodeToString(want[:]))
	}
	if _, _, err := ArtifactIdentity(filepath.Join("..", "outside"), 1<<20); err == nil {
		t.Fatal("parent traversal was accepted")
	}
	spacedPath := "sarif artifact with space.go"
	if err := os.WriteFile(spacedPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(spacedPath) })
	spacedURI, _, err := ArtifactIdentity(spacedPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if spacedURI != "sarif%20artifact%20with%20space.go" {
		t.Fatalf("artifact URI was not canonically escaped: %q", spacedURI)
	}
}

func targetGraph(uri, digest string) model.Graph {
	return model.Graph{
		SchemaVersion: model.SchemaVersion,
		GeneratedAt:   time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		Source:        "mcp-runtime-client",
		Nodes: []model.Node{{
			ID:         targetNodeID(),
			Type:       "mcp_server",
			Name:       "demo-security-tools",
			Digests:    map[string]string{"sha256": digest},
			Properties: map[string]string{PropertyArtifactURI: uri},
			Evidence:   model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1},
		}},
		Edges: []model.Edge{},
	}
}

func targetNodeID() string {
	return model.StableNodeID("mcp_server", "", "demo-security-tools")
}

func semgrepSARIF(artifactURI, results string) []byte {
	return []byte(`{
  "version": "2.1.0",
  "$schema": "https://docs.oasis-open.org/sarif/sarif/v2.1.0/os/schemas/sarif-schema-2.1.0.json",
  "runs": [{
    "tool": {"driver": {
      "name": "Semgrep OSS",
      "semanticVersion": "1.146.0",
      "rules": [
        {"id":"aiebom.mcp-shell-injection","defaultConfiguration":{"level":"error"}},
        {"id":"aiebom.suppressed","defaultConfiguration":{"level":"warning"}},
        {"id":"aiebom.other-artifact","defaultConfiguration":{"level":"note"}},
        {"id":"aiebom.rule","defaultConfiguration":{"level":"warning"}}
      ]
    }},
    "invocations": [{"executionSuccessful":true}],
    "results": [` + results + `]
  }]
}`)
}

func nodeByType(t *testing.T, graph model.Graph, nodeType string) model.Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Type == nodeType {
			return node
		}
	}
	t.Fatalf("node type %q not found", nodeType)
	return model.Node{}
}

func hasEdge(graph model.Graph, from, to, relation string) bool {
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to && edge.Relation == relation {
			return true
		}
	}
	return false
}
