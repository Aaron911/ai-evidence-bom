package graphdiff

import (
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
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

func TestCompareFindsConflictEvenWhenSelectedValueDoesNotChange(t *testing.T) {
	verified := model.EvidenceSummary{Level: model.EvidenceVerified, ObservationCount: 1}
	declared := model.EvidenceSummary{Level: model.EvidenceDeclared, ObservationCount: 1}
	beforeNode := model.Node{ID: "model:1", Type: "model", Name: "m", Version: "v1"}
	beforeNode.AddFieldEvidence(model.FieldVersion, "", "v1", verified)
	beforeNode.ResolveFieldEvidence()
	afterNode := beforeNode
	afterNode.FieldEvidence = append([]model.FieldEvidence(nil), beforeNode.FieldEvidence...)
	afterNode.FieldEvidence[0].Values = append([]model.FieldValueEvidence(nil), beforeNode.FieldEvidence[0].Values...)
	afterNode.AddFieldEvidence(model.FieldVersion, "", "v2", declared)
	afterNode.ResolveFieldEvidence()

	diff := Compare(model.Graph{Nodes: []model.Node{beforeNode}}, model.Graph{Nodes: []model.Node{afterNode}}, time.Unix(1, 0))
	if len(diff.ChangedNodes) != 1 || len(diff.ChangedNodes[0].Fields) != 2 {
		t.Fatalf("unexpected diff: %#v", diff)
	}
	if diff.ChangedNodes[0].Fields[0] != "version" || diff.ChangedNodes[0].Fields[1] != "fieldEvidence" {
		t.Fatalf("changed fields=%v", diff.ChangedNodes[0].Fields)
	}
}
