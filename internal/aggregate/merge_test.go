package aggregate

import (
	"reflect"
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

func TestMergePrefersStrongEvidenceAndExposesConflictRegardlessOfArrivalOrder(t *testing.T) {
	verifiedAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	declaredAt := verifiedAt.Add(time.Hour)
	nodeID := model.StableNodeID("model", "provider", "stable-alias")
	verifiedNode := model.Node{
		ID: nodeID, Type: "model", Name: "stable-alias", Provider: "provider", Version: "weights-v1",
		Digests: map[string]string{"sha256": "verified-digest"}, Properties: map[string]string{"region": "verified-region"},
		Evidence: model.EvidenceSummary{
			Level: model.EvidenceVerified, ObservationCount: 1, FirstSeen: verifiedAt, LastSeen: verifiedAt,
			Sources: []string{"signature-verifier"},
		},
	}
	verifiedNode.AddFieldEvidence(model.FieldVersion, "", "weights-v1", verifiedNode.Evidence)
	verifiedNode.AddFieldEvidence(model.FieldDigest, "sha256", "verified-digest", verifiedNode.Evidence)
	verifiedNode.AddFieldEvidence(model.FieldProperty, "region", "verified-region", verifiedNode.Evidence)
	verifiedNode.ResolveFieldEvidence()
	declaredNode := model.Node{
		ID: nodeID, Type: "model", Name: "stable-alias", Provider: "provider", Version: "weights-v2",
		Digests: map[string]string{"sha256": "declared-digest"}, Properties: map[string]string{"region": "declared-region"},
		Evidence: model.EvidenceSummary{
			Level: model.EvidenceDeclared, ObservationCount: 1, FirstSeen: declaredAt, LastSeen: declaredAt,
			Sources: []string{"deployment-config"},
		},
	}
	declaredNode.AddFieldEvidence(model.FieldVersion, "", "weights-v2", declaredNode.Evidence)
	declaredNode.AddFieldEvidence(model.FieldDigest, "sha256", "declared-digest", declaredNode.Evidence)
	declaredNode.AddFieldEvidence(model.FieldProperty, "region", "declared-region", declaredNode.Evidence)
	declaredNode.ResolveFieldEvidence()
	verified := model.Graph{Source: "evidence", Nodes: []model.Node{verifiedNode}}
	declared := model.Graph{Source: "evidence", Nodes: []model.Node{declaredNode}}

	forward := Merge(Merge(model.Graph{Source: "evidence"}, verified, verifiedAt), declared, declaredAt)
	reverse := Merge(Merge(model.Graph{Source: "evidence"}, declared, declaredAt), verified, declaredAt)
	if !reflect.DeepEqual(forward.Nodes, reverse.Nodes) {
		t.Fatalf("arrival order changed merged nodes:\nforward=%+v\nreverse=%+v", forward.Nodes, reverse.Nodes)
	}
	node := forward.Nodes[0]
	if node.Version != "weights-v1" || node.Digests["sha256"] != "verified-digest" || node.Properties["region"] != "verified-region" {
		t.Fatalf("weaker newer evidence replaced verified fields: %+v", node)
	}
	if len(node.FieldEvidence) != 3 {
		t.Fatalf("field evidence=%d want=3: %+v", len(node.FieldEvidence), node.FieldEvidence)
	}
	for _, claim := range node.FieldEvidence {
		if !claim.Conflict || len(claim.Values) != 2 {
			t.Fatalf("conflict was not retained: %+v", claim)
		}
	}
}

func TestMergeDoesNotInventFieldVerificationForLegacyGraph(t *testing.T) {
	seenAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	legacy := model.Graph{Nodes: []model.Node{{
		ID: "model:legacy", Type: "model", Name: "legacy", Version: "v1",
		ObservedVersions: []string{"v0", "v1"},
		Evidence:         model.EvidenceSummary{Level: model.EvidenceVerified, ObservationCount: 7, FirstSeen: seenAt, LastSeen: seenAt},
	}}}
	merged := Merge(model.Graph{}, legacy, seenAt)
	claim := merged.Nodes[0].FieldEvidence[0]
	if merged.Nodes[0].Version != "v1" || len(claim.Values) != 2 || claim.Values[0].Evidence.Level != model.EvidenceInferred || claim.Values[0].Evidence.ObservationCount != 1 {
		t.Fatalf("legacy field inherited unverifiable node evidence: %+v", claim)
	}
}
