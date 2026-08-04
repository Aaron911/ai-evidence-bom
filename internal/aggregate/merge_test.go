package aggregate

import (
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

func TestMergeTracksVersionsAndEvidence(t *testing.T) {
	firstSeen := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	lastSeen := firstSeen.Add(time.Hour)
	nodeID := model.StableNodeID("model", "openai", "gpt")
	current := model.Graph{
		SchemaVersion: model.SchemaVersion,
		Source:        "otlp-http",
		Nodes: []model.Node{{
			ID: nodeID, Type: "model", Name: "gpt", Provider: "openai", Version: "gpt-v1",
			ObservedVersions: []string{"gpt-v1"},
			Properties:       map[string]string{"region": "old"},
			Evidence: model.EvidenceSummary{
				Level: model.EvidenceObserved, ObservationCount: 1, FirstSeen: firstSeen, LastSeen: firstSeen,
				Sources: []string{"service-a"}, TraceIDs: []string{"trace-a"},
			},
		}},
	}
	incoming := model.Graph{
		SchemaVersion: model.SchemaVersion,
		Nodes: []model.Node{{
			ID: nodeID, Type: "model", Name: "gpt", Provider: "openai", Version: "gpt-v2",
			ObservedVersions: []string{"gpt-v2"},
			Properties:       map[string]string{"region": "new"},
			Evidence: model.EvidenceSummary{
				Level: model.EvidenceVerified, ObservationCount: 2, FirstSeen: lastSeen, LastSeen: lastSeen,
				Sources: []string{"service-b"}, TraceIDs: []string{"trace-b"},
			},
		}},
	}

	merged := Merge(current, incoming, lastSeen)
	if len(merged.Nodes) != 1 {
		t.Fatalf("nodes=%d want=1", len(merged.Nodes))
	}
	node := merged.Nodes[0]
	if node.Version != "gpt-v2" || len(node.ObservedVersions) != 2 {
		t.Fatalf("unexpected versions: current=%q observed=%v", node.Version, node.ObservedVersions)
	}
	if node.Evidence.Level != model.EvidenceVerified || node.Evidence.ObservationCount != 3 {
		t.Fatalf("unexpected evidence: %+v", node.Evidence)
	}
	if node.Properties["region"] != "new" {
		t.Fatalf("newer property was not retained: %v", node.Properties)
	}
	if !node.Evidence.FirstSeen.Equal(firstSeen) || !node.Evidence.LastSeen.Equal(lastSeen) {
		t.Fatalf("unexpected evidence window: %s - %s", node.Evidence.FirstSeen, node.Evidence.LastSeen)
	}
	if merged.Metadata["aggregation.mode"] != "continuous" {
		t.Fatalf("missing aggregation metadata: %v", merged.Metadata)
	}
}

func TestMergeDoesNotLetOlderSnapshotReplaceCurrentVersion(t *testing.T) {
	newer := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)
	nodeID := model.StableNodeID("agent", "", "reviewer")
	current := model.Graph{Nodes: []model.Node{{
		ID: nodeID, Type: "agent", Name: "reviewer", Version: "2", Properties: map[string]string{"env": "prod"},
		Evidence: model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1, FirstSeen: newer, LastSeen: newer},
	}}}
	incoming := model.Graph{Nodes: []model.Node{{
		ID: nodeID, Type: "agent", Name: "reviewer", Version: "1", Properties: map[string]string{"env": "dev"},
		Evidence: model.EvidenceSummary{Level: model.EvidenceObserved, ObservationCount: 1, FirstSeen: older, LastSeen: older},
	}}}

	merged := Merge(current, incoming, newer)
	if merged.Nodes[0].Version != "2" || merged.Nodes[0].Properties["env"] != "prod" {
		t.Fatalf("older snapshot replaced current values: %+v", merged.Nodes[0])
	}
}
