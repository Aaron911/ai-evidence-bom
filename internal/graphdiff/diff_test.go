package graphdiff

import (
	"testing"
	"time"

	"aibom-evidence/internal/model"
)

func TestCompareFindsVersionAndCapabilityChanges(t *testing.T) {
	before := model.Graph{
		Source: "before",
		Nodes:  []model.Node{{ID: "model:1", Type: "model", Name: "m", Version: "1"}},
		Edges:  []model.Edge{{ID: "edge:1", From: "agent:1", To: "model:1", Relation: "uses", Evidence: model.EvidenceSummary{Level: model.EvidenceDeclared}}},
	}
	after := model.Graph{Source: "after", Nodes: []model.Node{
		{ID: "model:1", Type: "model", Name: "m", Version: "2"},
		{ID: "tool:1", Type: "tool", Name: "shell.execute"},
	}, Edges: []model.Edge{{ID: "edge:1", From: "agent:1", To: "model:1", Relation: "uses", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}}}}
	diff := Compare(before, after, time.Unix(1, 0))
	if len(diff.AddedNodes) != 1 || len(diff.ChangedNodes) != 1 || !diff.HasChanges() {
		t.Fatalf("unexpected diff: %#v", diff)
	}
	if diff.ChangedNodes[0].Fields[0] != "version" {
		t.Fatalf("changed fields=%v", diff.ChangedNodes[0].Fields)
	}
	if len(diff.ChangedEdges) != 1 || diff.ChangedEdges[0].Fields[0] != "evidence.level" {
		t.Fatalf("changed edges=%v", diff.ChangedEdges)
	}
}
