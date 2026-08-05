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

func TestEvaluateRejectsDeniedMCPReachabilityPath(t *testing.T) {
	graph := model.Graph{Nodes: []model.Node{
		{ID: "agent:1", Type: "agent", Name: "security-agent", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}},
		{ID: "mcp:1", Type: "mcp_server", Name: "demo-security-tools", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}},
		{ID: "tool:1", Type: "tool", Name: "shell.execute", Evidence: model.EvidenceSummary{Level: model.EvidenceDeclared}},
	}, Edges: []model.Edge{
		{From: "agent:1", To: "mcp:1", Relation: "connects_to"},
		{From: "mcp:1", To: "tool:1", Relation: "provides"},
	}}
	rules := Policy{DeniedPaths: []PathRule{{
		Name:      "agent-to-shell-capability",
		From:      NodeSelector{Type: "agent", MinimumEvidence: model.EvidenceObserved},
		Via:       []NodeSelector{{Type: "mcp_server"}},
		Relations: []string{"connects_to", "provides"},
		To:        NodeSelector{Type: "tool", NamePattern: `^shell\.execute$`, MinimumEvidence: model.EvidenceDeclared},
	}}}
	report, err := Evaluate(graph, rules, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Violations) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	violation := report.Violations[0]
	if violation.Rule != "denied-path:agent-to-shell-capability" || len(violation.PathNodeIDs) != 3 || len(violation.PathRelations) != 2 {
		t.Fatalf("unexpected path violation: %#v", violation)
	}
}

func TestEvaluateAllowsMissingDeniedPath(t *testing.T) {
	graph := model.Graph{Nodes: []model.Node{
		{ID: "agent:1", Type: "agent", Name: "security-agent", Evidence: model.EvidenceSummary{Level: model.EvidenceObserved}},
		{ID: "tool:1", Type: "tool", Name: "shell.execute", Evidence: model.EvidenceSummary{Level: model.EvidenceDeclared}},
	}}
	rules := Policy{DeniedPaths: []PathRule{{
		From:      NodeSelector{Type: "agent"},
		Relations: []string{"connects_to", "provides"},
		To:        NodeSelector{Type: "tool", NamePattern: "shell"},
	}}}
	report, err := Evaluate(graph, rules, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Fatalf("disconnected nodes matched a path: %#v", report)
	}
}

func TestEvaluateRejectsMalformedPathRule(t *testing.T) {
	_, err := Evaluate(model.Graph{}, Policy{DeniedPaths: []PathRule{{
		Relations: []string{"connects_to", "provides"},
		Via:       []NodeSelector{{Type: "mcp_server"}, {Type: "tool"}},
	}}}, time.Unix(1, 0))
	if err == nil {
		t.Fatal("malformed path rule was accepted")
	}
}
