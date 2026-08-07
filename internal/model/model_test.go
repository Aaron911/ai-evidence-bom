package model

import (
	"testing"
	"time"
)

func TestResolveFieldEvidenceUsesRecencyThenLexicalTieBreak(t *testing.T) {
	seenAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		first  EvidenceSummary
		second EvidenceSummary
		want   string
	}{
		{
			name:   "later observation",
			first:  EvidenceSummary{Level: EvidenceObserved, LastSeen: seenAt, ObservationCount: 1},
			second: EvidenceSummary{Level: EvidenceObserved, LastSeen: seenAt.Add(time.Minute), ObservationCount: 1},
			want:   "v2",
		},
		{
			name:   "lexical exact tie",
			first:  EvidenceSummary{Level: EvidenceObserved, LastSeen: seenAt, ObservationCount: 1},
			second: EvidenceSummary{Level: EvidenceObserved, LastSeen: seenAt, ObservationCount: 1},
			want:   "v1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := Node{}
			node.AddFieldEvidence(FieldVersion, "", "v1", test.first)
			node.AddFieldEvidence(FieldVersion, "", "v2", test.second)
			node.ResolveFieldEvidence()
			if node.Version != test.want || !node.FieldEvidence[0].Conflict {
				t.Fatalf("resolved node=%+v want version=%q with conflict", node, test.want)
			}
		})
	}
}
