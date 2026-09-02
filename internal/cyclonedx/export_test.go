package cyclonedx

import (
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

func TestExportMapsModelAndRelations(t *testing.T) {
	modelNode := model.Node{ID: "model:1", Type: "model", Name: "model", Evidence: model.EvidenceSummary{Level: model.EvidenceVerified}}
	modelNode.AddFieldEvidence(model.FieldVersion, "", "v1", model.EvidenceSummary{Level: model.EvidenceVerified, ObservationCount: 1})
	modelNode.AddFieldEvidence(model.FieldVersion, "", "v2", model.EvidenceSummary{Level: model.EvidenceDeclared, ObservationCount: 1})
	modelNode.ResolveFieldEvidence()
	graph := model.Graph{
		GeneratedAt: time.Unix(1, 0).UTC(),
		Source:      "demo",
		Nodes: []model.Node{
			{ID: "agent:1", Type: "agent", Name: "agent", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}},
			modelNode,
		},
		Edges: []model.Edge{{ID: "edge:1", From: "agent:1", To: "model:1", Relation: "uses"}},
	}
	bom := Export(graph)
	if bom.SpecVersion != "1.7" || len(bom.Components) != 2 {
		t.Fatalf("unexpected bom: %#v", bom)
	}
	if len(bom.Metadata.Tools.Components) != 1 ||
		bom.Metadata.Tools.Components[0].Name != "aiebom" ||
		bom.Metadata.Tools.Components[0].Type != "application" {
		t.Fatalf("unexpected metadata tools: %#v", bom.Metadata.Tools)
	}
	if bom.Components[1].Type != "machine-learning-model" {
		t.Fatalf("model component type=%q", bom.Components[1].Type)
	}
	if !hasProperty(bom.Components[1].Properties, "aibom:field-conflict", "version") ||
		!hasProperty(bom.Components[1].Properties, "aibom:field-evidence:version:selected", "v1") {
		t.Fatalf("field evidence was not exported: %#v", bom.Components[1].Properties)
	}
	if len(bom.Dependencies) != 2 || len(bom.Dependencies[0].DependsOn) != 1 {
		t.Fatalf("dependencies=%#v", bom.Dependencies)
	}
}

func TestExportMapsFindingToVulnerabilityWithoutSourceContent(t *testing.T) {
	finding := model.Node{
		ID:       "finding:1",
		Type:     "finding",
		Name:     "scanner.rule",
		Provider: "Semgrep OSS",
		Properties: map[string]string{
			"aiebom.finding.sarif.level":     "error",
			"aiebom.finding.scanner.version": "1.146.0",
			"aiebom.finding.artifact.uri":    "server.go",
			"aiebom.finding.artifact.sha256": "digest",
			"aiebom.finding.assertion":       "scanner-reported",
		},
		Evidence: model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1},
	}
	finding.AddFieldEvidence(model.FieldProperty, "aiebom.finding.sarif.level", "error", finding.Evidence)
	finding.ResolveFieldEvidence()
	graph := model.Graph{
		GeneratedAt: time.Unix(1, 0).UTC(),
		Source:      "demo",
		Nodes: []model.Node{
			{ID: "mcp:1", Type: "mcp_server", Name: "server", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}},
			finding,
		},
		Edges: []model.Edge{{ID: "edge:1", From: "mcp:1", To: "finding:1", Relation: "affected_by"}},
	}
	bom := Export(graph)
	if len(bom.Components) != 1 || bom.Components[0].BOMRef != "mcp:1" {
		t.Fatalf("finding was exported as a component: %#v", bom.Components)
	}
	if len(bom.Dependencies) != 1 || len(bom.Dependencies[0].DependsOn) != 0 {
		t.Fatalf("finding edge leaked into dependencies: %#v", bom.Dependencies)
	}
	if len(bom.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities=%#v", bom.Vulnerabilities)
	}
	vulnerability := bom.Vulnerabilities[0]
	if vulnerability.ID != "scanner.rule" || vulnerability.Source.Name != "Semgrep OSS" ||
		len(vulnerability.Affects) != 1 || vulnerability.Affects[0].Ref != "mcp:1" ||
		!hasProperty(vulnerability.Properties, "aibom:sarif:level", "error") ||
		!hasProperty(vulnerability.Properties, "aibom:field-evidence:property:aiebom.finding.sarif.level:candidate:observed", "error") ||
		!hasProperty(vulnerability.Properties, "aibom:finding:assertion", "scanner-reported") {
		t.Fatalf("unexpected vulnerability: %#v", vulnerability)
	}
}

func hasProperty(properties []Property, name, value string) bool {
	for _, property := range properties {
		if property.Name == name && property.Value == value {
			return true
		}
	}
	return false
}
