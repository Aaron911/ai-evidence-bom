package policy

import (
	"testing"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

func TestEvaluateRejectsDangerousAndWeakEvidence(t *testing.T) {
	graph := model.Graph{Nodes: []model.Node{
		{ID: "tool:1", Type: "tool", Name: "shell.execute", Version: "1", Evidence: model.EvidenceSummary{Level: model.EvidenceDeclared}},
	}}
	rules := Policy{
		MinimumEvidence:    map[string]model.EvidenceLevel{"tool": model.EvidenceObserved},
		DeniedNamePatterns: []string{"(?i)shell"},
	}
	report, err := Evaluate(graph, rules, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Violations) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestEvaluateRejectsInvalidEvidenceLevel(t *testing.T) {
	_, err := Evaluate(model.Graph{}, Policy{
		MinimumEvidence: map[string]model.EvidenceLevel{"model": "certain"},
	}, time.Unix(1, 0))
	if err == nil {
		t.Fatal("invalid evidence level was accepted")
	}
}
