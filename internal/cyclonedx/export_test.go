package cyclonedx

import (
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

func TestExportMapsModelAndRelations(t *testing.T) {
	graph := model.Graph{
		GeneratedAt: time.Unix(1, 0).UTC(),
		Source:      "demo",
		Nodes: []model.Node{
			{ID: "agent:1", Type: "agent", Name: "agent", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}},
			{ID: "model:1", Type: "model", Name: "model", Evidence: model.EvidenceSummary{Level: model.EvidenceVerified}},
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
	if len(bom.Dependencies) != 2 || len(bom.Dependencies[0].DependsOn) != 1 {
		t.Fatalf("dependencies=%#v", bom.Dependencies)
	}
}
